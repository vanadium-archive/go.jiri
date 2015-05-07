// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"
	"time"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/runutil"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

const (
	defaultWWWTestTimeout = 5 * time.Minute
)

// Runs specified make target in WWW Makefile as a test.
func commonVanadiumWWW(ctx *tool.Context, testName, makeTarget string, timeout time.Duration) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"web"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	wwwDir := filepath.Join(root, "www")
	if err := ctx.Run().Chdir(wwwDir); err != nil {
		return nil, err
	}

	if err := ctx.Run().Command("make", "clean"); err != nil {
		return nil, err
	}

	// Invoke the make target.
	if err := ctx.Run().TimedCommand(timeout, "make", makeTarget); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: timeout,
			}, nil
		} else {
			return nil, internalTestError{err, "Make " + makeTarget}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumWWWSite(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(ctx, testName, "test-site", defaultWWWTestTimeout)
}

func vanadiumWWWTutorials(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(ctx, testName, "test-tutorials", defaultWWWTestTimeout)
}

func vanadiumWWWTutorialsUI(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(ctx, testName, "test-tutorials-ui", defaultWWWTestTimeout)
}
