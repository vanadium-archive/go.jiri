// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"v.io/x/devtools/internal/runutil"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/devtools/internal/xunit"
)

type TestStatus int

type TestResult struct {
	Status          TestStatus
	TimeoutValue    time.Duration       // Used when Status == TestTimedOut
	MergeConflictCL string              // Used when Status == TestFailedMergeConflict
	ExcludedTests   map[string][]string // Tests that are excluded within packages keyed by package name
	SkippedTests    map[string][]string // Tests that are skipped within packages keyed by package name
}

const (
	TestPending TestStatus = iota
	TestSkipped
	TestPassed
	TestFailed
	TestFailedMergeConflict
	TestTimedOut
)

const (
	// numLinesToOutput identifies the number of lines to be included in
	// the error messsage of an xUnit report.
	numLinesToOutput = 50
	// gsPrefix identifies the prefix of a Google Storage location where
	// test results are stored.
	gsPrefix = "gs://vanadium-test-results/v0/"
)

const (
	dummyTestResult = `<?xml version="1.0" encoding="utf-8"?>
<!--
  This file will be used to generate a dummy test results file
  in case the presubmit tests produce no test result files.
-->
<testsuites>
  <testsuite name="NO_TESTS" tests="1" errors="0" failures="0" skip="0">
    <testcase classname="NO_TESTS" name="NO_TESTS" time="0">
    </testcase>
  </testsuite>
</testsuites>
`
)

func (s TestStatus) String() string {
	switch s {
	case TestSkipped:
		return "SKIPPED"
	case TestPassed:
		return "PASSED"
	case TestFailed:
		return "FAILED"
	case TestFailedMergeConflict:
		return "MERGE CONFLICT"
	case TestTimedOut:
		return "TIMED OUT"
	default:
		return "UNKNOWN"
	}
}

// testNode represents a node of a test dependency graph.
type testNode struct {
	// deps is a list of its dependencies.
	deps []string
	// visited determines whether a DFS exploration of the test
	// dependency graph has visited this test.
	visited bool
	// stack determines whether this test is on the search stack
	// of a DFS exploration of the test dependency graph.
	stack bool
}

// testDepGraph captures the test dependency graph.
type testDepGraph map[string]*testNode

// DefaultTestTimeout identified the maximum time each test is allowed
// to run before being forcefully terminated.
var DefaultTestTimeout = 10 * time.Minute

var testMock = func(*tool.Context, string, ...TestOpt) (*TestResult, error) {
	return &TestResult{Status: TestPassed}, nil
}

var testFunctions = map[string]func(*tool.Context, string, ...TestOpt) (*TestResult, error){
	// TODO(jsimsa,cnicolaou): consider getting rid of the vanadium- prefix.
	"ignore-this":                     testMock,
	"third_party-go-build":            thirdPartyGoBuild,
	"third_party-go-test":             thirdPartyGoTest,
	"third_party-go-race":             thirdPartyGoRace,
	"vanadium-android-test":           vanadiumAndroidTest,
	"vanadium-android-build":          vanadiumAndroidBuild,
	"vanadium-bootstrap":              vanadiumBootstrap,
	"vanadium-chat-shell-test":        vanadiumChatShellTest,
	"vanadium-chat-web-test":          vanadiumChatWebTest,
	"vanadium-go-bench":               vanadiumGoBench,
	"vanadium-go-binaries":            vanadiumGoBinaries,
	"vanadium-go-build":               vanadiumGoBuild,
	"vanadium-go-cover":               vanadiumGoCoverage,
	"vanadium-go-doc":                 vanadiumGoDoc,
	"vanadium-go-generate":            vanadiumGoGenerate,
	"vanadium-go-race":                vanadiumGoRace,
	"vanadium-go-snapshot":            vanadiumGoSnapshot,
	"vanadium-go-test":                vanadiumGoTest,
	"vanadium-go-vdl":                 vanadiumGoVDL,
	"vanadium-go-rpc-stress":          vanadiumGoRPCStress,
	"vanadium-integration-test":       vanadiumIntegrationTest,
	"vanadium-js-build-extension":     vanadiumJSBuildExtension,
	"vanadium-js-doc":                 vanadiumJSDoc,
	"vanadium-js-browser-integration": vanadiumJSBrowserIntegration,
	"vanadium-js-node-integration":    vanadiumJSNodeIntegration,
	"vanadium-js-unit":                vanadiumJSUnit,
	"vanadium-js-vdl":                 vanadiumJSVdl,
	"vanadium-js-vom":                 vanadiumJSVom,
	"vanadium-namespace-browser-test": vanadiumNamespaceBrowserTest,
	"vanadium-pipe2browser-test":      vanadiumPipe2BrowserTest,
	"vanadium-playground-test":        vanadiumPlaygroundTest,
	"vanadium-postsubmit-poll":        vanadiumPostsubmitPoll,
	"vanadium-presubmit-poll":         vanadiumPresubmitPoll,
	"vanadium-presubmit-result":       vanadiumPresubmitResult,
	"vanadium-presubmit-test":         vanadiumPresubmitTest,
	"vanadium-prod-services-test":     vanadiumProdServicesTest,
	"vanadium-www-site":               vanadiumWWWSite,
	"vanadium-www-tutorials":          vanadiumWWWTutorials,
	"vanadium-github-mirror":          vanadiumGitHubMirror,
}

