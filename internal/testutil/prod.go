// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"time"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/devtools/internal/xunit"
)

// generateXUnitTestSuite generates an xUnit test suite that
// encapsulates the given input.
func generateXUnitTestSuite(ctx *tool.Context, success bool, pkg string, duration time.Duration, output string) *xunit.TestSuite {
	// Generate an xUnit test suite describing the result.
	s := xunit.TestSuite{Name: pkg}
	c := xunit.TestCase{
		Classname: pkg,
		Name:      "Test",
		Time:      fmt.Sprintf("%.2f", duration.Seconds()),
	}
	if !success {
		fmt.Fprintf(ctx.Stdout(), "%s ... failed\n%v\n", pkg, output)
		f := xunit.Failure{
			Message: "vrpc",
			Data:    output,
		}
		c.Failures = append(c.Failures, f)
		s.Failures++
	} else {
		fmt.Fprintf(ctx.Stdout(), "%s ... ok\n", pkg)
	}
	s.Tests++
	s.Cases = append(s.Cases, c)
	return &s
}

// testProdService test the given production service.
func testProdService(ctx *tool.Context, service prodService) (*xunit.TestSuite, error) {
	root, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}
	bin := filepath.Join(root, "release", "go", "bin", "vrpc")
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	start := time.Now()
	if err := ctx.Run().TimedCommandWithOpts(DefaultTestTimeout, opts, bin, "signature", service.objectName); err != nil {
		return generateXUnitTestSuite(ctx, false, service.name, time.Now().Sub(start), out.String()), nil
	}
	if !service.regexp.Match(out.Bytes()) {
		fmt.Fprintf(ctx.Stderr(), "couldn't match regexp `%s` in output:\n%v\n", service.regexp, out.String())
		return generateXUnitTestSuite(ctx, false, service.name, time.Now().Sub(start), "mismatching signature"), nil
	}
	return generateXUnitTestSuite(ctx, true, service.name, time.Now().Sub(start), ""), nil
}

type prodService struct {
	name       string
	objectName string
	regexp     *regexp.Regexp
}

// vanadiumProdServicesTest runs a test of vanadium production services.
func vanadiumProdServicesTest(ctx *tool.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, nil)
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Install the vrpc tool.
	if err := ctx.Run().Command("v23", "go", "install", "v.io/x/ref/cmd/vrpc"); err != nil {
		return nil, internalTestError{err, "Install VRPC"}
	}

	// Describe the test cases.
	namespaceRoot := "/ns.dev.v.io:8101"
	allPassed, suites := true, []xunit.TestSuite{}
	services := []prodService{
		prodService{
			name:       "mounttable",
			objectName: namespaceRoot,
			regexp:     regexp.MustCompile(`MountTable[[:space:]]+interface`),
		},
		prodService{
			name:       "application repository",
			objectName: namespaceRoot + "/applications",
			regexp:     regexp.MustCompile(`Application[[:space:]]+interface`),
		},
		prodService{
			name:       "binary repository",
			objectName: namespaceRoot + "/binaries",
			regexp:     regexp.MustCompile(`Binary[[:space:]]+interface`),
		},
		prodService{
			name:       "macaroon service",
			objectName: namespaceRoot + "/identity/dev.v.io/root/macaroon",
			regexp:     regexp.MustCompile(`MacaroonBlesser[[:space:]]+interface`),
		},
		prodService{
			name:       "google identity service",
			objectName: namespaceRoot + "/identity/dev.v.io/root/google",
			regexp:     regexp.MustCompile(`OAuthBlesser[[:space:]]+interface`),
		},
		prodService{
			name:       "binary discharger",
			objectName: namespaceRoot + "/identity/dev.v.io/root/discharger",
			regexp:     regexp.MustCompile(`Discharger[[:space:]]+interface`),
		},
	}

	for _, service := range services {
		suite, err := testProdService(ctx, service)
		if err != nil {
			return nil, err
		}
		allPassed = allPassed && (suite.Failures == 0)
		suites = append(suites, *suite)
	}

	// Create the xUnit report.
	if err := xunit.CreateReport(ctx, testName, suites); err != nil {
		return nil, err
	}
	if !allPassed {
		return &TestResult{Status: TestFailed}, nil
	}
	return &TestResult{Status: TestPassed}, nil
}
