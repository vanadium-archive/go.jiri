// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"v.io/x/lib/gosh"
)

func TestWhich(t *testing.T) {
	sh := gosh.NewShell(gosh.Opts{Fatalf: t.Fatalf, Logf: t.Logf})
	defer sh.Cleanup()
	jiriBinary := sh.BuildGoPkg("v.io/jiri")
	stdout, stderr := sh.Cmd(jiriBinary, []string{"which"}...).StdoutStderr()
	if got, want := stdout, fmt.Sprintf("# binary\n%s\n", jiriBinary); got != want {
		t.Errorf("stdout got %q, want %q", got, want)
	}
	if got, want := stderr, ""; got != want {
		t.Errorf("stderr got %q, want %q", got, want)
	}
}

// TestWhichScript tests the behavior of "jiri which" for the shim script.
func TestWhichScript(t *testing.T) {
	sh := gosh.NewShell(gosh.Opts{Fatalf: t.Fatalf, Logf: t.Logf})
	defer sh.Cleanup()

	jiriScript, err := filepath.Abs("./scripts/jiri")
	if err != nil {
		t.Fatalf("couldn't determine absolute path to jiri script")
	}
	c := sh.Cmd(jiriScript, []string{"which"}...)
	c.AddStdoutWriter(gosh.NopWriteCloser(os.Stdout))
	c.AddStderrWriter(gosh.NopWriteCloser(os.Stderr))
	stdout, stderr := c.StdoutStderr()
	if got, want := stdout, fmt.Sprintf("# script\n%s\n", jiriScript); got != want {
		t.Errorf("stdout got %q, want %q", got, want)
	}
	if got, want := stderr, ""; got != want {
		t.Errorf("stderr got %q, want %q", got, want)
	}
}