func newTestContext(ctx *tool.Context, env map[string]string) *tool.Context {
	tmpEnv := map[string]string{}
	for key, value := range ctx.Env() {
		tmpEnv[key] = value
	}
	for key, value := range env {
		tmpEnv[key] = value
	}
	return ctx.Clone(tool.ContextOpts{
		Env: tmpEnv,
	})
}

type TestOpt interface {
	TestOpt()
}

// PartOpt is an option that specifies which part of the test to run.
type PartOpt int

func (PartOpt) TestOpt() {}

// PrefixOpt is an option that specifies the location where to
// store test results.
type PrefixOpt string

func (PrefixOpt) TestOpt() {}

// PkgsOpt is an option that specifies which Go tests to run using a
// list of Go package expressions.
type PkgsOpt []string

func (PkgsOpt) TestOpt() {}

// ShortOpt is an option that specifies whether to run short tests
// only in VanadiumGoTest and VanadiumGoRace.
type ShortOpt bool

func (ShortOpt) TestOpt() {}

// RunProjectTests runs all tests associated with the given projects.
func RunProjectTests(ctx *tool.Context, env map[string]string, projects []string, opts ...TestOpt) (map[string]*TestResult, error) {
	testCtx := newTestContext(ctx, env)

	// Parse tests and dependencies from config file.
	config, err := util.LoadConfig(ctx)
	if err != nil {
		return nil, err
	}
	tests := config.ProjectTests(projects)
	if len(tests) == 0 {
		return nil, nil
	}
	sort.Strings(tests)
	graph, err := createTestDepGraph(config, tests)
	if err != nil {
		return nil, err
	}

	// Run tests.
	//
	// TODO(jingjin) parallelize the top-level scheduling loop so
	// that tests do not need to run serially.
	results := make(map[string]*TestResult, len(tests))
	for _, test := range tests {
		results[test] = &TestResult{}
	}
run:
	for i := 0; i < len(graph); i++ {
		// Find a test that can execute.
		for _, test := range tests {
			result, node := results[test], graph[test]
			if result.Status != TestPending {
				continue
			}
			ready := true
			for _, dep := range node.deps {
				switch results[dep].Status {
				case TestSkipped, TestFailed, TestTimedOut:
					results[test].Status = TestSkipped
					continue run
				case TestPending:
					ready = false
					break
				}
			}
			if !ready {
				continue
			}
			if err := runTests(testCtx, []string{test}, results, opts...); err != nil {
				return nil, err
			}
			continue run
		}
		// The following line should be never reached.
		return nil, fmt.Errorf("erroneous test running logic")
	}

	return results, nil
}

// RunTests executes the given tests and reports the test results.
func RunTests(ctx *tool.Context, env map[string]string, tests []string, opts ...TestOpt) (map[string]*TestResult, error) {
	results := make(map[string]*TestResult, len(tests))
	for _, test := range tests {
		results[test] = &TestResult{}
	}
	testCtx := newTestContext(ctx, env)
	if err := runTests(testCtx, tests, results, opts...); err != nil {
		return nil, err
	}
	return results, nil
}

// TestList returns a list of all tests known by the testutil package.
func TestList() ([]string, error) {
	result := []string{}
	for name := range testFunctions {
		if !strings.HasPrefix(name, "ignore") {
			result = append(result, name)
		}
	}
	sort.Strings(result)
	return result, nil
}

