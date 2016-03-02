// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jiri

import (
	"os"
	"path/filepath"
	"testing"

	"v.io/jiri/tool"
)

// TestFindRootEnvSymlink checks that FindRoot interprets the value of the
// JIRI_ROOT environment variable as a path, evaluates any symlinks the path
// might contain, and returns the result.
func TestFindRootEnvSymlink(t *testing.T) {
	ctx := tool.NewDefaultContext()

	// Create a temporary directory.
	tmpDir, err := ctx.NewSeq().TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer func() { ctx.NewSeq().RemoveAll(tmpDir).Done() }()

	// Make sure tmpDir is not a symlink itself.
	tmpDir, err = filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%v) failed: %v", tmpDir, err)
	}

	// Create a directory and a symlink to it.
	root, perm := filepath.Join(tmpDir, "root"), os.FileMode(0700)
	symRoot := filepath.Join(tmpDir, "sym_root")
	seq := ctx.NewSeq().MkdirAll(root, perm).Symlink(root, symRoot)
	if err := seq.Done(); err != nil {
		t.Fatalf("%v", err)
	}

	// Set the JIRI_ROOT to the symlink created above and check that FindRoot()
	// evaluates the symlink.
	oldRoot := os.Getenv(RootEnv)
	if err := os.Setenv(RootEnv, symRoot); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv(RootEnv, oldRoot)
	if got, want := FindRoot(), root; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}
