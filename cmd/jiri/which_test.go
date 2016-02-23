// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"v.io/x/lib/gosh"
)

func TestWhich(t *testing.T) {
	sh := gosh.NewShell(gosh.Opts{Fatalf: t.Fatalf, Logf: t.Logf, PropagateChildOutput: true})
	defer sh.Cleanup()

	jiriBinary := sh.BuildGoPkg("v.io/jiri/cmd/jiri")
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
	sh := gosh.NewShell(gosh.Opts{Fatalf: t.Fatalf, Logf: t.Logf, PropagateChildOutput: true})
	defer sh.Cleanup()

	jiriScript, err := filepath.Abs("./scripts/jiri")
	if err != nil {
		t.Fatalf("couldn't determine absolute path to jiri script")
	}
	stdout, stderr := sh.Cmd(jiriScript, "which").StdoutStderr()
	if got, want := stdout, fmt.Sprintf("# script\n%s\n", jiriScript); got != want {
		t.Errorf("stdout got %q, want %q", got, want)
	}
	if got, want := stderr, ""; got != want {
		t.Errorf("stderr got %q, want %q", got, want)
	}
}
