// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"v.io/x/lib/gosh"
)

func TestBootstrapJiri(t *testing.T) {
	sh := gosh.NewShell(t)
	sh.PropagateChildOutput = true
	defer sh.Cleanup()

	bootstrap, err := filepath.Abs("../../scripts/bootstrap_jiri")
	if err != nil {
		t.Fatalf("couldn't determine absolute path to bootstrap_jiri script")
	}
	rootDir := filepath.Join(sh.MakeTempDir(), "root")
	stdout, stderr := sh.Cmd(bootstrap, rootDir).StdoutStderr()
	if got, want := stdout, fmt.Sprintf("Please add %s to your PATH.\n", filepath.Join(rootDir, ".jiri_root", "scripts")); got != want {
		t.Errorf("stdout got %q, want %q", got, want)
	}
	if got, want := stderr, ""; got != want {
		t.Errorf("stderr got %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(rootDir, ".jiri_root", "bin", "jiri")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(rootDir, ".jiri_root", "scripts", "jiri")); err != nil {
		t.Error(err)
	}
}

// TestBuildJiriLocally checks that the jiri binary built in the bootstrap
// script can be built locally.
func TestBuildJiriLocally(t *testing.T) {
	sh := gosh.NewShell(t)
	sh.PropagateChildOutput = true
	defer sh.Cleanup()

	// Extract jiri package path from this line.
	// GOPATH="${tmp_dir}" go build -o "${bin_dir}/jiri" v.io/jiri/cmd/jiri
	bootstrap, err := filepath.Abs("../../scripts/bootstrap_jiri")
	if err != nil {
		t.Fatalf("couldn't determine absolute path to bootstrap_jiri script")
	}
	pkgRE := regexp.MustCompile(`.*go build.*\s([^\s]*)\n`)
	content, err := ioutil.ReadFile(bootstrap)
	if err != nil {
		t.Fatalf("couldn't read bootstrap script: %v", err)
	}
	matches := pkgRE.FindStringSubmatch(string(content))
	if len(matches) <= 1 {
		t.Fatalf("couldn't find jiri package from the bootstrap_jiri script")
	}
	pkg := matches[1]
	sh.Cmd("jiri", "go", "build", "-o", filepath.Join(sh.MakeTempDir(), "jiri"), pkg).Run()
}

func TestBootstrapJiriAlreadyExists(t *testing.T) {
	sh := gosh.NewShell(t)
	sh.PropagateChildOutput = true
	defer sh.Cleanup()

	bootstrap, err := filepath.Abs("../../scripts/bootstrap_jiri")
	if err != nil {
		t.Fatalf("couldn't determine absolute path to bootstrap_jiri script")
	}
	rootDir := sh.MakeTempDir()
	c := sh.Cmd(bootstrap, rootDir)
	c.ExitErrorIsOk = true
	stdout, stderr := c.StdoutStderr()
	if c.Err == nil {
		t.Errorf("error got %q, want nil", c.Err)
	}
	if got, want := stdout, ""; got != want {
		t.Errorf("stdout got %q, want %q", got, want)
	}
	if got, want := stderr, rootDir+" already exists"; !strings.Contains(got, want) {
		t.Errorf("stderr got %q, want substr %q", got, want)
	}
}
