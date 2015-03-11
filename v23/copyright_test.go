package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"v.io/x/devtools/lib/util"
)

func TestCopyright(t *testing.T) {
	var errOut bytes.Buffer
	ctx := util.NewContext(nil, os.Stdin, os.Stdout, io.MultiWriter(os.Stderr, &errOut), false, false, true)

	// Load assets.
	projects, tools, err := util.ReadManifest(ctx, manifestFlag)
	if err != nil {
		t.Fatalf("%v", err)
	}
	dataDir := filepath.Join(projects[tools["v23"].Project].Path, tools["v23"].Data)
	assets, err := loadAssets(ctx, dataDir)
	if err != nil {
		t.Fatalf("%v", err)
	}

	// Create a fake VANADIUM_ROOT.
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ctx.Run().RemoveAll(tmpDir)
	oldRoot := os.Getenv("VANADIUM_ROOT")
	if err := os.Setenv("VANADIUM_ROOT", tmpDir); err != nil {
		t.Fatalf("Setenv() failed: %v", err)
	}
	defer os.Setenv("VANADIUM_ROOT", oldRoot)

	// Write out test licensing files and sample source code files to a
	// project and verify that the project checks out.
	projectPath := filepath.Join(tmpDir, "test")
	project := util.Project{
		Path: projectPath,
	}
	if err := ctx.Run().MkdirAll(projectPath, os.FileMode(0700)); err != nil {
		t.Fatalf("%v", err)
	}
	for _, lang := range languages {
		path := filepath.Join(projectPath, "test"+lang.Suffix)
		if err := ctx.Run().WriteFile(path, nil, os.FileMode(0600)); err != nil {
			t.Fatalf("%v", err)
		}
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

	// Check that missing licensing files are reported correctly.
	for file, _ := range assets.Files {
		errOut.Reset()
		if err := checkProject(ctx, project, assets, true); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		path := filepath.Join(projectPath, file)
		if err := ctx.Run().RemoveAll(path); err != nil {
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
		path := filepath.Join(projectPath, "test"+lang.Suffix)
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
		path := filepath.Join(projectPath, "test"+lang.Suffix)
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
