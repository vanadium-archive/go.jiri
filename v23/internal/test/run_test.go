// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/devtools/internal/xunit"
)

func TestProjectTests(t *testing.T) {
	config := util.NewConfig(util.ProjectTestsOpt(map[string][]string{
		"vanadium": []string{"vanadium-go-build", "vanadium-go-test"},
		"default":  []string{"tools-go-build", "tools-go-test"},
	}))

	// Get tests for a project that is in the config file.
	got := config.ProjectTests([]string{"vanadium"})
	expected := []string{
		"vanadium-go-build",
		"vanadium-go-test",
	}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %v, got: %v", expected, got)
	}

	// Get tests for a project that is NOT in the config file.
	// This should return empty tests.
	got, expected = config.ProjectTests([]string{"non-exist-project"}), nil
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %#v, got: %#v", expected, got)
	}
}

func TestGenXUnitReportForError(t *testing.T) {
	ctx := tool.NewDefaultContext()

	// Set WORKSPACE to a tmp dir.
	workspaceDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ctx.Run().RemoveAll(workspaceDir)
	oldWorkspaceDir := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", workspaceDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("WORKSPACE", oldWorkspaceDir)

	expectedGenSuite := xunit.TestSuite{
		Name: "vanadium-go-test",
		Cases: []xunit.TestCase{
			xunit.TestCase{
				Name:      "Init",
				Classname: "vanadium-go-test",
				Failures: []xunit.Failure{
					xunit.Failure{
						Message: "Init",
						Data:    "Error message:\nInit:\nsomething is wrong\n\n\nConsole output:\n......\noutput message\n",
					},
				},
				Time: "0.00",
			},
		},
		Tests:    1,
		Failures: 1,
	}
	aFailedTestSuite := xunit.TestSuite{
		Name: "name1",
		Cases: []xunit.TestCase{
			xunit.TestCase{
				Name:      "test1",
				Classname: "class1",
				Failures: []xunit.Failure{
					xunit.Failure{
						Message: "failure",
						Data:    "test failed",
					},
				},
				Time: "0.10",
			},
		},
		Tests:    1,
		Failures: 1,
	}

	// Tests.
	testCases := []struct {
		createXUnitFile bool
		existingSuites  *xunit.TestSuites
		expectedSuites  *xunit.TestSuites
	}{
		// No xUnit file exists.
		{
			createXUnitFile: false,
			expectedSuites: &xunit.TestSuites{
				Suites: []xunit.TestSuite{expectedGenSuite},
				XMLName: xml.Name{
					Local: "testsuites",
				},
			},
		},
		// xUnit file exists but empty (invalid).
		{
			createXUnitFile: true,
			existingSuites:  &xunit.TestSuites{},
			expectedSuites: &xunit.TestSuites{
				Suites: []xunit.TestSuite{expectedGenSuite},
				XMLName: xml.Name{
					Local: "testsuites",
				},
			},
		},
		// xUnit file exists but doesn't contain failed test cases.
		{
			createXUnitFile: true,
			existingSuites: &xunit.TestSuites{
				Suites: []xunit.TestSuite{
					xunit.TestSuite{
						Name: "name1",
						Cases: []xunit.TestCase{
							xunit.TestCase{
								Name:      "test1",
								Classname: "class1",
								Time:      "0.10",
							},
						},
						Tests:    1,
						Failures: 0,
					},
				},
			},
			expectedSuites: &xunit.TestSuites{
				Suites: []xunit.TestSuite{expectedGenSuite},
				XMLName: xml.Name{
					Local: "testsuites",
				},
			},
		},
		// xUnit file exists and contains failed test cases.
		{
			createXUnitFile: true,
			existingSuites: &xunit.TestSuites{
				Suites: []xunit.TestSuite{aFailedTestSuite},
			},
			expectedSuites: &xunit.TestSuites{
				Suites: []xunit.TestSuite{aFailedTestSuite},
				XMLName: xml.Name{
					Local: "testsuites",
				},
			},
		},
	}

	xUnitFileName := xunit.ReportPath("vanadium-go-test")
	internalErr := internalTestError{fmt.Errorf("something is wrong"), "Init"}
	for _, testCase := range testCases {
		if err := os.RemoveAll(xUnitFileName); err != nil {
			t.Fatalf("RemoveAll(%s) failed: %v", xUnitFileName, err)
		}
		if testCase.createXUnitFile && testCase.existingSuites != nil {
			bytes, err := xml.MarshalIndent(testCase.existingSuites, "", "  ")
			if err != nil {
				t.Fatalf("MarshalIndent(%v) failed: %v", testCase.existingSuites, err)
			}
			if err := ioutil.WriteFile(xUnitFileName, bytes, os.FileMode(0644)); err != nil {
				t.Fatalf("WriteFile(%v) failed: %v", xUnitFileName, err)
			}
		}
		testResult, err := generateXUnitReportForError(ctx, "vanadium-go-test", internalErr, "output message")
		if err != nil {
			t.Fatalf("want no errors, got %v", err)
		}
		gotSuites, err := parseXUnitFile(xUnitFileName)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !reflect.DeepEqual(gotSuites, testCase.expectedSuites) {
			t.Fatalf("want\n%#v\n\ngot\n%#v", testCase.expectedSuites, gotSuites)
		}
		if got, expected := testResult.Status, test.Failed; got != expected {
			t.Fatalf("want %v, got %v", expected, got)
		}
	}
}

