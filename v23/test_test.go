// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/devtools/v23/internal/test"
	"v.io/x/lib/cmdline"
)

func TestTestProject(t *testing.T) {
	// Setup a fake V23_ROOT.
	ctx := tool.NewDefaultContext()
	root, err := util.NewFakeV23Root(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(ctx); err != nil {
			t.Fatalf("%v", err)
		}
	}()

	// Setup a fake config.
	config := util.NewConfig(util.ProjectTestsOpt(map[string][]string{"https://test-project": []string{"ignore-this"}}))
	if err := root.WriteLocalToolsConfig(ctx, config); err != nil {
		t.Fatalf("%v", err)
	}

	// Point the V23_ROOT and WORKSPACE environment variables to
	// the fake.
	oldRoot := os.Getenv("V23_ROOT")
	if err := os.Setenv("V23_ROOT", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("V23_ROOT", oldRoot)
	oldWorkspace := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("WORKSPACE", oldWorkspace)

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
	// Setup a fake V23_ROOT.
	ctx := tool.NewDefaultContext()
	root, err := util.NewFakeV23Root(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(ctx); err != nil {
			t.Fatalf("%v", err)
		}
	}()

	// Point the V23_ROOT and WORKSPACE environment variables to
	// the fake.
	oldRoot := os.Getenv("V23_ROOT")
	if err := os.Setenv("V23_ROOT", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("V23_ROOT", oldRoot)
	oldWorkspace := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", root.Dir); err != nil {
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
	// Setup a fake V23_ROOT.
	ctx := tool.NewDefaultContext()
	root, err := util.NewFakeV23Root(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(ctx); err != nil {
			t.Fatalf("%v", err)
		}
	}()

	// Point the V23_ROOT and WORKSPACE environment variables to
	// the fake.
	oldRoot := os.Getenv("V23_ROOT")
	if err := os.Setenv("V23_ROOT", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("V23_ROOT", oldRoot)
	oldWorkspace := os.Getenv("WORKSPACE")
	if err := os.Setenv("WORKSPACE", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("WORKSPACE", oldWorkspace)

	// Check that listing existing tests generates the expected output.
	var out bytes.Buffer
	command := cmdline.Command{}
	command.Init(nil, &out, &out)
	if err := runTestList(&command, []string{}); err != nil {
		t.Fatalf("%v", err)
	}
	testList, err := test.ListTests()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := strings.TrimSpace(out.String()), strings.Join(testList, "\n"); got != want {
		t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v", got, want)
	}
}