// runTests runs the given tests, populating the results map.
func runTests(ctx *tool.Context, tests []string, results map[string]*TestResult, opts ...TestOpt) error {
	path := ""
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case PrefixOpt:
			path = gsPrefix + string(typedOpt)
		}
	}

	for _, test := range tests {
		testFn, ok := testFunctions[test]
		if !ok {
			return fmt.Errorf("test %v does not exist", test)
		}
		fmt.Fprintf(ctx.Stdout(), "##### Running test %q #####\n", test)

		// Create a 1MB buffer to capture the test function output.
		var out bytes.Buffer
		const largeBufferSize = 1 << 20
		out.Grow(largeBufferSize)
		runOpts := ctx.Run().Opts()
		newCtx := ctx.Clone(tool.ContextOpts{
			Stdout: io.MultiWriter(&out, runOpts.Stdout),
			Stderr: io.MultiWriter(&out, runOpts.Stdout),
		})

		// Run the test and collect the test results.
		result, err := testFn(newCtx, test, opts...)
		if result != nil && result.Status == TestTimedOut {
			writeTimedOutTestReport(newCtx, test, *result)
		}
		if err == nil {
			err = checkTestReportFile(newCtx, test)
		}
		if err != nil {
			r, err := generateXUnitReportForError(newCtx, test, err, out.String())
			if err != nil {
				return err
			}
			result = r
		}
		if path != "" {
			if err := persistTestData(ctx, result, &out, test, path); err != nil {
				fmt.Fprintf(ctx.Stderr(), "failed to store test results: %v\n", err)
			}
		}
		results[test] = result
		fmt.Fprintf(ctx.Stdout(), "##### %s #####\n", results[test].Status)
	}
	return nil
}

// writeTimedOutTestReport writes a xUnit test report for the given timed-out test.
func writeTimedOutTestReport(ctx *tool.Context, testName string, result TestResult) {
	timeoutValue := DefaultTestTimeout
	if result.TimeoutValue != 0 {
		timeoutValue = result.TimeoutValue
	}
	errorMessage := fmt.Sprintf("The test timed out after %s.", timeoutValue)
	if err := xunit.CreateFailureReport(ctx, testName, testName, "Timeout", errorMessage, errorMessage); err != nil {
		fmt.Fprintf(ctx.Stderr(), "%v\n", err)
	}
}

// checkTestReportFile checks that the test report file exists, contains a
// valid xUnit test report, and the set of test cases is non-empty. If any of
// these is not true, the function generates a dummy test report file that
// meets these requirements.
func checkTestReportFile(ctx *tool.Context, testName string) error {
	// Skip the checks for presubmit-test itself.
	if testName == "vanadium-presubmit-test" {
		return nil
	}

	xUnitReportFile := xunit.ReportPath(testName)
	if _, err := os.Stat(xUnitReportFile); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// No test report.
		dummyFile, perm := filepath.Join(filepath.Dir(xUnitReportFile), "tests_dummy.xml"), os.FileMode(0644)
		if err := ctx.Run().WriteFile(dummyFile, []byte(dummyTestResult), perm); err != nil {
			return fmt.Errorf("WriteFile(%v) failed: %v", dummyFile, err)
		}
		return nil
	}

	// Invalid xUnit file.
	bytes, err := ioutil.ReadFile(xUnitReportFile)
	if err != nil {
		return fmt.Errorf("ReadFile(%s) failed: %v", xUnitReportFile, err)
	}
	var suites xunit.TestSuites
	if err := xml.Unmarshal(bytes, &suites); err != nil {
		ctx.Run().RemoveAll(xUnitReportFile)
		if err := xunit.CreateFailureReport(ctx, testName, testName, "Invalid xUnit Report", "Invalid xUnit Report", err.Error()); err != nil {
			return err
		}
		return nil
	}

	// No test cases.
	numTestCases := 0
	for _, suite := range suites.Suites {
		numTestCases += len(suite.Cases)
	}
	if numTestCases == 0 {
		ctx.Run().RemoveAll(xUnitReportFile)
		if err := xunit.CreateFailureReport(ctx, testName, testName, "No Test Cases", "No Test Cases", ""); err != nil {
			return err
		}
		return nil
	}

	return nil
}

// persistTestData uploads test data to Google Storage.
func persistTestData(ctx *tool.Context, result *TestResult, output *bytes.Buffer, test, path string) error {
	// Write test data to a temporary directory.
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return err
	}
	defer ctx.Run().RemoveAll(tmpDir)
	conf := struct {
		Arch string
		OS   string
	}{
		Arch: runtime.GOARCH,
		OS:   runtime.GOOS,
	}
	{
		bytes, err := json.Marshal(conf)
		if err != nil {
			return fmt.Errorf("Marshal(%v) failed: %v", err)
		}
		confFile := filepath.Join(tmpDir, "conf")
		if err := ctx.Run().WriteFile(confFile, bytes, os.FileMode(0600)); err != nil {
			return err
		}
	}
	{
		bytes, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("Marshal(%v) failed: %v", err)
		}
		resultFile := filepath.Join(tmpDir, "result")
		if err := ctx.Run().WriteFile(resultFile, bytes, os.FileMode(0600)); err != nil {
			return err
		}
	}
	outputFile := filepath.Join(tmpDir, "output")
	if err := ctx.Run().WriteFile(outputFile, output.Bytes(), os.FileMode(0600)); err != nil {
		return err
	}
	// Upload test data to Google Storage.
	{
		args := []string{"-q", "-m", "cp", filepath.Join(tmpDir, "*"), path + "/" + test}
		if err := ctx.Run().Command("gsutil", args...); err != nil {
			return err
		}
	}
	{
		xUnitFile := xunit.ReportPath(test)
		if _, err := os.Stat(xUnitFile); err == nil {
			args := []string{"-q", "cp", xUnitFile, path + "/" + test + "/" + "xunit.xml"}
			if err := ctx.Run().Command("gsutil", args...); err != nil {
				return err
			}
		} else {
			if !os.IsNotExist(err) {
				return fmt.Errorf("Stat(%v) failed: %v", xUnitFile, err)
			}
		}
	}
	return nil
}

