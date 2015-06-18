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
	"v.io/x/devtools/internal/xunit"
)

const (
	defaultJSTestTimeout = 15 * time.Minute
)

// runJSTest is a harness for executing javascript tests.
func runJSTest(ctx *tool.Context, testName, testDir, target string, cleanFn func() error, env map[string]string, extraDeps []string) (_ *test.Result, e error) {
	// Initialize the test.
	deps := append(extraDeps, "web")
	cleanup, err := initTest(ctx, testName, deps)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Navigate to the target directory.
	if err := ctx.Run().Chdir(testDir); err != nil {
		return nil, err
	}

	// Clean up after previous instances of the test.
	opts := ctx.Run().Opts()
	for key, value := range env {
		opts.Env[key] = value
	}
	if err := ctx.Run().CommandWithOpts(opts, "make", "clean"); err != nil {
		return nil, err
	}
	if cleanFn != nil {
		if err := cleanFn(); err != nil {
			return nil, err
		}
	}

	// Run the test target.
	if err := ctx.Run().TimedCommandWithOpts(defaultJSTestTimeout, opts, "make", target); err != nil {
		if err == runutil.CommandTimedOutErr {
			return &test.Result{
				Status:       test.TimedOut,
				TimeoutValue: defaultJSTestTimeout,
			}, nil
		} else {
			return nil, internalTestError{err, "Make " + target}
		}
	}

	return &test.Result{Status: test.Passed}, nil
}

// vanadiumJSBuildExtension tests the vanadium javascript build extension.
func vanadiumJSBuildExtension(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "extension/vanadium.zip"
	return runJSTest(ctx, testName, testDir, target, nil, nil, nil)
}

// vanadiumJSDoc (re)generates the content of the vanadium javascript
// documentation server.
func vanadiumJSDoc(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "docs"

	result, err := runJSTest(ctx, testName, testDir, target, nil, nil, nil)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// vanadiumJSBrowserIntegration runs the vanadium javascript integration test in a browser environment using nacl plugin.
func vanadiumJSBrowserIntegration(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-integration-browser"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["BROWSER_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env, nil)
}

// vanadiumJSNodeIntegration runs the vanadium javascript integration test in NodeJS environment using wspr.
func vanadiumJSNodeIntegration(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-integration-node"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env, nil)
}

// vanadiumJSUnit runs the vanadium javascript unit test.
func vanadiumJSUnit(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-unit"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env, nil)
}

// vanadiumJSVdl runs the vanadium javascript vdl test.
func vanadiumJSVdl(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-vdl"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env, nil)
}

// vanadiumJSVDLAudit checks that all VDL-based JS source files are up-to-date.
func vanadiumJSVdlAudit(ctx *tool.Context, testName string, _ ...Opt) (_ *test.Result, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-vdl-audit"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env, nil)
}

// vanadiumJSVom runs the vanadium javascript vom test.
func vanadiumJSVom(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "release", "javascript", "core")
	target := "test-vom"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env, nil)
}

// vanadiumJSSyncbaseBrowser runs the vanadium javascript syncbase test in a browser.
func vanadiumJSSyncbaseBrowser(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "roadmap", "javascript", "syncbase")
	target := "test-integration-browser"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["BROWSER_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env, []string{"syncbase"})
}

// vanadiumJSSyncbaseNode runs the vanadium javascript syncbase test in nodejs.
func vanadiumJSSyncbaseNode(ctx *tool.Context, testName string, _ ...Opt) (*test.Result, error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}
	testDir := filepath.Join(root, "roadmap", "javascript", "syncbase")
	target := "test-integration-node"
	env := map[string]string{}
	setCommonJSEnv(env)
	env["NODE_OUTPUT"] = xunit.ReportPath(testName)
	return runJSTest(ctx, testName, testDir, target, nil, env, []string{"syncbase"})
}

func setCommonJSEnv(env map[string]string) {
	env["XUNIT"] = "true"
}
