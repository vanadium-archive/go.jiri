// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/runutil"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

const (
	bucket                  = "gs://vanadium-release"
	localMountTable         = "/ns.dev.staging.v.io:8151"
	globalMountTable        = "/ns.dev.staging.v.io:8101"
	manifestEnvVar          = "SNAPSHOT_MANIFEST"
	numRetries              = 30
	objNameForDeviceManager = "devices/vanadium-cell-master/devmgr/device"
	propertiesFile          = ".release_candidate_properties"
	retryPeriod             = 10 * time.Second
	stagingBlessingsRoot    = "dev.staging.v.io" // TODO(jingjin): use a better name and update prod.go.
	snapshotName            = "rc"
	testsEnvVar             = "TESTS"
)

var (
	defaultReleaseTestTimeout = time.Minute * 5

	serviceBinaries = []string{
		"applicationd",
		"binaryd",
		"deviced",
		"groupsd",
		"identityd",
		"mounttabled",
		"proxyd",
		"proxyd:vlab-proxyd",
		"roled",
	}

	deviceManagerApplications = []string{
		"devmgr/apps/applicationd",
		"devmgr/apps/binaryd",
		"devmgr/apps/groupsd",
		"devmgr/apps/identityd",
		"devmgr/apps/proxyd",
		"devmgr/apps/roled",
		"devmgr/apps/VLabProxy",
	}
)

// vanadiumReleaseCandidate updates binaries of staging cloud services and run tests for them.
func vanadiumReleaseCandidate(ctx *tool.Context, testName string, opts ...Opt) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}

	cleanup, err := initTest(ctx, testName, []string{"syncbase"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	type step struct {
		msg string
		fn  func() error
	}
	rcLabel := ""
	adminCredDir, publisherCredDir := getCredDirOptValues(opts)
	steps := []step{
		step{
			msg: "Extract release candidate label\n",
			fn: func() error {
				var err error
				rcLabel, err = extractRCLabel()
				return err
			},
		},
		step{
			msg: fmt.Sprintf("Checking existence of credentials in %v (admin) and %v (publisher)\n", adminCredDir, publisherCredDir),
			fn: func() error {
				if _, err := os.Stat(adminCredDir); err != nil {
					return err
				}
				if _, err := os.Stat(publisherCredDir); err != nil {
					return err
				}
				return nil
			},
		},
		step{
			msg: "Prepare binaries\n",
			fn:  func() error { return prepareBinaries(ctx, root, rcLabel) },
		},
		step{
			msg: "Update services\n",
			fn:  func() error { return updateServices(ctx, root, adminCredDir, publisherCredDir) },
		},
		step{
			msg: "Check services\n",
			fn: func() error {
				// Wait 5 minutes.
				fmt.Fprintf(ctx.Stdout(), "Wait for 5 minutes before checking services...\n")
				time.Sleep(time.Minute * 5)

				return checkServices(ctx)
			},
		},
		step{
			msg: "Update the 'latest' file\n",
			fn:  func() error { return updateLatestFile(ctx, rcLabel) },
		},
	}
	for _, step := range steps {
		if result, err := invoker(ctx, step.msg, step.fn); result != nil || err != nil {
			return result, err
		}
	}
	return &test.Result{Status: test.Passed}, nil
}

// invoker invokes the given function and returns test.Result and/or
// errors based on function's results.
func invoker(ctx *tool.Context, msg string, fn func() error) (*test.Result, error) {
	if err := fn(); err != nil {
		test.Fail(ctx, msg)
		if err == runutil.CommandTimedOutErr {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: defaultReleaseTestTimeout,
			}, nil
		}
		fmt.Fprintf(ctx.Stderr(), "%s\n", err.Error())
		return nil, internalTestError{err, msg}
	}
	test.Pass(ctx, msg)
	return nil, nil
}

