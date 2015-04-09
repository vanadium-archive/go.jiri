// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/runutil"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

const (
	stagingBlessingsRoot    = "dev.staging.v.io" // TODO(jingjin): use a better name and update prod.go.
	localMountTable         = "/ns.dev.staging.v.io:8151"
	globalMountTable        = "/ns.dev.staging.v.io:8101"
	objNameForAllApps       = "devmgr/apps"
	objNameForDeviceManager = "devices/vanadium-cell-master/devmgr/device"
)

var (
	defaultReleaseTestTimeout = time.Minute * 5

	serviceBinaries = []string{
		"binaryd",
		"applicationd",
		"proxyd",
		"identityd",
		"mounttabled",
		"roled",
		"deviced",
	}
)

// vanadiumReleaseTest updates binaries of staging cloud services and run tests for them.
func vanadiumReleaseTest(ctx *tool.Context, testName string, opts ...TestOpt) (_ *TestResult, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}

	cleanup, err := initTest(ctx, testName, []string{})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	type step struct {
		msg string
		fn  func() error
	}
	credDir := getCredDirOptValue(opts)
	steps := []step{
		step{
			msg: fmt.Sprintf("Fetching credentials from %v\n", credDir),
			fn: func() error {
				_, err := os.Stat(credDir)
				return err
			},
		},
		step{
			msg: "Build binaries\n",
			fn:  func() error { return buildBinaries(ctx, root) },
		},
		step{
			msg: "Update services\n",
			fn:  func() error { return updateServices(ctx, root, credDir) },
		},
		step{
			msg: "Check services\n",
			fn:  func() error { return checkServices(ctx) },
		},
		step{
			msg: "Create snapshot\n",
			fn:  func() error { return createSnapshot(ctx) },
		},
	}
	for _, step := range steps {
		if result, err := invoker(ctx, step.msg, step.fn); result != nil || err != nil {
			return result, err
		}
	}
	return &TestResult{Status: TestPassed}, nil
}

// invoker invokes the given function and returns TestResult and/or errors based
// on function's results.
func invoker(ctx *tool.Context, msg string, fn func() error) (*TestResult, error) {
	if err := fn(); err != nil {
		Fail(ctx, msg)
		if err == runutil.CommandTimedOutErr {
			return &TestResult{
				Status:       TestTimedOut,
				TimeoutValue: defaultReleaseTestTimeout,
			}, nil
		}
		fmt.Fprintf(ctx.Stderr(), "%s\n", err.Error())
		return nil, internalTestError{err, msg}
	}
	Pass(ctx, msg)
	return nil, nil
}

// getCredDirOptValue returns the value of credentials dir from the given
// TestOpt slice.
func getCredDirOptValue(opts []TestOpt) string {
	credDir := ""
	for _, opt := range opts {
		switch v := opt.(type) {
		case CredDirOpt:
			credDir = string(v)
		}
	}
	return credDir
}

// buildBinaries builds all vanadium binaries.
func buildBinaries(ctx *tool.Context, root string) error {
	args := []string{"go", "install", "v.io/..."}
	if err := ctx.Run().Command("v23", args...); err != nil {
		return err
	}
	return nil
}

// updateServices pushes services' binaries to the applications and binaries
// services and tells the device manager to update all its app.
func updateServices(ctx *tool.Context, root, credDir string) (e error) {
	deviceBin := filepath.Join(root, "release", "go", "bin", "device")
	credentialsArg := fmt.Sprintf("--v23.credentials=%s", credDir)
	nsArg := fmt.Sprintf("--v23.namespace.root=%s", globalMountTable)

	// Push all binaries.
	{
		args := []string{
			credentialsArg,
			nsArg,
			"publish",
			"--goos=linux",
			"--goarch=amd64",
		}
		args = append(args, serviceBinaries...)
		if err := ctx.Run().TimedCommand(defaultReleaseTestTimeout, deviceBin, args...); err != nil {
			return err
		}
	}

	// Update services (except for device manager).
	{
		args := []string{
			credentialsArg,
			fmt.Sprintf("--v23.namespace.root=%s", localMountTable),
			"updateall",
			objNameForAllApps,
		}
		if err := ctx.Run().TimedCommand(defaultReleaseTestTimeout, deviceBin, args...); err != nil {
			return err
		}
	}

	// Update the envelope's title from "deviced" to "device manager".
	{
		applicationBin := filepath.Join(root, "release", "go", "bin", "application")
		appName := "applications/deviced/0"
		appProfile := "linux-amd64"
		// Get current envelope.
		args := []string{
			credentialsArg,
			nsArg,
			"match",
			appName,
			appProfile,
		}
		var out bytes.Buffer
		opts := ctx.Run().Opts()
		opts.Stdout = io.MultiWriter(opts.Stdout, &out)
		opts.Stderr = io.MultiWriter(opts.Stderr, &out)
		if err := ctx.Run().CommandWithOpts(opts, applicationBin, args...); err != nil {
			return err
		}

		// Replace title.
		strEnvelope := strings.Replace(out.String(), `"Title": "deviced"`, `"Title": "device manager"`, -1)
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		filename := filepath.Join(tmpDir, "envelope")
		if err := ctx.Run().WriteFile(filename, []byte(strEnvelope), os.FileMode(0600)); err != nil {
			return err
		}

		// Update envelope.
		args = []string{
			credentialsArg,
			nsArg,
			"put",
			appName,
			appProfile,
			filename,
		}
		if err := ctx.Run().Command(applicationBin, args...); err != nil {
			return err
		}
	}

	// Update Device Manager.
	{
		args := []string{
			credentialsArg,
			fmt.Sprintf("--v23.namespace.root=%s", globalMountTable),
			"update",
			objNameForDeviceManager,
		}
		fmt.Fprintf(ctx.Stdout(), `
######################################################################
Resolve errors are expected as we are waiting for mounttable to be up.
######################################################################
`)
		if err := ctx.Run().TimedCommand(defaultReleaseTestTimeout, deviceBin, args...); err != nil {
			return err
		}
	}
	return nil
}

// checkServices runs "v23 test run vanadium-prod-services-test" against
// staging.
func checkServices(ctx *tool.Context) error {
	args := []string{
		"test",
		"run",
		fmt.Sprintf("--namespace_root=%s", globalMountTable),
		fmt.Sprintf("--blessings_root=%s", stagingBlessingsRoot),
		"vanadium-prod-services-test",
	}
	if err := ctx.Run().TimedCommand(defaultReleaseTestTimeout, "v23", args...); err != nil {
		return err
	}
	return nil
}

// createSnapshot creates a snapshot with "release" label.
func createSnapshot(ctx *tool.Context) (e error) {
	args := []string{
		"snapshot",
		"--remote",
		"create",
		"--time_format=2006-01-02", // Only include date in label names
		"release",
	}
	if err := ctx.Run().Command("v23", args...); err != nil {
		return err
	}
	return nil
}
