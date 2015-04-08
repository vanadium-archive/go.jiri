// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

func writeFileOrDie(t *testing.T, ctx *tool.Context, path, contents string) {
	if err := ctx.Run().WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", path, contents, err)
	}
}

type testEnv struct {
	oldRoot        string
	fakeRoot       *util.FakeV23Root
	ctx            *tool.Context
	gotoolsPath    string
	gotoolsCleanup func() error
}

// setupApiTest sets up the test environment and returns a testEnv representing
// the environment that was created.
func setupApiTest(t *testing.T, ctx *tool.Context) testEnv {
	root, err := util.NewFakeV23Root(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
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
	// The code under test wants to build visualfc/gotools, but this won't
	// exist in our fake environment unless we copy it in there.
	oldRoot, err := util.V23Root()
	if err != nil {
		t.Fatalf("%v", err)
	}
	// Before we replace the vanadium root with our new fake one, we have
	// to build gotools. This is because the fake environment does not
	// contain the gotools source.
	gotoolsPath, cleanup, err := buildGotools(ctx)
	if err != nil {
		t.Fatalf("buildGotools failed: %v", err)
	}
	gotoolsBinPathFlag = gotoolsPath
	if err := os.Setenv("V23_ROOT", root.Dir); err != nil {
		t.Fatalf("Setenv() failed: %v", err)
	}

	return testEnv{oldRoot, root, ctx, gotoolsPath, cleanup}
}

func teardownApiTest(t *testing.T, env testEnv) {
	os.Setenv("V23_ROOT", env.oldRoot)
	if err := env.fakeRoot.Cleanup(env.ctx); err != nil {
		t.Fatalf("%v")
	}
	if err := env.gotoolsCleanup(); err != nil {
		t.Fatalf("%v")
	}
	gotoolsBinPathFlag = ""
}

// TestPublicApiCheckError checks that the public API check fails for a CL that
// introduces changes to the public API.
func TestPublicApiCheckError(t *testing.T) {
	ctx := tool.NewDefaultContext()
	env := setupApiTest(t, ctx)
	defer teardownApiTest(t, env)

	branch := "my-branch"
	projectPath := filepath.Join(env.fakeRoot.Dir, "test")
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Simulate an API with an existing public function called TestFunction.
	writeFileOrDie(t, ctx, filepath.Join(projectPath, ".api"), `# This is a comment that should be ignored
pkg main, func TestFunction()
`)

	// Write a change that un-exports TestFunction.
	writeFileOrDie(t, ctx, filepath.Join(projectPath, "file.go"), `package main

func testFunction() {
}`)

	commitMessage := "Commit file.go"
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	var buf bytes.Buffer
	command := cmdline.Command{}
	command.Init(nil, io.MultiWriter(ctx.Stdout(), &buf), ctx.Stderr())
	if err := runApiCheck(&command, []string{"test"}); err != nil {
		t.Fatalf("runApiCheck failed: %v", err)
	} else if buf.String() == "" {
		t.Fatalf("runApiCheck detected no changes, but some were expected")
	}
}

// TestPublicApiCheckOk checks that the public API check succeeds for a CL that
// introduces no changes to the public API.
func TestPublicApiCheckOk(t *testing.T) {
	ctx := tool.NewDefaultContext()
	env := setupApiTest(t, ctx)
	defer teardownApiTest(t, env)

	branch := "my-branch"
	projectPath := filepath.Join(env.fakeRoot.Dir, "test")
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Simulate an API with an existing public function called TestFunction.
	writeFileOrDie(t, ctx, filepath.Join(projectPath, ".api"), `# This is a comment that should be ignored
pkg main, func TestFunction()
`)

	// Write a change that un-exports TestFunction.
	writeFileOrDie(t, ctx, filepath.Join(projectPath, "file.go"), `package main

func TestFunction() {
}`)

	commitMessage := "Commit file.go"
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	var buf bytes.Buffer
	command := cmdline.Command{}
	command.Init(nil, io.MultiWriter(ctx.Stdout(), &buf), ctx.Stderr())
	if err := runApiCheck(&command, []string{"test"}); err != nil {
		t.Fatalf("runApiCheck failed: %v", err)
	} else if buf.String() != "" {
		t.Fatalf("runApiCheck detected changes, but none were expected: %s", buf.String())
	}
}

// TestPublicApiUpdate checks that the api update command correctly updates the
// API definition.
func TestPublicApiUpdate(t *testing.T) {
	ctx := tool.NewDefaultContext()
	env := setupApiTest(t, ctx)
	defer teardownApiTest(t, env)

	branch := "my-branch"
	projectPath := filepath.Join(env.fakeRoot.Dir, "test")
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Simulate an API with an existing public function called TestFunction.
	writeFileOrDie(t, ctx, filepath.Join(projectPath, ".api"), `# This is a comment that should be ignored
pkg main, func TestFunction()
`)

	// Write a change that un-exports TestFunction.
	writeFileOrDie(t, ctx, filepath.Join(projectPath, "file.go"), `package main

func testFunction() {
}`)

	commitMessage := "Commit file.go"
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	command := cmdline.Command{}
	if err := runApiFix(&command, []string{"test"}); err != nil {
		t.Fatalf("should have succeeded but did not: %v", err)
	}

	// The new public API is empty, so there should be nothing in the .api file.
	var contents bytes.Buffer
	err := readApiFileContents(filepath.Join(projectPath, ".api"), &contents)
	if err != nil {
		t.Fatalf("%v", err)
	}

	if got, want := contents.String(), ""; got != want {
		t.Fatalf("expected empty string, got %s", got)
	}
}
