// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package testutil

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/xunit"
)

func init() {
	// Prevent the initTest() function from cleaning up Go object
	// files and binaries to avoid interference with concurrently
	// running tests.
	cleanGo = false
}

// caseMatch checks whether the given test cases match modulo their
// execution time.
func caseMatch(c1, c2 xunit.TestCase) bool {

	// Test names can have a CPU count appended to them (e.g. TestFoo-12)
	// so we take care to strip that out when comparing with
	// expected results.
	ncpu := runtime.NumCPU()
	re := regexp.MustCompile(fmt.Sprintf("(.*)-%d(.*)", ncpu))
	stripNumCPU := func(s string) string {
		parts := re.FindStringSubmatch(s)
		switch len(parts) {
		case 3:
			return strings.TrimRight(parts[1]+parts[2], " ")
		default:
			return s
		}
	}

	if stripNumCPU(c1.Name) != stripNumCPU(c2.Name) {
		return false
	}
	if c1.Classname != c2.Classname {
		return false
	}
	if !reflect.DeepEqual(c1.Errors, c2.Errors) {
		return false
	}
	if !reflect.DeepEqual(c1.Failures, c2.Failures) {
		return false
	}
	return true
}

// coverageMatch checks whether the given test coverages match modulo
// their timestamps and sources.
func coverageMatch(c1, c2 testCoverage) bool {
	if c1.BranchRate != c2.BranchRate {
		return false
	}
	if c1.LineRate != c2.LineRate {
		return false
	}
	if !reflect.DeepEqual(c1.Packages, c2.Packages) {
		return false
	}
	return true
}

// suiteMatch checks whether the given test suites match modulo their
// execution time.
func suiteMatch(s1, s2 xunit.TestSuite) bool {
	if s1.Name != s2.Name {
		return false
	}
	if s1.Errors != s2.Errors {
		return false
	}
	if s1.Failures != s2.Failures {
		return false
	}
	if s1.Skip != s2.Skip {
		return false
	}
	if s1.Tests != s2.Tests {
		return false
	}
	if len(s1.Cases) != len(s2.Cases) {
		return false
	}
	for i := 0; i < len(s1.Cases); i++ {
		found := false
		for j := 0; j < len(s2.Cases); j++ {
			if caseMatch(s1.Cases[i], s2.Cases[j]) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// suitesMatch checks whether the given test suites match modulo their
// execution time.
func suitesMatch(ss1, ss2 xunit.TestSuites) bool {
	if len(ss1.Suites) != len(ss2.Suites) {
		return false
	}
	for i := 0; i < len(ss1.Suites); i++ {
		if !suiteMatch(ss1.Suites[i], ss2.Suites[i]) {
			return false
		}
	}
	return true
}

var (
	wantBuild = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v_io.x/devtools/internal/testutil/testdata/foo",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v_io.x/devtools/internal/testutil/testdata/foo",
						Name:      "Build",
					},
				},
				Tests: 1,
			},
		},
	}
	wantTest = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v_io.x/devtools/internal/testutil/testdata/foo",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v_io.x/devtools/internal/testutil/testdata/foo",
						Name:      "Test1",
					},
					xunit.TestCase{
						Classname: "v_io.x/devtools/internal/testutil/testdata/foo",
						Name:      "Test2",
					},
					xunit.TestCase{
						Classname: "v_io.x/devtools/internal/testutil/testdata/foo",
						Name:      "Test3",
					},
					xunit.TestCase{
						Classname: "v_io.x/devtools/internal/testutil/testdata/foo",
						Name:      "TestV23",
					},
				},
				Tests: 4,
				Skip:  1,
			},
		},
	}
	wantV23Test = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v_io.x/devtools/internal/testutil/testdata/foo",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v_io.x/devtools/internal/testutil/testdata/foo",
						Name:      "TestV23",
					},
				},
				Tests: 1,
				Skip:  0,
			},
		},
	}
	wantTestWithSuffix = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v_io.x/devtools/internal/testutil/testdata/foo",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v_io.x/devtools/internal/testutil/testdata/foo",
						Name:      "Test1 [Suffix]",
					},
					xunit.TestCase{
						Classname: "v_io.x/devtools/internal/testutil/testdata/foo",
						Name:      "Test2 [Suffix]",
					},
					xunit.TestCase{
						Classname: "v_io.x/devtools/internal/testutil/testdata/foo",
						Name:      "Test3 [Suffix]",
					},
					xunit.TestCase{
						Classname: "v_io.x/devtools/internal/testutil/testdata/foo",
						Name:      "TestV23 [Suffix]",
					},
				},
				Tests: 4,
				Skip:  1,
			},
		},
	}
	wantTestWithExcludedTests = xunit.TestSuites{
		Suites: []xunit.TestSuite{
			xunit.TestSuite{
				Name: "v_io.x/devtools/internal/testutil/testdata/foo",
				Cases: []xunit.TestCase{
					xunit.TestCase{
						Classname: "v_io.x/devtools/internal/testutil/testdata/foo",
						Name:      "Test1",
					},
					xunit.TestCase{
						Classname: "v_io.x/devtools/internal/testutil/testdata/foo",
						Name:      "TestV23",
					},
				},
				Tests: 2,
				Skip:  1,
			},
		},
	}
	wantExcludedPackage = xunit.TestSuites{
		Suites: []xunit.TestSuite{},
	}
	wantCoverage = testCoverage{
		LineRate:   0,
		BranchRate: 0,
		Packages: []testCoveragePkg{
			testCoveragePkg{
				Name:       "v.io/x/devtools/internal/testutil/testdata/foo",
				LineRate:   0,
				BranchRate: 0,
				Complexity: 0,
				Classes: []testCoverageClass{
					testCoverageClass{
						Name:       "-",
						Filename:   "v.io/x/devtools/internal/testutil/testdata/foo/foo.go",
						LineRate:   0,
						BranchRate: 0,
						Complexity: 0,
						Methods: []testCoverageMethod{
							testCoverageMethod{
								Name:       "Foo",
								LineRate:   0,
								BranchRate: 0,
								Signature:  "",
								Lines: []testCoverageLine{
									testCoverageLine{Number: 7, Hits: 1},
									testCoverageLine{Number: 8, Hits: 1},
									testCoverageLine{Number: 9, Hits: 1},
								},
							},
						},
					},
				},
			},
		},
	}
)