// extractRCLabel extracts release candidate label from the manifest path stored
// in the <manifestEnvVar> environment variable.
func extractRCLabel() (string, error) {
	manifestPath := os.Getenv(manifestEnvVar)
	if manifestPath == "" {
		return "", fmt.Errorf("Environment variable %q not set", manifestEnvVar)
	}
	return filepath.Base(manifestPath), nil
}

// getCredDirOptValues returns the values of credentials dirs (admin, publisher)
// from the given Opt slice.
func getCredDirOptValues(opts []Opt) (string, string) {
	adminCredDir, publisherCredDir := "", ""
	for _, opt := range opts {
		switch v := opt.(type) {
		case AdminCredDirOpt:
			adminCredDir = string(v)
		case PublisherCredDirOpt:
			publisherCredDir = string(v)
		}
	}
	return adminCredDir, publisherCredDir
}

// prepareBinaries builds all vanadium binaries and uploads them to Google Storage bucket.
func prepareBinaries(ctx *tool.Context, root, rcLabel string) error {
	// Build binaries.
	//
	// The "leveldb" tag is needed to compile the levelDB-based storage
	// engine for the groups service. See v.io/i/632 for more details.
	args := []string{"go", "install", "-tags=leveldb", "v.io/..."}
	if err := ctx.Run().Command("v23", args...); err != nil {
		return err
	}

	// Upload binaries.
	args = []string{
		"-q", "-m", "cp", "-r",
		filepath.Join(root, "release", "go", "bin"),
		fmt.Sprintf("%s/%s", bucket, rcLabel),
	}
	if err := ctx.Run().Command("gsutil", args...); err != nil {
		return err
	}

	// Upload the .done file to signal that all binaries have been
	// successfully uploaded.
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return err
	}
	defer ctx.Run().RemoveAll(tmpDir)
	doneFile := filepath.Join(tmpDir, ".done")
	if err := ctx.Run().WriteFile(doneFile, nil, os.FileMode(0600)); err != nil {
		return err
	}
	args = []string{"-q", "cp", doneFile, fmt.Sprintf("%s/%s", bucket, rcLabel)}
	if err := ctx.Run().Command("gsutil", args...); err != nil {
		return err
	}

	return nil
}

