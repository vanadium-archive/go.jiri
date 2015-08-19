// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"v.io/x/devtools/internal/project"
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
	fakeRoot       *project.FakeV23Root
	ctx            *tool.Context
	gotoolsPath    string
	gotoolsCleanup func() error
}

// setupAPITest sets up the test environment and returns a testEnv
// representing the environment that was created.
func setupAPITest(t *testing.T, ctx *tool.Context) testEnv {
	root, err := project.NewFakeV23Root(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
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
	// The code under test wants to build visualfc/gotools, but this won't
	// exist in our fake environment unless we copy it in there.
	oldRoot, err := project.V23Root()
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

func teardownAPITest(t *testing.T, env testEnv) {
	os.Setenv("V23_ROOT", env.oldRoot)
	if err := env.fakeRoot.Cleanup(env.ctx); err != nil {
		t.Fatalf("%v", err)
	}
	if err := env.gotoolsCleanup(); err != nil {
		t.Fatalf("%v", err)
	}
	gotoolsBinPathFlag = ""
}

// TestPublicAPICheckError checks that the public API check fails for
// a CL that introduces changes to the public API.
func TestPublicAPICheckError(t *testing.T) {
	ctx := tool.NewDefaultContext()
	env := setupAPITest(t, ctx)
	defer teardownAPITest(t, env)

	config := util.NewConfig(util.APICheckProjectsOpt(map[string]struct{}{"test": struct{}{}}))
	if err := util.SaveConfig(ctx, config); err != nil {
		t.Fatalf("%v", err)
	}
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
	if err := doAPICheck(&buf, ctx.Stderr(), []string{"test"}, true); err != nil {
		t.Fatalf("doAPICheck failed: %v", err)
	} else if buf.String() == "" {
		t.Fatalf("doAPICheck detected no changes, but some were expected")
	}
}

// TestPublicAPICheckOk checks that the public API check succeeds for
// a CL that introduces no changes to the public API.
func TestPublicAPICheckOk(t *testing.T) {
	ctx := tool.NewDefaultContext()
	env := setupAPITest(t, ctx)
	defer teardownAPITest(t, env)

	config := util.NewConfig(util.APICheckProjectsOpt(map[string]struct{}{"test": struct{}{}}))
	if err := util.SaveConfig(ctx, config); err != nil {
		t.Fatalf("%v", err)
	}
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
	if err := doAPICheck(&buf, ctx.Stderr(), []string{"test"}, true); err != nil {
		t.Fatalf("doAPICheck failed: %v", err)
	} else if buf.String() != "" {
		t.Fatalf("doAPICheck detected changes, but none were expected: %s", buf.String())
	}
}

// TestPublicAPIMissingAPIFile ensures that the check will fail if a 'required
// check' project has a missing .api file and a non-empty public API.
func TestPublicAPIMissingAPIFile(t *testing.T) {
	ctx := tool.NewDefaultContext()
	env := setupAPITest(t, ctx)
	defer teardownAPITest(t, env)

	config := util.NewConfig(util.APICheckProjectsOpt(map[string]struct{}{"test": struct{}{}}))
	if err := util.SaveConfig(ctx, config); err != nil {
		t.Fatalf("%v", err)
	}
	branch := "my-branch"
	projectPath := filepath.Join(env.fakeRoot.Dir, "test")
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Write a go file with a public API and no corresponding .api file.
	writeFileOrDie(t, ctx, filepath.Join(projectPath, "file.go"), `package main

func TestFunction() {
}`)

	commitMessage := "Commit file.go"
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	var buf bytes.Buffer
	if err := doAPICheck(&buf, ctx.Stderr(), []string{"test"}, true); err != nil {
		t.Fatalf("doAPICheck failed: %v", err)
	} else if buf.String() == "" {
		t.Fatalf("doAPICheck should have failed, but did not")
	} else if !strings.Contains(buf.String(), "could not read the package's .api file") {
		t.Fatalf("doAPICheck failed, but not for the expected reason: %s", buf.String())
	}
}

// TestPublicAPIMissingAPIFileNoPublicAPI ensures that the check will pass if a
// 'required check' project has a missing .api but the public API is empty.
func TestPublicAPIMissingAPIFileNoPublicAPI(t *testing.T) {
	ctx := tool.NewDefaultContext()
	env := setupAPITest(t, ctx)
	defer teardownAPITest(t, env)

	config := util.NewConfig(util.APICheckProjectsOpt(map[string]struct{}{"test": struct{}{}}))
	if err := util.SaveConfig(ctx, config); err != nil {
		t.Fatalf("%v", err)
	}
	branch := "my-branch"
	projectPath := filepath.Join(env.fakeRoot.Dir, "test")
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Write a go file with a public API and no corresponding .api file.
	writeFileOrDie(t, ctx, filepath.Join(projectPath, "file.go"), `package main

func testFunction() {
}`)

	commitMessage := "Commit file.go"
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	var buf bytes.Buffer
	if err := doAPICheck(&buf, ctx.Stderr(), []string{"test"}, true); err != nil {
		t.Fatalf("doAPICheck failed: %v", err)
	} else if output := buf.String(); output != "" {
		t.Fatalf("doAPICheck should have passed, but did not: %s", output)
	}
}

// TestPublicAPIMissingAPIFileNotRequired ensures that the check will
// not fail if a 'required check' project has a missing .api file but
// that API file is in an 'internal' package.
func TestPublicAPIMissingAPIFileNotRequired(t *testing.T) {
	ctx := tool.NewDefaultContext()
	env := setupAPITest(t, ctx)
	defer teardownAPITest(t, env)

	config := util.NewConfig(util.APICheckProjectsOpt(map[string]struct{}{"test": struct{}{}}))
	if err := util.SaveConfig(ctx, config); err != nil {
		t.Fatalf("%v", err)
	}
	branch := "my-branch"
	projectPath := filepath.Join(env.fakeRoot.Dir, "test")
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Write a go file with a public API and no corresponding .api file.
	if err := os.Mkdir(filepath.Join(projectPath, "internal"), 0744); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}
	testFilePath := filepath.Join(projectPath, "internal", "file.go")
	writeFileOrDie(t, ctx, testFilePath, `package main

func TestFunction() {
}`)

	commitMessage := "Commit file.go"
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CommitFile(testFilePath, commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	var buf bytes.Buffer
	if err := doAPICheck(&buf, ctx.Stderr(), []string{"test"}, true); err != nil {
		t.Fatalf("doAPICheck failed: %v", err)
	} else if buf.String() != "" {
		t.Fatalf("doAPICheck should have passed, but did not: %s", buf.String())
	}
}

// TestPublicAPIUpdate checks that the api update command correctly
// updates the API definition.
func TestPublicAPIUpdate(t *testing.T) {
	ctx := tool.NewDefaultContext()
	env := setupAPITest(t, ctx)
	defer teardownAPITest(t, env)

	if err := util.SaveConfig(ctx, util.NewConfig()); err != nil {
		t.Fatalf("%v", err)
	}
	branch := "my-branch"
	projectPath := filepath.Join(env.fakeRoot.Dir, "test")
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Simulate an API with an existing public function called TestFunction.
	writeFileOrDie(t, ctx, filepath.Join(projectPath, ".api"), `# This is a comment that should be ignored
pkg main, func TestFunction()
`)

	// Write a change that changes TestFunction to TestFunction1.
	writeFileOrDie(t, ctx, filepath.Join(projectPath, "file.go"), `package main

func TestFunction1() {
}`)

	commitMessage := "Commit file.go"
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	var out bytes.Buffer
	cmdlineEnv := &cmdline.Env{Stdout: &out, Stderr: &out}
	if err := runAPIFix(cmdlineEnv, []string{"test"}); err != nil {
		t.Fatalf("should have succeeded but did not: %v\n%v", err, out.String())
	}

	contents, err := readAPIFileContents(ctx, filepath.Join(projectPath, ".api"))
	if err != nil {
		t.Fatalf("%v", err)
	}

	if got, want := string(contents), "pkg main, func TestFunction1()\n"; got != want {
		t.Fatalf("expected %s, got %s", want, got)
	}

	// Now write a change that changes TestFunction1 to testFunction1.
	// There should be no more public API left and the .api file should be
	// removed.
	writeFileOrDie(t, ctx, filepath.Join(projectPath, "file.go"), `package main

func testFunction1() {
}`)
	if err := ctx.Git(tool.RootDirOpt(projectPath)).CommitFile("file.go", commitMessage); err != nil {
		t.Fatalf("%v", err)
	}

	out.Reset()
	if err := runAPIFix(cmdlineEnv, []string{"test"}); err != nil {
		t.Fatalf("should have succeeded but did not: %v", err)
	}
	if _, err := ctx.Run().Stat(filepath.Join(projectPath, ".api")); err == nil {
		t.Fatalf(".api file exists when it should have been removed: %v", err)
	} else if !os.IsNotExist(err) {
		t.Fatalf("%v", err)
	}
}