// TestGoBuild checks the Go build based test logic.
func TestGoBuild(t *testing.T) {
	ctx := tool.NewDefaultContext()

	testName, pkgName := "test-go-build", "v.io/x/devtools/internal/testutil/testdata/foo"
	result, err := goBuild(ctx, testName, pkgsOpt([]string{pkgName}))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := result.Status, TestPassed; got != want {
		t.Fatalf("unexpected result: got %s, want %s", got, want)
	}

	// Check the xUnit report.
	xUnitFile := xunit.ReportPath(testName)
	data, err := ioutil.ReadFile(xUnitFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", xUnitFile, err)
	}
	defer os.RemoveAll(xUnitFile)
	var gotBuild xunit.TestSuites
	if err := xml.Unmarshal(data, &gotBuild); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, string(data))
	}
	if !suitesMatch(gotBuild, wantBuild) {
		t.Fatalf("unexpected result:\ngot\n%v\nwant\n%v", gotBuild, wantBuild)
	}
}

// TestGoCoverage checks the Go test coverage based test logic.
func TestGoCoverage(t *testing.T) {
	ctx := tool.NewDefaultContext()

	testName, pkgName := "test-go-coverage", "v.io/x/devtools/internal/testutil/testdata/foo"
	result, err := goCoverage(ctx, testName, pkgsOpt([]string{pkgName}))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := result.Status, TestPassed; got != want {
		t.Fatalf("unexpected result: got %s, want %s", got, want)
	}

	// Check the xUnit report.
	xUnitFile := xunit.ReportPath(testName)
	data, err := ioutil.ReadFile(xUnitFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", xUnitFile, err)
	}
	defer os.RemoveAll(xUnitFile)
	var gotTest xunit.TestSuites
	if err := xml.Unmarshal(data, &gotTest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, string(data))
	}
	if !suitesMatch(gotTest, wantTest) {
		t.Fatalf("unexpected result:\ngot\n%v\nwant\n%v", gotTest, wantTest)
	}

	// Check the cobertura report.
	coberturaFile := coberturaReportPath(testName)
	data, err = ioutil.ReadFile(coberturaFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", coberturaFile, err)
	}
	var gotCoverage testCoverage
	if err := xml.Unmarshal(data, &gotCoverage); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, string(data))
	}
	if !coverageMatch(gotCoverage, wantCoverage) {
		t.Fatalf("unexpected result:\ngot\n%v\nwant\n%v", gotCoverage, wantCoverage)
	}
}

