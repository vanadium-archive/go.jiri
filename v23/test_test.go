package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"v.io/x/devtools/lib/testutil"
	"v.io/x/devtools/lib/util"
	"v.io/x/lib/cmdline"
)

func TestTestProject(t *testing.T) {
	ctx := util.DefaultContext()

	// Setup an instance of vanadium universe.
	rootDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer ctx.Run().RemoveAll(rootDir)
	oldRoot := os.Getenv("VANADIUM_ROOT")
	if err := os.Setenv("VANADIUM_ROOT", rootDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("VANADIUM_ROOT", oldRoot)
	oldWorkspace := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", rootDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("WORKSPACE", oldWorkspace)

	config := util.NewConfig(util.ProjectTestsOpt(map[string][]string{"https://test-project": []string{"ignore-this"}}))
	createConfig(t, ctx, config)

	// Check that running the tests for the test project generates
	// the expected output.
	var out bytes.Buffer
	command := cmdline.Command{}
	command.Init(nil, &out, &out)
	if err := runTestProject(&command, []string{"https://test-project"}); err != nil {
		t.Fatalf("%v", err)
	}
	got, want := out.String(), `##### Running test "ignore-this" #####
##### PASSED #####
SUMMARY:
ignore-this PASSED
`
	if got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTestRun(t *testing.T) {
	ctx := util.DefaultContext()

	// Setup an instance of vanadium universe.
	rootDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer ctx.Run().RemoveAll(rootDir)
	oldRoot := os.Getenv("VANADIUM_ROOT")
	if err := os.Setenv("VANADIUM_ROOT", rootDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("VANADIUM_ROOT", oldRoot)
	oldWorkspace := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", rootDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("WORKSPACE", oldWorkspace)

	// Check that running the test generates the expected output.
	var out bytes.Buffer
	command := cmdline.Command{}
	command.Init(nil, &out, &out)
	if err := runTestRun(&command, []string{"ignore-this"}); err != nil {
		t.Fatalf("%v", err)
	}
	got, want := out.String(), `##### Running test "ignore-this" #####
##### PASSED #####
SUMMARY:
ignore-this PASSED
`
	if got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}

func TestTestList(t *testing.T) {
	ctx := util.DefaultContext()

	// Setup an instance of vanadium universe.
	rootDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer ctx.Run().RemoveAll(rootDir)
	oldRoot := os.Getenv("VANADIUM_ROOT")
	if err := os.Setenv("VANADIUM_ROOT", rootDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("VANADIUM_ROOT", oldRoot)

	// Check that listing existing tests generates the expected
	// output.
	var out bytes.Buffer
	command := cmdline.Command{}
	command.Init(nil, &out, &out)
	if err := runTestList(&command, []string{}); err != nil {
		t.Fatalf("%v", err)
	}
	testList, err := testutil.TestList()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := strings.TrimSpace(out.String()), strings.Join(testList, "\n"); got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}
