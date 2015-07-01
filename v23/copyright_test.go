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
	"regexp"
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

	// Setup a fake V23_ROOT.
	root, err := util.NewFakeV23Root(ctx)
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

	oldRoot, err := util.V23Root()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.Setenv("V23_ROOT", root.Dir); err != nil {
		t.Fatalf("Setenv() failed: %v", err)
	}
	defer os.Setenv("V23_ROOT", oldRoot)

	allFiles := map[string]string{}
	for file, data := range assets.MatchFiles {
		allFiles[file] = data
	}
	for file, data := range assets.MatchPrefixFiles {
		allFiles[file] = data
	}

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
	for file, data := range allFiles {
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
	for file, _ := range allFiles {
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
	for file, _ := range allFiles {
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
	for file, _ := range allFiles {
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
	for file, _ := range allFiles {
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

	// Check that third-party files are skipped when checking for copyright
	// headers.
	errOut.Reset()
	if err := checkProject(ctx, project, assets, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	path := filepath.Join(projectPath, "third_party")
	if err := ctx.Run().MkdirAll(path, 0700); err != nil {
		t.Fatalf("%v", err)
	}
	path = filepath.Join(path, "test.go")
	if err := ctx.Run().WriteFile(path, []byte("garbage"), os.FileMode(0600)); err != nil {
		t.Fatalf("%v", err)
	}
	// Since this file is in a subdir, we must run "git add" to have git track it.
	// Without this, the test passes regardless of the subdir name.
	if err := ctx.Git(tool.RootDirOpt(projectPath)).Add(path); err != nil {
		t.Fatalf("%v", err)
	}
	if err := checkProject(ctx, project, assets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected error message: %q", errOut.String())
	}

	// Test .v23ignore functionality.
	errOut.Reset()
	// Add .v23ignore file.
	ignoreFile := filepath.Join(projectPath, v23Ignore)
	if err := ctx.Run().WriteFile(ignoreFile, []byte("public/fancy.js"), os.FileMode(0600)); err != nil {
		t.Fatalf("%v", err)
	}
	publicDir := filepath.Join(projectPath, "public")
	if err := ctx.Run().MkdirAll(publicDir, 0700); err != nil {
		t.Fatalf("%v", err)
	}
	filename := filepath.Join(publicDir, "fancy.js")
	if err := ctx.Run().WriteFile(filename, []byte("garbage"), os.FileMode(0600)); err != nil {
		t.Fatalf("%v", err)
	}
	// Since the copyright check only applies to tracked files, we must run "git
	// add" to have git track it. Without this, the test passes regardless of the
	// subdir name.
	if err := ctx.Git(tool.RootDirOpt(projectPath)).Add(filename); err != nil {
		t.Fatalf("%v", err)
	}
	if err := checkProject(ctx, project, assets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("unexpected error message: %q", errOut.String())
	}
}

func TestCopyrightIsIgnored(t *testing.T) {
	lines := []string{
		"public/bundle.*",
		"build/",
		"dist/min.js",
	}
	expressions := []*regexp.Regexp{}
	for _, line := range lines {
		expression, err := regexp.Compile(line)
		if err != nil {
			t.Fatalf("unexpected regexp.Compile(%v) failed: %v", line, err)
		}

		expressions = append(expressions, expression)
	}

	shouldIgnore := []string{
		"public/bundle.js",
		"public/bundle.css",
		"dist/min.js",
		"build/bar",
	}

	for _, path := range shouldIgnore {
		if ignore := isIgnored(path, expressions); !ignore {
			t.Errorf("isIgnored(%s, expressions) == %v, should be %v", path, ignore, true)
		}
	}

	shouldNotIgnore := []string{"foo", "bar"}
	for _, path := range shouldNotIgnore {
		if ignore := isIgnored(path, expressions); ignore {
			t.Errorf("isIgnored(%s, expressions) == %v, should be %v", path, ignore, false)
		}
	}
}
