// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/devtools/internal/xunit"
)

// vanadiumGoVDL checks that all VDL-based Go source files are
// up-to-date.
func vanadiumGoVDL(ctx *tool.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	fmt.Fprintf(ctx.Stdout(), "NOTE: This test checks that all VDL-based Go source files are up-to-date.\nIf it fails, you probably just need to run 'v23 run vdl generate --lang=go all'.\n")

	root, err := util.V23Root()
	if err != nil {
		return nil, err
	}

	cleanup, err := initTest(ctx, testName, []string{})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install the vdl tool.
	if err := ctx.Run().Command("v23", "go", "install", "v.io/x/ref/cmd/vdl"); err != nil {
		return nil, internalTestError{err, "Install VDL"}
	}

	// Check that "vdl audit --lang=go all" produces no output.
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	venv, err := util.VanadiumEnvironment(ctx, util.HostPlatform())
	if err != nil {
		return nil, err
	}
	opts.Env["VDLPATH"] = venv.Get("VDLPATH")
	vdl := filepath.Join(root, "release", "go", "bin", "vdl")
	err = ctx.Run().CommandWithOpts(opts, vdl, "audit", "--lang=go", "all")
	output := strings.TrimSpace(out.String())
	if err != nil || len(output) != 0 {
		fmt.Fprintf(ctx.Stdout(), "%v\n", output)
		// Create xUnit report.
		files := strings.Split(output, "\n")
		suites := []xunit.TestSuite{}
		for _, file := range files {
			s := xunit.CreateTestSuiteWithFailure("VDLAudit", file, "VDL audit failure", "Outdated file:\n"+file, 0)
			suites = append(suites, *s)
		}
		if err := xunit.CreateReport(ctx, testName, suites); err != nil {
			return nil, err
		}
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}