// updateServices pushes services' binaries to the applications and binaries
// services and tells the device manager to update all its app.
func updateServices(ctx *tool.Context, root, adminCredDir, publisherCredDir string) (e error) {
	debugBin := filepath.Join(root, "release", "go", "bin", "debug")
	deviceBin := filepath.Join(root, "release", "go", "bin", "device")
	adminCredentialsArg := fmt.Sprintf("--v23.credentials=%s", adminCredDir)
	publisherCredentialsArg := fmt.Sprintf("--v23.credentials=%s", publisherCredDir)
	nsArg := fmt.Sprintf("--v23.namespace.root=%s", globalMountTable)

	// Push all binaries.
	{
		fmt.Fprintln(ctx.Stdout(), "\n\n### Pushing binaries ###")
		args := []string{
			publisherCredentialsArg,
			nsArg,
			"publish",
			"--goos=linux",
			"--goarch=amd64",
		}
		msg := "Push binaries\n"
		args = append(args, serviceBinaries...)
		if err := ctx.Run().TimedCommand(defaultReleaseTestTimeout, deviceBin, args...); err != nil {
			test.Fail(ctx, msg)
			return err
		}
		test.Pass(ctx, msg)
	}

	// A helper function to update a single app.
	updateAppFn := func(appName string) error {
		args := []string{
			adminCredentialsArg,
			fmt.Sprintf("--v23.namespace.root=%s", localMountTable),
			"update",
			"-parallelism=BYKIND",
			appName + "/...",
		}
		msg := fmt.Sprintf("Update %q\n", appName)
		if err := ctx.Run().TimedCommand(defaultReleaseTestTimeout, deviceBin, args...); err != nil {
			test.Fail(ctx, msg)
			return err
		}
		test.Pass(ctx, msg)
		return nil
	}

	// A helper function to check a single app's build time.
	checkBuildTimeFn := func(appName string) error {
		msg := fmt.Sprintf("Verify build time for %q\n", appName)
		now := time.Now()
		adminCredentialsArg := fmt.Sprintf("--v23.credentials=%s", adminCredDir)
		args := []string{
			adminCredentialsArg,
			fmt.Sprintf("--v23.namespace.root=%s", localMountTable),
			"stats",
			"read",
			fmt.Sprintf("%s/*/*/stats/system/metadata/build.Time", appName),
		}
		var out bytes.Buffer
		opts := ctx.Run().Opts()
		opts.Stdout = io.MultiWriter(opts.Stdout, &out)
		if err := ctx.Run().TimedCommandWithOpts(defaultReleaseTestTimeout, opts, debugBin, args...); err != nil {
			test.Fail(ctx, msg)
			return err
		}
		// TODO(jingjin): check the build manifest label after changing the
		// pre-release process to first cut the snapshot and then get the binaries
		// for staging from the snapshot where we should be able to exactly match
		// the build label.
		expectedBuildTime := now.Format("2006-01-02")
		buildTimeRE := regexp.MustCompile(fmt.Sprintf(`.*build\.Time:\s%sT.*`, expectedBuildTime))
		statsOutput := out.String()
		if !buildTimeRE.MatchString(statsOutput) {
			test.Fail(ctx, msg)
			return fmt.Errorf("Failed to verify build time.\nWant: %s\nGot: %s", expectedBuildTime, statsOutput)
		}
		test.Pass(ctx, msg)
		return nil
	}

	// Update services except for device manager and mounttable.
	{
		fmt.Fprintln(ctx.Stdout(), "\n\n### Updating services other than device manager and mounttable ###")
		for _, app := range deviceManagerApplications {
			if err := updateAppFn(app); err != nil {
				return err
			}
			if err := checkBuildTimeFn(app); err != nil {
				return err
			}
		}
	}

	// Update Device Manager.
	{
		fmt.Fprintln(ctx.Stdout(), "\n\n### Updating device manager ###")
		if err := updateDeviceManagerEnvelope(ctx, root, publisherCredentialsArg, nsArg); err != nil {
			return err
		}
		args := []string{
			adminCredentialsArg,
			fmt.Sprintf("--v23.namespace.root=%s", globalMountTable),
			"update",
			objNameForDeviceManager,
		}
		if err := ctx.Run().TimedCommand(defaultReleaseTestTimeout, deviceBin, args...); err != nil {
			return err
		}
		if err := waitForMounttable(ctx, root, adminCredentialsArg, localMountTable, `.*8151/devmgr.*`); err != nil {
			return err
		}
		// TODO(jingjin): check build time for device manager.
	}

	// Update mounttable last.
	{
		fmt.Fprintln(ctx.Stdout(), "\n\n### Updating mounttable ###")
		mounttableName := "devmgr/apps/mounttabled"
		if err := updateAppFn(mounttableName); err != nil {
			return err
		}
		if err := waitForMounttable(ctx, root, adminCredentialsArg, globalMountTable, `.+`); err != nil {
			return err
		}
		if err := checkBuildTimeFn(mounttableName); err != nil {
			return err
		}
	}
	return nil
}

// updateDeviceManagerEnvelope updates the envelope's title from "deviced" to
// "device manager".
func updateDeviceManagerEnvelope(ctx *tool.Context, root, credentialsArg, nsArg string) (e error) {
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
	return nil
}

// waitForMounttable waits for the given mounttable to be up and optionally
// checks output against outputRegexp if it is not empty.
// (timeout: 5 minutes)
func waitForMounttable(ctx *tool.Context, root, credentialsArg, mounttableRoot, outputRegexp string) error {
	debugBin := filepath.Join(root, "release", "go", "bin", "debug")
	args := []string{
		credentialsArg,
		"glob",
		mounttableRoot + "/*",
	}
	up := false
	outputRE := regexp.MustCompile(outputRegexp)
	for i := 0; i < numRetries; i++ {
		var out bytes.Buffer
		opts := ctx.Run().Opts()
		opts.Stdout = io.MultiWriter(opts.Stdout, &out)
		err := ctx.Run().CommandWithOpts(opts, debugBin, args...)
		if err != nil || !outputRE.MatchString(out.String()) {
			time.Sleep(retryPeriod)
			continue
		} else {
			up = true
			break
		}
	}
	if !up {
		return fmt.Errorf("mounttable %q not up after 5 minute", mounttableRoot)
	}
	return nil
}

