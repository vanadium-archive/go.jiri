// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"fmt"
	"os"
	"path/filepath"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

var (
	jenkinsHost = "http://localhost:8001/jenkins"
	netrcFile   = filepath.Join(os.Getenv("HOME"), ".netrc")
)

// requireEnv makes sure that the given environment variables are set.
func requireEnv(names []string) error {
	for _, name := range names {
		if os.Getenv(name) == "" {
			return fmt.Errorf("environment variable %q is not set", name)
		}
	}
	return nil
}

// vanadiumPresubmitPoll polls vanadium projects for new patchsets for
// which to run presubmit tests.
func vanadiumPresubmitPoll(ctx *tool.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Use the "presubmit query" command to poll for new changes.
	logfile := filepath.Join(root, ".presubmit_log")
	args := []string{}
	if ctx.Verbose() {
		args = append(args, "-v")
	}
	args = append(args,
		"-host", jenkinsHost,
		"-netrc", netrcFile,
		"query",
		"-log-file", logfile,
		"-manifest", "tools",
	)
	if err := ctx.Run().Command("presubmit", args...); err != nil {
		return nil, err
	}

	return &TestResult{Status: TestPassed}, nil
}

// vanadiumPresubmitTest runs presubmit tests for a given project specified
// in TEST environment variable.
func vanadiumPresubmitTest(ctx *tool.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	if err := requireEnv([]string{"BUILD_NUMBER", "REFS", "PROJECTS", "TEST", "WORKSPACE"}); err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Use the "presubmit test" command to run the presubmit test.
	args := []string{}
	if ctx.Verbose() {
		args = append(args, "-v")
	}
	test := os.Getenv("TEST")
	args = append(args,
		"-host", jenkinsHost,
		"-netrc", netrcFile,
		"test",
		"-build-number", os.Getenv("BUILD_NUMBER"),
		"-manifest", "tools",
		"-projects", os.Getenv("PROJECTS"),
		"-refs", os.Getenv("REFS"),
		"-test", test,
	)
	if err := ctx.Run().Command("presubmit", args...); err != nil {
		return nil, err
	}

	// Remove any test result files that are empty.
	testResultFiles, err := findTestResultFiles(ctx, test)
	if err != nil {
		return nil, err
	}
	for _, file := range testResultFiles {
		fileInfo, err := os.Stat(file)
		if err != nil {
			return nil, err
		}
		if fileInfo.Size() == 0 {
			if err := ctx.Run().RemoveAll(file); err != nil {
				return nil, err
			}
		}
	}

	return &TestResult{Status: TestPassed}, nil
}

// vanadiumPresubmitResult runs "presubmit result" command to process and post test resutls.
func vanadiumPresubmitResult(ctx *tool.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	if err := requireEnv([]string{"BUILD_NUMBER", "REFS", "PROJECTS", "WORKSPACE"}); err != nil {
		return nil, err
	}

	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Run "presubmit result".
	args := []string{}
	if ctx.Verbose() {
		args = append(args, "-v")
	}
	args = append(args,
		"-host", jenkinsHost,
		"-netrc", netrcFile,
		"result",
		"-build-number", os.Getenv("BUILD_NUMBER"),
		"-manifest", "tools",
		"-refs", os.Getenv("REFS"),
		"-projects", os.Getenv("PROJECTS"),
	)
	if err := ctx.Run().Command("presubmit", args...); err != nil {
		return nil, err
	}

	return &TestResult{Status: TestPassed}, nil
}