func parseXUnitFile(fileName string) (*xunit.TestSuites, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, fmt.Errorf("ReadFile(%s) failed: %v", fileName, err)
	}
	var s xunit.TestSuites
	if err := xml.Unmarshal(bytes, &s); err != nil {
		return nil, fmt.Errorf("Unmarshal() failed: %v\n%v", err, string(bytes))
	}
	return &s, nil
}

func TestCreateDepGraph(t *testing.T) {
	type testCase struct {
		config        *util.Config
		tests         []string
		expectedTests testDepGraph
		expectDepLoop bool
	}
	testCases := []testCase{
		// A single test without any dependencies.
		testCase{
			config: util.NewConfig(util.TestDependenciesOpt(map[string][]string{"A": []string{}})),
			tests:  []string{"A"},
			expectedTests: testDepGraph{
				"A": &testNode{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// A -> B
		testCase{
			config: util.NewConfig(util.TestDependenciesOpt(map[string][]string{"A": []string{"B"}})),
			tests:  []string{"A", "B"},
			expectedTests: testDepGraph{
				"A": &testNode{
					deps:    []string{"B"},
					visited: true,
				},
				"B": &testNode{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// A -> {B, C, D}
		testCase{
			config: util.NewConfig(util.TestDependenciesOpt(map[string][]string{"A": []string{"B", "C", "D"}})),
			tests:  []string{"A", "B", "C", "D"},
			expectedTests: testDepGraph{
				"A": &testNode{
					deps:    []string{"B", "C", "D"},
					visited: true,
				},
				"B": &testNode{
					deps:    []string{},
					visited: true,
				},
				"C": &testNode{
					deps:    []string{},
					visited: true,
				},
				"D": &testNode{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// Same as above, but "dep" has no data.
		testCase{
			config: util.NewConfig(),
			tests:  []string{"A", "B", "C", "D"},
			expectedTests: testDepGraph{
				"A": &testNode{
					deps:    []string{},
					visited: true,
				},
				"B": &testNode{
					deps:    []string{},
					visited: true,
				},
				"C": &testNode{
					deps:    []string{},
					visited: true,
				},
				"D": &testNode{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// A -> {B, C, D}, but A is the only given test to resolve dependency for.
		testCase{
			config: util.NewConfig(util.TestDependenciesOpt(map[string][]string{"A": []string{"B", "C", "D"}})),
			tests:  []string{"A"},
			expectedTests: testDepGraph{
				"A": &testNode{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// A -> {B, C, D} -> E
		testCase{
			config: util.NewConfig(util.TestDependenciesOpt(map[string][]string{
				"A": []string{"B", "C", "D"},
				"B": []string{"E"},
				"C": []string{"E"},
				"D": []string{"E"},
			})),
			tests: []string{"A", "B", "C", "D", "E"},
			expectedTests: testDepGraph{
				"A": &testNode{
					deps:    []string{"B", "C", "D"},
					visited: true,
				},
				"B": &testNode{
					deps:    []string{"E"},
					visited: true,
				},
				"C": &testNode{
					deps:    []string{"E"},
					visited: true,
				},
				"D": &testNode{
					deps:    []string{"E"},
					visited: true,
				},
				"E": &testNode{
					deps:    []string{},
					visited: true,
				},
			},
		},
		// Dependency loop:
		// A -> B
		// B -> C, C -> B
		testCase{
			config: util.NewConfig(util.TestDependenciesOpt(map[string][]string{
				"A": []string{"B"},
				"B": []string{"C"},
				"C": []string{"B"},
			})),
			tests:         []string{"A", "B", "C"},
			expectDepLoop: true,
		},
	}
	for index, test := range testCases {
		got, err := createTestDepGraph(test.config, test.tests)
		if test.expectDepLoop {
			if err == nil {
				t.Fatalf("test case %d: want errors, got: %v", index, err)
			}
		} else {
			if err != nil {
				t.Fatalf("test case %d: want no errors, got: %v", index, err)
			}
			if !reflect.DeepEqual(test.expectedTests, got) {
				t.Fatalf("test case %d: want %v, got %v", index, test.expectedTests, got)
			}
		}
	}
}