// checkServices runs "v23 test run vanadium-prod-services-test" against
// staging.
func checkServices(ctx *tool.Context) error {
	args := []string{
		"test",
		"run",
		fmt.Sprintf("--v23.namespace.root=%s", globalMountTable),
		fmt.Sprintf("--blessings-root=%s", stagingBlessingsRoot),
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
		"--time-format=2006-01-02", // Only include date in label names
		"release",
	}
	if err := ctx.Run().Command("v23", args...); err != nil {
		return err
	}
	return nil
}

// updateLatestFile updates the "latest" file in Google Storage bucket to the
// given release candidate label.
func updateLatestFile(ctx *tool.Context, rcLabel string) error {
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return err
	}
	defer ctx.Run().RemoveAll(tmpDir)
	latestFile := filepath.Join(tmpDir, "latest")
	if err := ctx.Run().WriteFile(latestFile, []byte(rcLabel), os.FileMode(0600)); err != nil {
		return err
	}
	args := []string{"-q", "cp", latestFile, fmt.Sprintf("%s/latest", bucket)}
	if err := ctx.Run().Command("gsutil", args...); err != nil {
		return err
	}
	return nil
}

// vanadiumReleaseCandidateSnapshot takes a snapshot of the current V23_ROOT and
// writes the symlink target (the relative path to V23_ROOT) of that snapshot
// in the form of "<manifestEnvVar>=<symlinkTarget>" to
// "V23_ROOT/<snapshotManifestFile>".
func vanadiumReleaseCandidateSnapshot(ctx *tool.Context, testName string, opts ...Opt) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}

	cleanup, err := initTest(ctx, testName, []string{})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Take snapshot.
	args := []string{
		"snapshot",
		"--remote",
		"create",
		// TODO(jingjin): change this to use "date-rc<n>" format when the function is ready.
		"--time-format=2006-01-02.15:04",
		snapshotName,
	}
	if err := ctx.Run().Command("v23", args...); err != nil {
		return nil, internalTestError{err, "Snapshot"}
	}

	// Get the symlink target of the newly created snapshot manifest.
	snapshotDir, err := util.RemoteSnapshotDir()
	if err != nil {
		return nil, err
	}
	symlink := filepath.Join(snapshotDir, snapshotName)
	target, err := filepath.EvalSymlinks(symlink)
	if err != nil {
		return nil, internalTestError{fmt.Errorf("EvalSymlinks(%s) failed: %v", symlink, err), "Resolve Snapshot Symlink"}
	}

	// Get manifest file's relative path to the root manifest dir.
	manifestDir, err := util.ManifestDir()
	if err != nil {
		return nil, err
	}
	relativePath := strings.TrimPrefix(target, manifestDir+string(filepath.Separator))

	// Get all the tests to run.
	config, err := util.LoadConfig(ctx)
	if err != nil {
		return nil, internalTestError{err, "LoadConfig"}
	}
	tests := config.GroupTests([]string{"go", "java", "javascript", "projects", "third_party-go"})

	// Write to the properties file.
	content := fmt.Sprintf("%s=%s\n%s=%s", manifestEnvVar, relativePath, testsEnvVar, strings.Join(tests, " "))
	if err := ctx.Run().WriteFile(filepath.Join(root, propertiesFile), []byte(content), os.FileMode(0644)); err != nil {
		return nil, internalTestError{err, "Record Properties"}
	}

	return &test.Result{Status: test.Passed}, nil
}