// generateXUnitReportForError generates an xUnit test report for the
// given (internal) error.
func generateXUnitReportForError(ctx *tool.Context, test string, err error, output string) (*TestResult, error) {
	// Skip the report generation for presubmit-test itself.
	if test == "vanadium-presubmit-test" {
		return &TestResult{Status: TestPassed}, nil
	}

	xUnitFilePath := xunit.ReportPath(test)

	// Only create the report when the xUnit file doesn't exist, is
	// invalid, or exist but doesn't have failed test cases.
	createXUnitFile := false
	if _, err := os.Stat(xUnitFilePath); err != nil {
		if os.IsNotExist(err) {
			createXUnitFile = true
		} else {
			return nil, fmt.Errorf("Stat(%s) failed: %v", xUnitFilePath, err)
		}
	} else {
		bytes, err := ioutil.ReadFile(xUnitFilePath)
		if err != nil {
			return nil, fmt.Errorf("ReadFile(%s) failed: %v", xUnitFilePath, err)
		}
		var existingSuites xunit.TestSuites
		if err := xml.Unmarshal(bytes, &existingSuites); err != nil {
			createXUnitFile = true
		} else {
			createXUnitFile = true
			for _, curSuite := range existingSuites.Suites {
				if curSuite.Failures > 0 || curSuite.Errors > 0 {
					createXUnitFile = false
					break
				}
			}
		}
	}

	if createXUnitFile {
		errType := "Internal Error"
		if internalErr, ok := err.(internalTestError); ok {
			errType = internalErr.name
			err = internalErr.err
		}
		// Create a test suite to encapsulate the error. Include last
		// <numLinesToOutput> lines of the output in the error message.
		lines := strings.Split(output, "\n")
		startLine := int(math.Max(0, float64(len(lines)-numLinesToOutput)))
		consoleOutput := "......\n" + strings.Join(lines[startLine:], "\n")
		errMsg := fmt.Sprintf("Error message:\n%s:\n%s\n\n\nConsole output:\n%s\n", errType, err.Error(), consoleOutput)
		if err := xunit.CreateFailureReport(ctx, test, test, errType, errType, errMsg); err != nil {
			return nil, err
		}

		if err == runutil.CommandTimedOutErr {
			return &TestResult{Status: TestTimedOut}, nil
		}
	}
	return &TestResult{Status: TestFailed}, nil
}

// createTestDepGraph creates a test dependency graph given a map of
// dependencies and a list of tests.
func createTestDepGraph(config *util.Config, tests []string) (testDepGraph, error) {
	// For the given list of tests, build a map from the test name
	// to its testInfo object using the dependency data extracted
	// from the given dependency config data "dep".
	depGraph := testDepGraph{}
	for _, test := range tests {
		// Make sure the test dependencies are included in <tests>.
		deps := []string{}
		for _, curDep := range config.TestDependencies(test) {
			isDepInTests := false
			for _, test := range tests {
				if curDep == test {
					isDepInTests = true
					break
				}
			}
			if isDepInTests {
				deps = append(deps, curDep)
			}
		}
		depGraph[test] = &testNode{
			deps: deps,
		}
	}

	// Detect dependency loop using depth-first search.
	for name, info := range depGraph {
		if info.visited {
			continue
		}
		if findCycle(name, depGraph) {
			return nil, fmt.Errorf("found dependency loop: %v", depGraph)
		}
	}
	return depGraph, nil
}

// findCycle checks whether there are any cycles in the test
// dependency graph.
func findCycle(name string, depGraph testDepGraph) bool {
	node := depGraph[name]
	node.visited = true
	node.stack = true
	for _, dep := range node.deps {
		depNode := depGraph[dep]
		if depNode.stack {
			return true
		}
		if depNode.visited {
			continue
		}
		if findCycle(dep, depGraph) {
			return true
		}
	}
	node.stack = false
	return false
}
