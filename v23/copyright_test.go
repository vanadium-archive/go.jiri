// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

func TestCopyright(t *testing.T) {
	var errOut bytes.Buffer
	ctx := tool.NewContext(tool.ContextOpts{
		Stderr: io.MultiWriter(os.Stderr, &errOut),
	})

	// Load assets.
	dataDir, err := util.DataDirPath(ctx, "v23")
	if err != nil {
		t.Fatalf("%v", err)
	}
	assets, err := loadAssets(ctx, dataDir)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// Setup a fake VANADIUM_ROOT.
	root, err := util.NewFakeVanadiumRoot(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(ctx); err != nil {
			t.Fatalf("%v", err)
		}
	}()
	if err := root.CreateRemoteProject(ctx, "test"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := root.AddProject(ctx, util.Project{
		Name:   "test",
		Path:   "test",
		Remote: root.Projects["test"],
	}); err != nil {
		t.Fatalf("%v", err)
	}
	if err := root.UpdateUniverse(ctx, false); err != nil {
		t.Fatalf("%v", err)
	}

	oldRoot, err := util.VanadiumRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.Setenv("VANADIUM_ROOT", root.Dir); err != nil {
		t.Fatalf("Setenv() failed: %v", err)
	}
	defer os.Setenv("VANADIUM_ROOT", oldRoot)

	// Write out test licensing files and sample source code files to a
	// project and verify that the project checks out.
	projectPath := filepath.Join(root.Dir, "test")
	project := util.Project{Path: projectPath}
	for _, lang := range languages {
		file := "test" + lang.FileExtension
		if err := ctx.Run().WriteFile(filepath.Join(projectPath, file), nil, os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
		if checkFile(ctx, filepath.Join(project.Path, file), assets, true); err != nil {
			t.Fatalf("%v", err)
		}
		if err := ctx.Git(tool.RootDirOpt(projectPath)).CommitFile(file, "adding "+file); err != nil {
			t.Fatalf("%v", err)
		}
	}
	for file, data := range assets.Files {
		if err := ctx.Run().WriteFile(filepath.Join(projectPath, file), []byte(data), os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
		if err := ctx.Git(tool.RootDirOpt(projectPath)).CommitFile(file, "adding "+file); err != nil {
			t.Fatalf("%v", err)
		}
	}
	if err := checkProject(ctx, project, assets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := errOut.String(), ""; got != want {
		t.Fatalf("unexpected error message: got %q, want %q", got, want)
	}

	// Check that missing licensing files are reported correctly.
	for file, _ := range assets.Files {
		errOut.Reset()
		if err := checkProject(ctx, project, assets, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := filepath.Join(projectPath, file)
		if err := ctx.Git(tool.RootDirOpt(projectPath)).Remove(file); err != nil {
			t.Fatalf("%v", err)
		}
		if err := checkProject(ctx, project, assets, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := errOut.String(), fmt.Sprintf("%v is missing\n", path); got != want {
			t.Fatalf("unexpected error message: got %q, want %q", got, want)
		}
	}

	// Check that out-of-date licensing files are reported correctly.
	for file, _ := range assets.Files {
		errOut.Reset()
		if err := checkProject(ctx, project, assets, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := filepath.Join(projectPath, file)
		if err := ctx.Run().WriteFile(path, []byte("garbage"), os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
		if err := checkProject(ctx, project, assets, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := errOut.String(), fmt.Sprintf("%v is not up-to-date\n", path); got != want {
			t.Fatalf("unexpected error message: got %q, want %q", got, want)
		}
	}

	// Check that source code files without the copyright header are
	// reported correctly.
	for _, lang := range languages {
		errOut.Reset()
		if err := checkProject(ctx, project, assets, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := filepath.Join(projectPath, "test"+lang.FileExtension)
		if err := ctx.Run().WriteFile(path, []byte("garbage"), os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
		if err := checkProject(ctx, project, assets, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := errOut.String(), fmt.Sprintf("%v copyright is missing\n", path); got != want {
			t.Fatalf("unexpected error message: got %q, want %q", got, want)
		}
	}

	// Check that missing licensing files are fixed up correctly.
	for file, _ := range assets.Files {
		errOut.Reset()
		if err := checkProject(ctx, project, assets, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := filepath.Join(projectPath, file)
		if err := ctx.Run().RemoveAll(path); err != nil {
			t.Fatalf("%v", err)
		}
		if err := checkProject(ctx, project, assets, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := checkProject(ctx, project, assets, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := errOut.String(), ""; got != want {
			t.Fatalf("unexpected error message: got %q, want %q", got, want)
		}
	}

	// Check that out-of-date licensing files are fixed up correctly.
	for file, _ := range assets.Files {
		errOut.Reset()
		if err := checkProject(ctx, project, assets, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := filepath.Join(projectPath, file)
		if err := ctx.Run().WriteFile(path, []byte("garbage"), os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
		if err := checkProject(ctx, project, assets, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := checkProject(ctx, project, assets, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := errOut.String(), ""; got != want {
			t.Fatalf("unexpected error message: got %q, want %q", got, want)
		}
	}

	// Check that source code files missing the copyright header are
	// fixed up correctly.
	for _, lang := range languages {
		errOut.Reset()
		if err := checkProject(ctx, project, assets, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := filepath.Join(projectPath, "test"+lang.FileExtension)
		if err := ctx.Run().WriteFile(path, []byte("garbage"), os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
		if err := checkProject(ctx, project, assets, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := checkProject(ctx, project, assets, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := errOut.String(), ""; got != want {
			t.Fatalf("unexpected error message: got %q, want %q", got, want)
		}
	}
}
