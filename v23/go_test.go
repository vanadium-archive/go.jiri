// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"v.io/x/devtools/internal/buildinfo"
	"v.io/x/devtools/internal/runutil"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline2"
	"v.io/x/lib/metadata"
)

// TestGoVanadiumEnvironment checks that the implementation of the
// "v23 go" command sets up the vanadium environment and then
// dispatches calls to the go tool.
func TestGoVanadiumEnvironment(t *testing.T) {
	ctx := tool.NewDefaultContext()
	var stdout, stderr bytes.Buffer
	cmdlineEnv := &cmdline2.Env{Stdout: &stdout, Stderr: &stderr}
	oldGoPath := os.Getenv("GOPATH")
	if err := os.Setenv("GOPATH", ""); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("GOPATH", oldGoPath)
	if err := runGo(cmdlineEnv, []string{"env", "GOPATH"}); err != nil {
		t.Fatalf("%v", err)
	}
	env, err := util.VanadiumEnvironment(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := strings.TrimSpace(stdout.String()), env.Get("GOPATH"); got != want {
		t.Fatalf("GOPATH got %v, want %v", got, want)
	}
}

// TestGoVDLGeneration checks that the implementation of the "v23
// go" command generates up-to-date VDL files for select go tool
// commands before dispatching these commands to the go tool.
func TestGoVDLGeneration(t *testing.T) {
	ctx := tool.NewDefaultContext()
	var stdout, stderr bytes.Buffer
	cmdlineEnv := &cmdline2.Env{Stdout: &stdout, Stderr: &stderr}
	// Create a temporary directory for all our work.
	const tmpDirPrefix = "test_vgo"
	tmpDir, err := ctx.Run().TempDir("", tmpDirPrefix)
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer ctx.Run().RemoveAll(tmpDir)

	// Create test files <tmpDir>/src/testpkg/test.vdl and
	// <tmpDir>/src/testpkg/doc.go
	pkgdir := filepath.Join(tmpDir, "src", "testpkg")
	const perm = os.ModePerm
	if err := ctx.Run().MkdirAll(pkgdir, perm); err != nil {
		t.Fatalf(`MkdirAll(%q) failed: %v`, pkgdir, err)
	}
	goFile := filepath.Join(pkgdir, "doc.go")
	if err := ctx.Run().WriteFile(goFile, []byte("package testpkg\n"), perm); err != nil {
		t.Fatalf(`WriteFile(%q) failed: %v`, goFile, err)
	}
	inFile := filepath.Join(pkgdir, "test.vdl")
	outFile := inFile + ".go"
	if err := ctx.Run().WriteFile(inFile, []byte("package testpkg\n"), perm); err != nil {
		t.Fatalf(`WriteFile(%q) failed: %v`, inFile, err)
	}
	// Add <tmpDir> as first component of GOPATH and VDLPATH, so
	// we'll be able to find testpkg.  We need GOPATH for the "go
	// list" call when computing dependencies, and VDLPATH for the
	// "vdl generate" call.
	oldGoPath := os.Getenv("GOPATH")
	if err := os.Setenv("GOPATH", tmpDir+":"+oldGoPath); err != nil {
		t.Fatalf("Setenv() failed: %v", err)
	}
	defer os.Setenv("GOPATH", oldGoPath)
	oldVdlPath := os.Getenv("VDLPATH")
	if err := os.Setenv("VDLPATH", tmpDir+":"+oldVdlPath); err != nil {
		t.Fatalf("Setenv() failed: %v", err)
	}
	defer os.Setenv("VDLPATH", oldVdlPath)
	// Check that the 'env' go command does not generate the test VDL file.
	if err := runGo(cmdlineEnv, []string{"env", "GOPATH"}); err != nil {
		t.Fatalf("%v\n==STDOUT==\n%s\n==STDERR==\n%s", err, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(outFile); err != nil {
		if !os.IsNotExist(err) {
			t.Fatalf("Stat(%v) failed: %v", outFile, err)
		}
	} else {
		t.Fatalf("file %v exists and it should not.", outFile)
	}
	// Check that the 'build' go command generates the test VDL file.
	if err := runGo(cmdlineEnv, []string{"build", "testpkg"}); err != nil {
		t.Fatalf("%v\n==STDOUT==\n%s\n==STDERR==\n%s", err, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("Stat(%v) failed: %v", outFile, err)
	}
}

// TestProcessGoCmdAndArgs tests the internal function that parses and filters
// out flags from the go tool command line.
func TestProcessGoCmdAndArgs(t *testing.T) {
	const (
		buildcmds = "build install test"
		allcmds   = "build generate install run test"
	)
	tests := []struct {
		Cmds        string
		Args        []string
		Pkgs, Files []string
		Tags        string
	}{
		{allcmds, nil, nil, nil, ""},
		{allcmds, []string{}, nil, nil, ""},

		// PACKAGES
		{buildcmds, []string{"pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"pkg1", "pkg2"}, []string{"pkg1", "pkg2"}, nil, ""},
		// Single dash
		{buildcmds, []string{"-a"}, nil, nil, ""},
		{buildcmds, []string{"-a", "pkg"}, []string{"pkg"}, nil, ""},
		// Single dash with equals
		{buildcmds, []string{"-p=99"}, nil, nil, ""},
		{buildcmds, []string{"-p=99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"-a", "-p=99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"-p=99", "-a", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"-tags=foo"}, nil, nil, "foo"},
		{buildcmds, []string{"-tags=foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"-a", "-tags=foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"-tags=foo", "-a", "pkg"}, []string{"pkg"}, nil, "foo"},
		// Single dash with space
		{buildcmds, []string{"-p", "99"}, nil, nil, ""},
		{buildcmds, []string{"-p", "99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"-a", "-p", "99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"-p", "99", "-a", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"-tags", "foo"}, nil, nil, "foo"},
		{buildcmds, []string{"-tags", "foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"-a", "-tags", "foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"-tags", "foo", "-a", "pkg"}, []string{"pkg"}, nil, "foo"},
		// Double dash
		{buildcmds, []string{"--a"}, nil, nil, ""},
		{buildcmds, []string{"--a", "pkg"}, []string{"pkg"}, nil, ""},
		// Double dash with equals
		{buildcmds, []string{"--p=99"}, nil, nil, ""},
		{buildcmds, []string{"--p=99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"--a", "--p=99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"--p=99", "--a", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"--tags=foo"}, nil, nil, "foo"},
		{buildcmds, []string{"--tags=foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"--a", "--tags=foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"--tags=foo", "--a", "pkg"}, []string{"pkg"}, nil, "foo"},
		// Double dash with space
		{buildcmds, []string{"--p", "99"}, nil, nil, ""},
		{buildcmds, []string{"--p", "99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"--a", "--p", "99", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"--p", "99", "--a", "pkg"}, []string{"pkg"}, nil, ""},
		{buildcmds, []string{"--tags", "foo"}, nil, nil, "foo"},
		{buildcmds, []string{"--tags", "foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"--a", "--tags", "foo", "pkg"}, []string{"pkg"}, nil, "foo"},
		{buildcmds, []string{"--tags", "foo", "--a", "pkg"}, []string{"pkg"}, nil, "foo"},
		// Mixed
		{buildcmds, []string{"--p", "99", "-a", "-ccflags", "-I inc -X", "pkg1", "pkg2"}, []string{"pkg1", "pkg2"}, nil, ""},
		{buildcmds, []string{"--p", "99", "-tags=foo", "-a", "-ccflags", "-I inc -X", "pkg1", "pkg2"}, []string{"pkg1", "pkg2"}, nil, "foo"},

		// FILES
		{buildcmds, []string{"1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"1.go", "2.go"}, nil, []string{"1.go", "2.go"}, ""},
		// Single dash
		{buildcmds, []string{"-a"}, nil, nil, ""},
		{buildcmds, []string{"-a", "1.go"}, nil, []string{"1.go"}, ""},
		// Single dash with equals
		{buildcmds, []string{"-p=99"}, nil, nil, ""},
		{buildcmds, []string{"-p=99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"-a", "-p=99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"-p=99", "-a", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"-tags=foo"}, nil, nil, "foo"},
		{buildcmds, []string{"-tags=foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"-a", "-tags=foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"-tags=foo", "-a", "1.go"}, nil, []string{"1.go"}, "foo"},
		// Single dash with space
		{buildcmds, []string{"-p", "99"}, nil, nil, ""},
		{buildcmds, []string{"-p", "99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"-a", "-p", "99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"-p", "99", "-a", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"-tags", "foo"}, nil, nil, "foo"},
		{buildcmds, []string{"-tags", "foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"-a", "-tags", "foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"-tags", "foo", "-a", "1.go"}, nil, []string{"1.go"}, "foo"},
		// Double dash
		{buildcmds, []string{"--a"}, nil, nil, ""},
		{buildcmds, []string{"--a", "1.go"}, nil, []string{"1.go"}, ""},
		// Double dash with equals
		{buildcmds, []string{"--p=99"}, nil, nil, ""},
		{buildcmds, []string{"--p=99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"--a", "--p=99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"--p=99", "--a", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"--tags=foo"}, nil, nil, "foo"},
		{buildcmds, []string{"--tags=foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"--a", "--tags=foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"--tags=foo", "--a", "1.go"}, nil, []string{"1.go"}, "foo"},
		// Double dash with space
		{buildcmds, []string{"--p", "99"}, nil, nil, ""},
		{buildcmds, []string{"--p", "99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"--a", "--p", "99", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"--p", "99", "--a", "1.go"}, nil, []string{"1.go"}, ""},
		{buildcmds, []string{"--tags", "foo"}, nil, nil, "foo"},
		{buildcmds, []string{"--tags", "foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"--a", "--tags", "foo", "1.go"}, nil, []string{"1.go"}, "foo"},
		{buildcmds, []string{"--tags", "foo", "--a", "1.go"}, nil, []string{"1.go"}, "foo"},
		// Mixed
		{buildcmds, []string{"--p", "99", "-a", "-ccflags", "-I inc -X", "1.go", "2.go"}, nil, []string{"1.go", "2.go"}, ""},
		{buildcmds, []string{"--p", "99", "-tags=foo", "-a", "-ccflags", "-I inc -X", "1.go", "2.go"}, nil, []string{"1.go", "2.go"}, "foo"},

		// Go run requires gofiles, and treats every non-gofile as an arg.
		{"run", []string{"-a"}, nil, nil, ""},
		{"run", []string{"--a"}, nil, nil, ""},
		{"run", []string{"-p=99"}, nil, nil, ""},
		{"run", []string{"--p=99"}, nil, nil, ""},
		{"run", []string{"-p", "99"}, nil, nil, ""},
		{"run", []string{"--p", "99"}, nil, nil, ""},
		{"run", []string{"-tags=foo"}, nil, nil, "foo"},
		{"run", []string{"--tags=foo"}, nil, nil, "foo"},
		{"run", []string{"-tags", "foo"}, nil, nil, "foo"},
		{"run", []string{"--tags", "foo"}, nil, nil, "foo"},
		{"run", []string{"arg"}, nil, nil, ""},
		{"run", []string{"1.go"}, nil, []string{"1.go"}, ""},
		{"run", []string{"1.go", "2.go"}, nil, []string{"1.go", "2.go"}, ""},
		{"run", []string{"1.go", "2.go", "arg"}, nil, []string{"1.go", "2.go"}, ""},
		{"run", []string{"-a", "--p", "99", "1.go", "2.go", "arg"}, nil, []string{"1.go", "2.go"}, ""},
		{"run", []string{"-a", "--tags", "foo", "1.go", "2.go", "arg"}, nil, []string{"1.go", "2.go"}, "foo"},

		// Go test treats the first dash-prefix as the start of the testbin flags.
		{"test", []string{"pkg", "-t"}, []string{"pkg"}, nil, ""},
		{"test", []string{"pkg", "-t1", "-t2"}, []string{"pkg"}, nil, ""},
		{"test", []string{"pkg1", "pkg2", "-t1", "-t2"}, []string{"pkg1", "pkg2"}, nil, ""},
		{"test", []string{"--a", "-p", "99", "pkg1", "pkg2", "-t1", "-t2"}, []string{"pkg1", "pkg2"}, nil, ""},
		{"test", []string{"--a", "-tags", "foo", "pkg1", "pkg2", "-t1", "-t2"}, []string{"pkg1", "pkg2"}, nil, "foo"},
		{"test", []string{"1.go", "-t"}, nil, []string{"1.go"}, ""},
		{"test", []string{"1.go", "-t1", "-t2"}, nil, []string{"1.go"}, ""},
		{"test", []string{"1.go", "2.go", "-t1", "-t2"}, nil, []string{"1.go", "2.go"}, ""},
		{"test", []string{"--a", "-p", "99", "1.go", "2.go", "-t1", "-t2"}, nil, []string{"1.go", "2.go"}, ""},
		{"test", []string{"--a", "-tags", "foo", "1.go", "2.go", "-t1", "-t2"}, nil, []string{"1.go", "2.go"}, "foo"},

		// Go generate only supports the -run non-bool flag.
		{"generate", []string{"-a"}, nil, nil, ""},
		{"generate", []string{"--a"}, nil, nil, ""},
		{"generate", []string{"-run=XX"}, nil, nil, ""},
		{"generate", []string{"--run=XX"}, nil, nil, ""},
		{"generate", []string{"-run", "XX"}, nil, nil, ""},
		{"generate", []string{"--run", "XX"}, nil, nil, ""},
		{"generate", []string{"pkg"}, []string{"pkg"}, nil, ""},
		{"generate", []string{"pkg1", "pkg2"}, []string{"pkg1", "pkg2"}, nil, ""},
		{"generate", []string{"-a", "--run", "XX", "pkg1", "pkg2"}, []string{"pkg1", "pkg2"}, nil, ""},
		{"generate", []string{"--run", "XX", "-a", "pkg1", "pkg2"}, []string{"pkg1", "pkg2"}, nil, ""},

		{"generate", []string{"1.go"}, nil, []string{"1.go"}, ""},
		{"generate", []string{"1.go", "2.go"}, nil, []string{"1.go", "2.go"}, ""},
		{"generate", []string{"-a", "--run", "XX", "1.go", "2.go"}, nil, []string{"1.go", "2.go"}, ""},
		{"generate", []string{"--run", "XX", "-a", "1.go", "2.go"}, nil, []string{"1.go", "2.go"}, ""},
	}
	for _, test := range tests {
		for _, cmd := range strings.Split(test.Cmds, " ") {
			pkgs, files, tags := processGoCmdAndArgs(cmd, test.Args)
			if got, want := pkgs, test.Pkgs; !reflect.DeepEqual(got, want) {
				t.Errorf("%s %v got pkgs %#v, want %#v", cmd, test.Args, got, want)
			}
			if got, want := files, test.Files; !reflect.DeepEqual(got, want) {
				t.Errorf("%s %v got files %#v, want %#v", cmd, test.Args, got, want)
			}
			if got, want := tags, test.Tags; got != want {
				t.Errorf("%s %v got tags %#v, want %#v", cmd, test.Args, got, want)
			}
		}
	}
}

// TestComputeGoDeps tests the internal function that calls "go list" to get
// transitive dependencies.
func TestComputeGoDeps(t *testing.T) {
	ctx := tool.NewDefaultContext()
	env, err := util.VanadiumEnvironment(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	tests := []struct {
		Pkgs, Deps []string
	}{
		// This is checking the actual dependencies of the specified packages, so it
		// may break if we change the implementation; we try to pick dependencies
		// that are likely to remain in these packages.
		{nil, []string{"v.io/x/devtools/v23", "fmt"}},
		{[]string{"."}, []string{"v.io/x/devtools/v23", "fmt"}},
		{[]string{"v.io/x/devtools/v23"}, []string{"v.io/x/devtools/v23", "fmt"}},
		{[]string{"v.io/x/devtools/v23/..."}, []string{"v.io/x/devtools/v23", "fmt"}},
	}
	for _, test := range tests {
		t.Logf("%v\n", test.Pkgs)
		got, err := computeGoDeps(ctx, env, test.Pkgs, "")
		if err != nil {
			t.Errorf("%v failed: %v", test.Pkgs, err)
		}
		if want := test.Deps; !containsStrings(got, want) {
			t.Errorf("%v got %v, want to contain %v", test.Pkgs, got, want)
		}
	}
}

func containsStrings(super, sub []string) bool {
	superMap := make(map[string]bool)
	for _, x := range super {
		superMap[x] = true
	}
	for _, x := range sub {
		if !superMap[x] {
			return false
		}
	}
	return true
}

func TestGoBuildWithMetaData(t *testing.T) {
	ctx, start := tool.NewDefaultContext(), time.Now().UTC()
	// Set up a temp directory.
	dir, err := ctx.Run().TempDir("", "v23_metadata_test")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer ctx.Run().RemoveAll(dir)
	// Build the v23 binary itself.
	var buf bytes.Buffer
	opts := runutil.Opts{Stdout: &buf, Stderr: &buf, Verbose: true}
	testbin := filepath.Join(dir, "testbin")
	if err := ctx.Run().CommandWithOpts(opts, "v23", "go", "build", "-o", testbin); err != nil {
		t.Fatalf("build of v23 failed: %v\n%s", err, buf.String())
	}
	// Run the v23 binary.
	buf.Reset()
	if err := ctx.Run().CommandWithOpts(opts, testbin, "-v23.metadata"); err != nil {
		t.Errorf("run of v23 -v23.metadata failed: %v\n%s", err, buf.String())
	}
	// Decode the output metadata and spot-check some values.
	outData := buf.Bytes()
	t.Log(string(outData))
	md, err := metadata.FromXML(outData)
	if err != nil {
		t.Errorf("FromXML failed: %v\n%s", err, outData)
	}
	bi, err := buildinfo.FromMetaData(md)
	if err != nil {
		t.Errorf("DecodeMetaData(%#v) failed: %v", md, err)
	}
	const fudge = -5 * time.Second
	if bi.Time.Before(start.Add(fudge)) {
		t.Errorf("build time %v < start %v", bi.Time, start)
	}
}
