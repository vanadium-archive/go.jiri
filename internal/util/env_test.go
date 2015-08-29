// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"os"
	"path/filepath"
	"testing"

	"v.io/jiri/internal/project"
	"v.io/jiri/internal/tool"
	"v.io/x/lib/envvar"
)

// TestV23RootSymlink checks that V23Root interprets the value
// of the V23_ROOT environment variable as a path, evaluates any
// symlinks the path might contain, and returns the result.
func TestV23RootSymlink(t *testing.T) {
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

	// Set the V23_ROOT to the symlink created above and check
	// that V23Root() evaluates the symlink.
	oldRoot := os.Getenv("V23_ROOT")
	if err := os.Setenv("V23_ROOT", symRoot); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("V23_ROOT", oldRoot)
	got, err := project.V23Root()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if want := root; got != want {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

func testSetPathHelper(t *testing.T, name string) {
	ctx := tool.NewDefaultContext()

	// Setup a fake V23_ROOT.
	root, err := project.NewFakeV23Root(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(ctx); err != nil {
			t.Fatalf("%v", err)
		}
	}()

	// Create a test project and identify it as a Go workspace.
	if err := root.CreateRemoteProject(ctx, "test"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := root.AddProject(ctx, project.Project{
		Name:   "test",
		Path:   "test",
		Remote: root.Projects["test"],
	}); err != nil {
		t.Fatalf("%v", err)
	}
	if err := root.UpdateUniverse(ctx, false); err != nil {
		t.Fatalf("%v", err)
	}
	var config *Config
	switch name {
	case "GOPATH":
		config = NewConfig(GoWorkspacesOpt([]string{"test", "does/not/exist"}))
	case "VDLPATH":
		config = NewConfig(VDLWorkspacesOpt([]string{"test", "does/not/exist"}))
	}

	oldRoot, err := project.V23Root()
	if err := os.Setenv("V23_ROOT", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("V23_ROOT", oldRoot)

	// Retrieve V23_ROOT through V23Root() to account for symlinks.
	jiriRoot, err := project.V23Root()
	if err != nil {
		t.Fatalf("%v", err)
	}
	env := new(envvar.Vars)
	var want string
	switch name {
	case "GOPATH":
		want = filepath.Join(jiriRoot, "test")
		if err := setGoPath(ctx, env, jiriRoot, config); err != nil {
			t.Fatalf("%v", err)
		}
	case "VDLPATH":
		// Make a fake src directory.
		want = filepath.Join(jiriRoot, "test", "src")
		if err := ctx.Run().MkdirAll(want, 0755); err != nil {
			t.Fatalf("%v", err)
		}
		if err := setVdlPath(ctx, env, jiriRoot, config); err != nil {
			t.Fatalf("%v", err)
		}
	}
	if got := env.Get(name); got != want {
		t.Fatalf("unexpected value: got %v, want %v", got, want)
	}
}

func TestSetGoPath(t *testing.T) {
	testSetPathHelper(t, "GOPATH")
}

func TestSetVdlPath(t *testing.T) {
	testSetPathHelper(t, "VDLPATH")
}
