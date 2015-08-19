// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/retry"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/xunit"
)

// vanadiumBootstrap runs a test of Vanadium bootstrapping.
func vanadiumBootstrap(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Create a new temporary V23_ROOT.
	oldRoot := os.Getenv("V23_ROOT")
	defer collect.Error(func() error { return os.Setenv("V23_ROOT", oldRoot) }, &e)
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return nil, internalTestError{err, "TempDir"}
	}
	defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)

	root := filepath.Join(tmpDir, "root")
	if err := os.Setenv("V23_ROOT", root); err != nil {
		return nil, internalTestError{err, "Setenv"}
	}

	// Run the setup script.
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = io.MultiWriter(opts.Stdout, &out)
	opts.Stderr = io.MultiWriter(opts.Stderr, &out)
	// Find the PATH element containing the "v23" binary and remove it.
	v23Path, err := exec.LookPath("v23")
	if err != nil {
		return nil, internalTestError{err, "LookPath"}
	}
	opts.Env["PATH"] = strings.Replace(os.Getenv("PATH"), filepath.Dir(v23Path), "", -1)
	fn := func() error {
		return ctx.Run().CommandWithOpts(opts, filepath.Join(oldRoot, "www", "public", "bootstrap"))
	}
	if err := retry.Function(ctx, fn); err != nil {
		// Create xUnit report.
		if err := xunit.CreateFailureReport(ctx, testName, "VanadiumGo", "bootstrap", "Vanadium bootstrapping failed", out.String()); err != nil {
			return nil, err
		}
		return &test.Result{Status: test.Failed}, nil
	}
	return &test.Result{Status: test.Passed}, nil
}
