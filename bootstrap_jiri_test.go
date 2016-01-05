// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"v.io/x/lib/gosh"
)

func TestBootstrapJiri(t *testing.T) {
	sh := gosh.NewShell(gosh.Opts{Fatalf: t.Fatalf, Logf: t.Logf})
	defer sh.Cleanup()

	bootstrap, err := filepath.Abs("./scripts/bootstrap_jiri")
	if err != nil {
		t.Fatalf("couldn't determine absolute path to bootstrap_jiri script")
	}
	rootDir := filepath.Join(sh.MakeTempDir(), "root")
	c := sh.Cmd(bootstrap, []string{rootDir}...)
	c.AddStdoutWriter(gosh.NopWriteCloser(os.Stdout))
	c.AddStderrWriter(gosh.NopWriteCloser(os.Stderr))
	stdout, stderr := c.StdoutStderr()
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

func TestBootstrapJiriAlreadyExists(t *testing.T) {
	sh := gosh.NewShell(gosh.Opts{Fatalf: t.Fatalf, Logf: t.Logf})
	defer sh.Cleanup()

	bootstrap, err := filepath.Abs("./scripts/bootstrap_jiri")
	if err != nil {
		t.Fatalf("couldn't determine absolute path to bootstrap_jiri script")
	}
	rootDir := sh.MakeTempDir()
	c := sh.Cmd(bootstrap, []string{rootDir}...)
	c.AddStdoutWriter(gosh.NopWriteCloser(os.Stdout))
	c.AddStderrWriter(gosh.NopWriteCloser(os.Stderr))
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
