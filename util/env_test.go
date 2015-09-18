// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"os"
	"path/filepath"
	"testing"

	"v.io/jiri/project"
	"v.io/jiri/tool"
)

// TestJiriRootSymlink checks that JiriRoot interprets the value
// of the JIRI_ROOT environment variable as a path, evaluates any
// symlinks the path might contain, and returns the result.
func TestJiriRootSymlink(t *testing.T) {
	ctx := tool.NewDefaultContext()

	// Create a temporary directory.
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer ctx.Run().RemoveAll(tmpDir)

	// Make sure tmpDir is not a symlink itself.
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%v) failed: %v", tmpDir, err)
	}

	// Create a directory and a symlink to it.
	root, perm := filepath.Join(tmpDir, "root"), os.FileMode(0700)
	if err := ctx.Run().MkdirAll(root, perm); err != nil {
		t.Fatalf("%v", err)
	}
	symRoot := filepath.Join(tmpDir, "sym_root")
	if err := ctx.Run().Symlink(root, symRoot); err != nil {
		t.Fatalf("%v", err)
	}

	// Set the JIRI_ROOT to the symlink created above and check
	// that JiriRoot() evaluates the symlink.
	oldRoot := os.Getenv("JIRI_ROOT")
	if err := os.Setenv("JIRI_ROOT", symRoot); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("JIRI_ROOT", oldRoot)
	got, err := project.JiriRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if want := root; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}