// TestGoTest checks the Go test based test logic.
func TestGoTest(t *testing.T) {
	runGoTest(t, "", nil, wantTest)
}

// TestGoTestWithSuffix checks the suffix mode of Go test based test
// logic.
func TestGoTestWithSuffix(t *testing.T) {
	runGoTest(t, "[Suffix]", nil, wantTestWithSuffix)
}

// TestGoTestWithExcludedTests checks the excluded test mode of Go
// test based test logic.
func TestGoTestWithExcludedTests(t *testing.T) {
	tests := []exclusion{
		exclusion{test{pkg: "v.io/x/devtools/internal/testutil/testdata/foo", name: "Test2"}, true},
		exclusion{test{pkg: "v.io/x/devtools/internal/testutil/testdata/foo", name: "Test3"}, true},
	}
	exclusions, err := excludedTests(tests)
	if err != nil {
		t.Fatalf("%v", err)
	}
	runGoTest(t, "", exclusions, wantTestWithExcludedTests)
}

func TestGoTestWithExcludedTestsWithWildcards(t *testing.T) {
	tests := []exclusion{
		exclusion{test{pkg: "v.io/x/devtools/internal/testutil/testdata/foo", name: "Test[23]$"}, true},
	}
	exclusions, err := excludedTests(tests)
	if err != nil {
		t.Fatalf("%v", err)
	}
	runGoTest(t, "", exclusions, wantTestWithExcludedTests)
}

func TestGoTestExcludedPackage(t *testing.T) {
	tests := []exclusion{
		exclusion{test{pkg: "v.io/x/devtools/internal/testutil/testdata/foo", name: ".*"}, true},
	}
	exclusions, err := excludedTests(tests)
	if err != nil {
		t.Fatalf("%v", err)
	}
	runGoTest(t, "", exclusions, wantExcludedPackage)
}

func TestGoTestV23(t *testing.T) {
	runGoTest(t, "", nil, wantV23Test, argsOpt{"--run=TestV23"}, nonTestArgsOpt([]string{"--v23.tests"}))
}

func runGoTest(t *testing.T, suffix string, excludedTests []test, expectedTestSuite xunit.TestSuites, testOpts ...goTestOpt) {
	ctx := tool.NewDefaultContext()

	testName, pkgName := "test-go-test", "v.io/x/devtools/internal/testutil/testdata/foo"

	opts := []goTestOpt{
		pkgsOpt([]string{pkgName}),
		suffixOpt(suffix),
		excludedTestsOpt(excludedTests)}
	opts = append(opts, testOpts...)

	result, err := goTest(ctx, testName, opts...)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := result.Status, TestPassed; got != want {
		t.Fatalf("unexpected result: got %s, want %s", got, want)
	}

	// Check the xUnit report.
	xUnitFile := xunit.ReportPath(testName)
	data, err := ioutil.ReadFile(xUnitFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", xUnitFile, err)
	}
	defer os.RemoveAll(xUnitFile)
	var gotTest xunit.TestSuites
	fmt.Fprintf(os.Stderr, "XML: %s\n", data)
	if err := xml.Unmarshal(data, &gotTest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, string(data))
	}
	if !suitesMatch(gotTest, expectedTestSuite) {
		t.Fatalf("unexpected result:\ngot\n%v\nwant\n%v", gotTest, expectedTestSuite)
	}
}
