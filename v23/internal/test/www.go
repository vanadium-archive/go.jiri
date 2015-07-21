// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"path/filepath"
	"strings"
	"time"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/runutil"
	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

const (
	defaultWWWTestTimeout = 10 * time.Minute
)

// Runs specified make target in WWW Makefile as a test.
func commonVanadiumWWW(ctx *tool.Context, testName, makeTarget string, timeout time.Duration, extraDeps []string) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, append([]string{"nodejs", "syncbase"}, extraDeps...))
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
	return commonVanadiumWWW(ctx, testName, "test-site", defaultWWWTestTimeout, nil)
}

func vanadiumWWWTutorialsCore(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(ctx, testName, "test-tutorials-core", defaultWWWTestTimeout, nil)
}

func vanadiumWWWTutorialsExternal(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(ctx, testName, "test-tutorials-external", defaultWWWTestTimeout, nil)
}

func vanadiumWWWTutorialsJava(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(ctx, testName, "test-tutorials-java", defaultWWWTestTimeout, []string{"java"})
}

func vanadiumWWWTutorialsJSNode(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(ctx, testName, "test-tutorials-js-node", defaultWWWTestTimeout, nil)
}

func vanadiumWWWTutorialsJSWeb(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(ctx, testName, "test-tutorials-js-web", defaultWWWTestTimeout, nil)
}

func vanadiumWWWDeployStaging(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(ctx, testName, "deploy-staging", defaultWWWTestTimeout, nil)
}

func vanadiumWWWDeployProduction(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	return commonVanadiumWWW(ctx, testName, "deploy-production", defaultWWWTestTimeout, nil)
}

// vanadiumWWWConfigDeployHelper updates remote instance configuration and restarts remote nginx, auth, and proxy services.
func vanadiumWWWConfigDeployHelper(ctx *tool.Context, testName string, env string, _ ...Opt) (_ *test.Result, e error) {
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Change dir to infrastructure/nginx.
	root, err := util.V23Root()
	if err != nil {
		return nil, internalTestError{err, "V23Root"}
	}

	dir := filepath.Join(root, "infrastructure", "nginx")
	if err := ctx.Run().Chdir(dir); err != nil {
		return nil, internalTestError{err, "Chdir"}
	}

	// Update configuration.
	target := strings.Join([]string{"deploy", env}, "-")
	if err := ctx.Run().Command("make", target); err != nil {
		return &test.Result{Status: test.Failed}, nil
	}

	// Restart remote services.
	project := strings.Join([]string{"vanadium", env}, "-")
	if err := ctx.Run().Command("./restart.sh", project); err != nil {
		return &test.Result{Status: test.Failed}, nil
	}

	return &test.Result{Status: test.Passed}, nil
}

func vanadiumWWWConfigDeployProduction(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumWWWConfigDeployHelper(ctx, testName, "production")
}
func vanadiumWWWConfigDeployStaging(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	return vanadiumWWWConfigDeployHelper(ctx, testName, "staging")
}
