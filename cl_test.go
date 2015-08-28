// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"v.io/jiri/internal/gerrit"
	"v.io/jiri/internal/gitutil"
	"v.io/jiri/internal/project"
	"v.io/jiri/internal/tool"
)

// assertCommitCount asserts that the commit count between two
// branches matches the expectedCount.
func assertCommitCount(t *testing.T, ctx *tool.Context, branch, baseBranch string, expectedCount int) {
	got, err := ctx.Git().CountCommits(branch, baseBranch)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if want := 1; got != want {
		t.Fatalf("unexpected number of commits: got %v, want %v", got, want)
	}
}

// assertFileContent asserts that the content of the given file
// matches the expected content.
func assertFileContent(t *testing.T, ctx *tool.Context, file, want string) {
	got, err := ctx.Run().ReadFile(file)
	if err != nil {
		t.Fatalf("%v\n", err)
	}
	if string(got) != want {
		t.Fatalf("unexpected content of file %v: got %v, want %v", file, got, want)
	}
}

// assertFilesExist asserts that the files exist.
func assertFilesExist(t *testing.T, ctx *tool.Context, files []string) {
	for _, file := range files {
		if _, err := ctx.Run().Stat(file); err != nil {
			if os.IsNotExist(err) {
				t.Fatalf("expected file %v to exist but it did not", file)
			}
			t.Fatalf("%v", err)
		}
	}
}

// assertFilesDoNotExist asserts that the files do not exist.
func assertFilesDoNotExist(t *testing.T, ctx *tool.Context, files []string) {
	for _, file := range files {
		if _, err := ctx.Run().Stat(file); err != nil && !os.IsNotExist(err) {
			t.Fatalf("%v", err)
		} else if err == nil {
			t.Fatalf("expected file %v to not exist but it did", file)
		}
	}
}

// assertFilesCommitted asserts that the files exist and are committed
// in the current branch.
func assertFilesCommitted(t *testing.T, ctx *tool.Context, files []string) {
	assertFilesExist(t, ctx, files)
	for _, file := range files {
		if !ctx.Git().IsFileCommitted(file) {
			t.Fatalf("expected file %v to be committed but it is not", file)
		}
	}
}

// assertFilesNotCommitted asserts that the files exist and are *not*
// committed in the current branch.
func assertFilesNotCommitted(t *testing.T, ctx *tool.Context, files []string) {
	assertFilesExist(t, ctx, files)
	for _, file := range files {
		if ctx.Git().IsFileCommitted(file) {
			t.Fatalf("expected file %v not to be committed but it is", file)
		}
	}
}

// assertFilesPushedToRef asserts that the given files have been
// pushed to the given remote repository reference.
func assertFilesPushedToRef(t *testing.T, ctx *tool.Context, repoPath, gerritPath, pushedRef string, files []string) {
	chdir(t, ctx, gerritPath)
	assertCommitCount(t, ctx, pushedRef, "master", 1)
	if err := ctx.Git().CheckoutBranch(pushedRef); err != nil {
		t.Fatalf("%v", err)
	}
	assertFilesCommitted(t, ctx, files)
	chdir(t, ctx, repoPath)
}

// assertStashSize asserts that the stash size matches the expected
// size.
func assertStashSize(t *testing.T, ctx *tool.Context, want int) {
	got, err := ctx.Git().StashSize()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got != want {
		t.Fatalf("unxpected stash size: got %v, want %v", got, want)
	}
}

// commitFile commits a file with the specified content into a branch
func commitFile(t *testing.T, ctx *tool.Context, filename string, content string) {
	if err := ctx.Run().WriteFile(filename, []byte(content), 0644); err != nil {
		t.Fatalf("%v", err)
	}
	commitMessage := "Commit " + filename
	if err := ctx.Git().CommitFile(filename, commitMessage); err != nil {
		t.Fatalf("%v", err)
	}
}

// commitFiles commits the given files into to current branch.
func commitFiles(t *testing.T, ctx *tool.Context, filenames []string) {
	// Create and commit the files one at a time.
	for _, filename := range filenames {
		content := "This is file " + filename
		commitFile(t, ctx, filename, content)
	}
}

// createRepo creates a new repository in the given working directory.
func createRepo(t *testing.T, ctx *tool.Context, workingDir, prefix string) string {
	repoPath, err := ctx.Run().TempDir(workingDir, "repo-"+prefix)
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	if err := os.Chmod(repoPath, 0777); err != nil {
		t.Fatalf("Chmod(%v) failed: %v", repoPath, err)
	}
	if err := ctx.Git().Init(repoPath); err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Run().MkdirAll(filepath.Join(repoPath, project.MetadataDirName()), os.FileMode(0755)); err != nil {
		t.Fatalf("%v", err)
	}
	return repoPath
}

// Simple commit-msg hook that adds a fake Change Id.
var commitMsgHook string = `
#!/bin/sh
MSG="$1"
echo "Change-Id: I0000000000000000000000000000000000000000" >> $MSG
`

// installCommitMsgHook links the gerrit commit-msg hook into a different repo.
func installCommitMsgHook(t *testing.T, ctx *tool.Context, repoPath string) {
	hookLocation := path.Join(repoPath, ".git/hooks/commit-msg")
	if err := ctx.Run().WriteFile(hookLocation, []byte(commitMsgHook), 0755); err != nil {
		t.Fatalf("WriteFile(%v) failed: %v", hookLocation, err)
	}
}

// chdir changes the runtime working directory and traps any errors.
func chdir(t *testing.T, ctx *tool.Context, path string) {
	if err := ctx.Run().Chdir(path); err != nil {
		_, file, line, _ := runtime.Caller(1)
		t.Fatalf("%s: %d: Chdir(%v) failed: %v", file, line, path, err)
	}
}

// createRepoFromOrigin creates a Git repo tracking origin/master.
func createRepoFromOrigin(t *testing.T, ctx *tool.Context, workingDir string, subpath string, originPath string) string {
	repoPath := createRepo(t, ctx, workingDir, subpath)
	chdir(t, ctx, repoPath)
	if err := ctx.Git().AddRemote("origin", originPath); err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Git().Pull("origin", "master"); err != nil {
		t.Fatalf("%v", err)
	}
	return repoPath
}

// createTestRepos sets up three local repositories: origin, gerrit,
// and the main test repository which pulls from origin and can push
// to gerrit.
func createTestRepos(t *testing.T, ctx *tool.Context, workingDir string) (string, string, string) {
	// Create origin.
	originPath := createRepo(t, ctx, workingDir, "origin")
	chdir(t, ctx, originPath)
	if err := ctx.Git().CommitWithMessage("initial commit"); err != nil {
		t.Fatalf("%v", err)
	}
	// Create test repo.
	repoPath := createRepoFromOrigin(t, ctx, workingDir, "test", originPath)
	// Add Gerrit remote.
	gerritPath := createRepoFromOrigin(t, ctx, workingDir, "gerrit", originPath)
	// Switch back to test repo.
	chdir(t, ctx, repoPath)
	return repoPath, originPath, gerritPath
}

// submit mocks a Gerrit review submit by pushing the Gerrit remote to origin.
// Actually origin pulls from Gerrit since origin isn't actually a bare git repo.
// Some of our tests actually rely on accessing .git in origin, so it must be non-bare.
func submit(t *testing.T, ctx *tool.Context, originPath string, gerritPath string, review *review) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	chdir(t, ctx, originPath)
	expectedRef := gerrit.Reference(review.CLOpts)
	if err := ctx.Git().Pull(gerritPath, expectedRef); err != nil {
		t.Fatalf("Pull gerrit to origin failed: %v", err)
	}
	chdir(t, ctx, cwd)
}

// setupTest creates a setup for testing the review tool.
func setupTest(t *testing.T, installHook bool) (ctx *tool.Context, cwd string, root *project.FakeV23Root, repoPath, originPath, gerritPath string) {
	ctx = tool.NewDefaultContext()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	root, err = project.NewFakeV23Root(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	repoPath, originPath, gerritPath = createTestRepos(t, ctx, root.Dir)
	if installHook == true {
		for _, path := range []string{repoPath, originPath, gerritPath} {
			installCommitMsgHook(t, ctx, path)
		}
	}
	chdir(t, ctx, repoPath)
	return
}

// teardownTest cleans up the setup for testing the review tool.
func teardownTest(t *testing.T, ctx *tool.Context, oldWorkDir string, root *project.FakeV23Root) {
	chdir(t, ctx, oldWorkDir)
	if err := root.Cleanup(ctx); err != nil {
		t.Fatalf("%v", err)
	}
}

func createCLWithFile(t *testing.T, ctx *tool.Context, branch string, file string) {
	if err := newCL(ctx, []string{branch}); err != nil {
		t.Fatalf("%v", err)
	}
	commitFiles(t, ctx, []string{file})
}

// TestCleanupClean checks that cleanup succeeds if the branch to be
// cleaned up has been merged with the master.
func TestCleanupClean(t *testing.T) {
	ctx, cwd, root, repoPath, originPath, _ := setupTest(t, true)
	defer teardownTest(t, ctx, cwd, root)
	branch := "my-branch"
	if err := ctx.Git().CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}
	commitFiles(t, ctx, []string{"file1", "file2"})
	if err := ctx.Git().CheckoutBranch("master"); err != nil {
		t.Fatalf("%v", err)
	}
	chdir(t, ctx, originPath)
	commitFiles(t, ctx, []string{"file1", "file2"})
	chdir(t, ctx, repoPath)
	if err := cleanupCL(ctx, []string{branch}); err != nil {
		t.Fatalf("cleanup() failed: %v", err)
	}
	if ctx.Git().BranchExists(branch) {
		t.Fatalf("cleanup failed to remove the feature branch")
	}
}

// TestCleanupDirty checks that cleanup is a no-op if the branch to be
// cleaned up has unmerged changes.
func TestCleanupDirty(t *testing.T) {
	ctx, cwd, root, _, _, _ := setupTest(t, true)
	defer teardownTest(t, ctx, cwd, root)
	branch := "my-branch"
	if err := ctx.Git().CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}
	files := []string{"file1", "file2"}
	commitFiles(t, ctx, files)
	if err := ctx.Git().CheckoutBranch("master"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := cleanupCL(ctx, []string{branch}); err == nil {
		t.Fatalf("cleanup did not fail when it should")
	}
	if err := ctx.Git().CheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}
	assertFilesCommitted(t, ctx, files)
}

// TestCreateReviewBranch checks that the temporary review branch is
// created correctly.
func TestCreateReviewBranch(t *testing.T) {
	ctx, cwd, root, _, _, _ := setupTest(t, true)
	defer teardownTest(t, ctx, cwd, root)
	branch := "my-branch"
	if err := ctx.Git().CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}
	files := []string{"file1", "file2", "file3"}
	commitFiles(t, ctx, files)
	review, err := newReview(ctx, gerrit.CLOpts{})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if expected, got := branch+"-REVIEW", review.reviewBranch; expected != got {
		t.Fatalf("Unexpected review branch name: expected %v, got %v", expected, got)
	}
	commitMessage := "squashed commit"
	if err := review.createReviewBranch(commitMessage); err != nil {
		t.Fatalf("%v", err)
	}
	// Verify that the branch exists.
	if !ctx.Git().BranchExists(review.reviewBranch) {
		t.Fatalf("review branch not found")
	}
	if err := ctx.Git().CheckoutBranch(review.reviewBranch); err != nil {
		t.Fatalf("%v", err)
	}
	assertCommitCount(t, ctx, review.reviewBranch, "master", 1)
	assertFilesCommitted(t, ctx, files)
}

// TestCreateReviewBranchWithEmptyChange checks that running
// createReviewBranch() on a branch with no changes will result in an
// EmptyChangeError.
func TestCreateReviewBranchWithEmptyChange(t *testing.T) {
	ctx, cwd, root, _, _, _ := setupTest(t, true)
	defer teardownTest(t, ctx, cwd, root)
	branch := "my-branch"
	if err := ctx.Git().CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}
	review, err := newReview(ctx, gerrit.CLOpts{Remote: branch})
	if err != nil {
		t.Fatalf("%v", err)
	}
	commitMessage := "squashed commit"
	err = review.createReviewBranch(commitMessage)
	if err == nil {
		t.Fatalf("creating a review did not fail when it should")
	}
	if _, ok := err.(emptyChangeError); !ok {
		t.Fatalf("unexpected error type: %v", err)
	}
}

// TestSendReview checks the various options for sending a review.
func TestSendReview(t *testing.T) {
	ctx, cwd, root, repoPath, _, gerritPath := setupTest(t, true)
	defer teardownTest(t, ctx, cwd, root)
	branch := "my-branch"
	if err := ctx.Git().CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}
	files := []string{"file1"}
	commitFiles(t, ctx, files)
	{
		// Test with draft = false, no reviewiers, and no ccs.
		review, err := newReview(ctx, gerrit.CLOpts{Remote: gerritPath})
		if err != nil {
			t.Fatalf("%v", err)
		}
		if err := review.send(); err != nil {
			t.Fatalf("failed to send a review: %v", err)
		}
		expectedRef := gerrit.Reference(review.CLOpts)
		assertFilesPushedToRef(t, ctx, repoPath, gerritPath, expectedRef, files)
	}
	{
		// Test with draft = true, no reviewers, and no ccs.
		review, err := newReview(ctx, gerrit.CLOpts{
			Draft:  true,
			Remote: gerritPath,
		})
		if err != nil {
			t.Fatalf("%v", err)
		}
		if err := review.send(); err != nil {
			t.Fatalf("failed to send a review: %v", err)
		}
		expectedRef := gerrit.Reference(review.CLOpts)
		assertFilesPushedToRef(t, ctx, repoPath, gerritPath, expectedRef, files)
	}
	{
		// Test with draft = false, reviewers, and no ccs.
		review, err := newReview(ctx, gerrit.CLOpts{
			Remote:    gerritPath,
			Reviewers: parseEmails("reviewer1,reviewer2@example.org"),
		})
		if err != nil {
			t.Fatalf("%v", err)
		}
		if err := review.send(); err != nil {
			t.Fatalf("failed to send a review: %v", err)
		}
		expectedRef := gerrit.Reference(review.CLOpts)
		assertFilesPushedToRef(t, ctx, repoPath, gerritPath, expectedRef, files)
	}
	{
		// Test with draft = true, reviewers, and ccs.
		review, err := newReview(ctx, gerrit.CLOpts{
			Ccs:       parseEmails("cc1@example.org,cc2"),
			Draft:     true,
			Remote:    gerritPath,
			Reviewers: parseEmails("reviewer3@example.org,reviewer4"),
		})
		if err != nil {
			t.Fatalf("%v", err)
		}
		if err := review.send(); err != nil {
			t.Fatalf("failed to send a review: %v", err)
		}
		expectedRef := gerrit.Reference(review.CLOpts)
		assertFilesPushedToRef(t, ctx, repoPath, gerritPath, expectedRef, files)
	}
}

// TestSendReviewNoChangeID checks that review.send() correctly errors when
// not run with a commit hook that adds a Change-Id.
func TestSendReviewNoChangeID(t *testing.T) {
	// Pass 'false' to setup so it doesn't install the commit-msg hook.
	ctx, cwd, root, _, _, gerritPath := setupTest(t, false)
	defer teardownTest(t, ctx, cwd, root)
	branch := "my-branch"
	if err := ctx.Git().CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}
	commitFiles(t, ctx, []string{"file1"})
	review, err := newReview(ctx, gerrit.CLOpts{Remote: gerritPath})
	if err != nil {
		t.Fatalf("%v", err)
	}
	err = review.send()
	if err == nil {
		t.Fatalf("sending a review did not fail when it should")
	}
	if _, ok := err.(noChangeIDError); !ok {
		t.Fatalf("unexpected error type: %v", err)
	}
}

// TestEndToEnd checks the end-to-end functionality of the review tool.
func TestEndToEnd(t *testing.T) {
	ctx, cwd, root, repoPath, _, gerritPath := setupTest(t, true)
	defer teardownTest(t, ctx, cwd, root)
	branch := "my-branch"
	if err := ctx.Git().CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}
	files := []string{"file1", "file2", "file3"}
	commitFiles(t, ctx, files)
	review, err := newReview(ctx, gerrit.CLOpts{Remote: gerritPath})
	if err != nil {
		t.Fatalf("%v", err)
	}
	setTopicFlag = false
	if err := review.run(); err != nil {
		t.Fatalf("run() failed: %v", err)
	}
	expectedRef := gerrit.Reference(review.CLOpts)
	assertFilesPushedToRef(t, ctx, repoPath, gerritPath, expectedRef, files)
}

// TestLabelsInCommitMessage checks the labels are correctly processed
// for the commit message.
//
// HACK ALERT: This test runs the review.run() function multiple
// times. The function ends up pushing a commit to a fake "gerrit"
// repository created by the setupTest() function. For the real gerrit
// repository, it is possible to push to the refs/for/change reference
// multiple times, because it is a special reference that "maps"
// incoming commits to CL branches based on the commit message
// Change-Id. The fake "gerrit" repository does not implement this
// logic and thus the same reference cannot be pushed to multiple
// times. To overcome this obstacle, the test takes advantage of the
// fact that the reference name is a function of the reviewers and
// uses different reviewers for different review runs.
func TestLabelsInCommitMessage(t *testing.T) {
	ctx, cwd, root, repoPath, _, gerritPath := setupTest(t, true)
	defer teardownTest(t, ctx, cwd, root)
	branch := "my-branch"
	if err := ctx.Git().CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}

	// Test setting -presubmit=none and autosubmit.
	files := []string{"file1", "file2", "file3"}
	commitFiles(t, ctx, files)
	review, err := newReview(ctx, gerrit.CLOpts{
		Autosubmit: true,
		Presubmit:  gerrit.PresubmitTestTypeNone,
		Remote:     gerritPath,
		Reviewers:  parseEmails("run1"),
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	setTopicFlag = false
	if err := review.run(); err != nil {
		t.Fatalf("%v", err)
	}
	expectedRef := gerrit.Reference(review.CLOpts)
	assertFilesPushedToRef(t, ctx, repoPath, gerritPath, expectedRef, files)
	// The last three lines of the gerrit commit message file should be:
	// AutoSubmit
	// PresubmitTest: none
	// Change-Id: ...
	file, err := getCommitMessageFileName(review.ctx, review.CLOpts.Branch)
	if err != nil {
		t.Fatalf("%v", err)
	}
	bytes, err := ctx.Run().ReadFile(file)
	if err != nil {
		t.Fatalf("%v\n", err)
	}
	content := string(bytes)
	lines := strings.Split(content, "\n")
	// Make sure the Change-Id line is the last line.
	if got := lines[len(lines)-1]; !strings.HasPrefix(got, "Change-Id") {
		t.Fatalf("no Change-Id line found: %s", got)
	}
	// Make sure the "AutoSubmit" label exists.
	if autosubmitLabelRE.FindString(content) == "" {
		t.Fatalf("AutoSubmit label doesn't exist in the commit message: %s", content)
	}
	// Make sure the "PresubmitTest" label exists.
	if presubmitTestLabelRE.FindString(content) == "" {
		t.Fatalf("PresubmitTest label doesn't exist in the commit message: %s", content)
	}

	// Test setting -presubmit=all but keep autosubmit=true.
	review, err = newReview(ctx, gerrit.CLOpts{
		Autosubmit: true,
		Remote:     gerritPath,
		Reviewers:  parseEmails("run2"),
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := review.run(); err != nil {
		t.Fatalf("%v", err)
	}
	expectedRef = gerrit.Reference(review.CLOpts)
	assertFilesPushedToRef(t, ctx, repoPath, gerritPath, expectedRef, files)
	bytes, err = ctx.Run().ReadFile(file)
	if err != nil {
		t.Fatalf("%v\n", err)
	}
	content = string(bytes)
	// Make sure there is no PresubmitTest=none any more.
	match := presubmitTestLabelRE.FindString(content)
	if match != "" {
		t.Fatalf("want no presubmit label line, got: %s", match)
	}
	// Make sure the "AutoSubmit" label still exists.
	if autosubmitLabelRE.FindString(content) == "" {
		t.Fatalf("AutoSubmit label doesn't exist in the commit message: %s", content)
	}

	// Test setting autosubmit=false.
	review, err = newReview(ctx, gerrit.CLOpts{
		Remote:    gerritPath,
		Reviewers: parseEmails("run3"),
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := review.run(); err != nil {
		t.Fatalf("%v", err)
	}
	expectedRef = gerrit.Reference(review.CLOpts)
	assertFilesPushedToRef(t, ctx, repoPath, gerritPath, expectedRef, files)
	bytes, err = ctx.Run().ReadFile(file)
	if err != nil {
		t.Fatalf("%v\n", err)
	}
	content = string(bytes)
	// Make sure there is no AutoSubmit label any more.
	match = autosubmitLabelRE.FindString(content)
	if match != "" {
		t.Fatalf("want no AutoSubmit label line, got: %s", match)
	}
}

// TestDirtyBranch checks that the tool correctly handles unstaged and
// untracked changes in a working branch with stashed changes.
func TestDirtyBranch(t *testing.T) {
	ctx, cwd, root, _, _, gerritPath := setupTest(t, true)
	defer teardownTest(t, ctx, cwd, root)
	branch := "my-branch"
	if err := ctx.Git().CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}
	files := []string{"file1", "file2"}
	commitFiles(t, ctx, files)
	assertStashSize(t, ctx, 0)
	stashedFile, stashedFileContent := "stashed-file", "stashed-file content"
	if err := ctx.Run().WriteFile(stashedFile, []byte(stashedFileContent), 0644); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", stashedFile, stashedFileContent, err)
	}
	if err := ctx.Git().Add(stashedFile); err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := ctx.Git().Stash(); err != nil {
		t.Fatalf("%v", err)
	}
	assertStashSize(t, ctx, 1)
	modifiedFile, modifiedFileContent := "file1", "modified-file content"
	if err := ctx.Run().WriteFile(modifiedFile, []byte(modifiedFileContent), 0644); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", modifiedFile, modifiedFileContent, err)
	}
	stagedFile, stagedFileContent := "file2", "staged-file content"
	if err := ctx.Run().WriteFile(stagedFile, []byte(stagedFileContent), 0644); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", stagedFile, stagedFileContent, err)
	}
	if err := ctx.Git().Add(stagedFile); err != nil {
		t.Fatalf("%v", err)
	}
	untrackedFile, untrackedFileContent := "file3", "untracked-file content"
	if err := ctx.Run().WriteFile(untrackedFile, []byte(untrackedFileContent), 0644); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", untrackedFile, untrackedFileContent, err)
	}
	review, err := newReview(ctx, gerrit.CLOpts{Remote: gerritPath})
	if err != nil {
		t.Fatalf("%v", err)
	}
	setTopicFlag = false
	if err := review.run(); err == nil {
		t.Fatalf("run() didn't fail when it should")
	}
	assertFilesNotCommitted(t, ctx, []string{stagedFile})
	assertFilesNotCommitted(t, ctx, []string{untrackedFile})
	assertFileContent(t, ctx, modifiedFile, modifiedFileContent)
	assertFileContent(t, ctx, stagedFile, stagedFileContent)
	assertFileContent(t, ctx, untrackedFile, untrackedFileContent)
	// As of git 2.4.3 "git stash pop" fails if there are uncommitted
	// changes in the index. So we need to commit them first.
	if err := ctx.Git().Commit(); err != nil {
		t.Fatalf("%v", err)
	}
	assertStashSize(t, ctx, 1)
	if err := ctx.Git().StashPop(); err != nil {
		t.Fatalf("%v", err)
	}
	assertStashSize(t, ctx, 0)
	assertFilesNotCommitted(t, ctx, []string{stashedFile})
	assertFileContent(t, ctx, stashedFile, stashedFileContent)
}

// TestRunInSubdirectory checks that the command will succeed when run from
// within a subdirectory of a branch that does not exist on master branch, and
// will return the user to the subdirectory after completion.
func TestRunInSubdirectory(t *testing.T) {
	ctx, cwd, root, repoPath, _, gerritPath := setupTest(t, true)
	defer teardownTest(t, ctx, cwd, root)
	branch := "my-branch"
	if err := ctx.Git().CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}
	subdir := "sub/directory"
	subdirPerms := os.FileMode(0744)
	if err := ctx.Run().MkdirAll(subdir, subdirPerms); err != nil {
		t.Fatalf("MkdirAll(%v, %v) failed: %v", subdir, subdirPerms, err)
	}
	files := []string{path.Join(subdir, "file1")}
	commitFiles(t, ctx, files)
	chdir(t, ctx, subdir)
	review, err := newReview(ctx, gerrit.CLOpts{Remote: gerritPath})
	if err != nil {
		t.Fatalf("%v", err)
	}
	setTopicFlag = false
	if err := review.run(); err != nil {
		t.Fatalf("run() failed: %v", err)
	}
	path := path.Join(repoPath, subdir)
	want, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%v) failed: %v", path, err)
	}
	cwd, err = os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	got, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatalf("EvalSymlinks(%v) failed: %v", cwd, err)
	}
	if got != want {
		t.Fatalf("unexpected working directory: got %v, want %v", got, want)
	}
	expectedRef := gerrit.Reference(review.CLOpts)
	assertFilesPushedToRef(t, ctx, repoPath, gerritPath, expectedRef, files)
}

// TestProcessLabels checks that the processLabels function works as expected.
func TestProcessLabels(t *testing.T) {
	ctx, cwd, root, _, _, _ := setupTest(t, true)
	defer teardownTest(t, ctx, cwd, root)
	testCases := []struct {
		autosubmit      bool
		presubmitType   gerrit.PresubmitTestType
		originalMessage string
		expectedMessage string
	}{
		{
			presubmitType:   gerrit.PresubmitTestTypeNone,
			originalMessage: "",
			expectedMessage: "PresubmitTest: none\n",
		},
		{
			autosubmit:      true,
			presubmitType:   gerrit.PresubmitTestTypeNone,
			originalMessage: "",
			expectedMessage: "AutoSubmit\nPresubmitTest: none\n",
		},
		{
			presubmitType:   gerrit.PresubmitTestTypeNone,
			originalMessage: "review message\n",
			expectedMessage: "review message\nPresubmitTest: none\n",
		},
		{
			autosubmit:      true,
			presubmitType:   gerrit.PresubmitTestTypeNone,
			originalMessage: "review message\n",
			expectedMessage: "review message\nAutoSubmit\nPresubmitTest: none\n",
		},
		{
			presubmitType: gerrit.PresubmitTestTypeNone,
			originalMessage: `review message

Change-Id: I0000000000000000000000000000000000000000`,
			expectedMessage: `review message

PresubmitTest: none
Change-Id: I0000000000000000000000000000000000000000`,
		},
		{
			autosubmit:    true,
			presubmitType: gerrit.PresubmitTestTypeNone,
			originalMessage: `review message

Change-Id: I0000000000000000000000000000000000000000`,
			expectedMessage: `review message

AutoSubmit
PresubmitTest: none
Change-Id: I0000000000000000000000000000000000000000`,
		},
		{
			presubmitType:   gerrit.PresubmitTestTypeAll,
			originalMessage: "",
			expectedMessage: "",
		},
		{
			presubmitType:   gerrit.PresubmitTestTypeAll,
			originalMessage: "review message\n",
			expectedMessage: "review message\n",
		},
		{
			presubmitType: gerrit.PresubmitTestTypeAll,
			originalMessage: `review message

Change-Id: I0000000000000000000000000000000000000000`,
			expectedMessage: `review message

Change-Id: I0000000000000000000000000000000000000000`,
		},
	}
	for _, test := range testCases {
		review, err := newReview(ctx, gerrit.CLOpts{
			Autosubmit: test.autosubmit,
			Presubmit:  test.presubmitType,
		})
		if err != nil {
			t.Fatalf("%v", err)
		}
		if got := review.processLabels(test.originalMessage); got != test.expectedMessage {
			t.Fatalf("want %s, got %s", test.expectedMessage, got)
		}
	}
}

// TestCLNew checks the operation of the "v23 cl new" command.
func TestCLNew(t *testing.T) {
	ctx, cwd, root, _, _, _ := setupTest(t, true)
	defer teardownTest(t, ctx, cwd, root)

	// Create some dependent CLs.
	if err := newCL(ctx, []string{"feature1"}); err != nil {
		t.Fatalf("%v", err)
	}
	if err := newCL(ctx, []string{"feature2"}); err != nil {
		t.Fatalf("%v", err)
	}

	// Check that their dependency paths have been recorded correctly.
	testCases := []struct {
		branch string
		data   []byte
	}{
		{
			branch: "feature1",
			data:   []byte("master"),
		},
		{
			branch: "feature2",
			data:   []byte("master\nfeature1"),
		},
	}
	for _, testCase := range testCases {
		file, err := getDependencyPathFileName(ctx, testCase.branch)
		if err != nil {
			t.Fatalf("%v", err)
		}
		data, err := ctx.Run().ReadFile(file)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if bytes.Compare(data, testCase.data) != 0 {
			t.Fatalf("unexpected data:\ngot\n%v\nwant\n%v", string(data), string(testCase.data))
		}
	}
}

// TestParallelDev checks "v23 cl mail" behavior when parallel development has
// been submitted upstream.
func TestParallelDev(t *testing.T) {
	ctx, cwd, root, repoPath, originPath, gerritAPath := setupTest(t, true)
	gerritBPath := createRepoFromOrigin(t, ctx, root.Dir, "gerritB", originPath)
	chdir(t, ctx, repoPath)
	defer teardownTest(t, ctx, cwd, root)

	// Create parallel branches with:
	// * non-conflicting changes in different files
	// * conflicting changes in a file
	createCLWithFile(t, ctx, "feature1-A", "A")

	if err := ctx.Git().CheckoutBranch("master"); err != nil {
		t.Fatalf("%v", err)
	}
	createCLWithFile(t, ctx, "feature1-B", "B")
	commitFile(t, ctx, "A", "Don't tread on me.")

	reviewB, err := newReview(ctx, gerrit.CLOpts{Remote: gerritBPath})
	if err != nil {
		t.Fatalf("%v", err)
	}
	setTopicFlag = false
	if err := reviewB.run(); err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	// Submit B and verify A doesn't revert it.
	submit(t, ctx, originPath, gerritBPath, reviewB)

	// Assert files pushed to origin.
	chdir(t, ctx, originPath)
	assertFilesExist(t, ctx, []string{"A", "B"})
	chdir(t, ctx, repoPath)

	if err := ctx.Git().CheckoutBranch("feature1-A"); err != nil {
		t.Fatalf("%v", err)
	}

	reviewA, err := newReview(ctx, gerrit.CLOpts{Remote: gerritAPath})
	if err == nil {
		t.Fatalf("creating a review did not fail when it should")
	}
	// Assert state restored after failed review.
	assertFileContent(t, ctx, "A", "This is file A")
	assertFilesDoNotExist(t, ctx, []string{"B"})

	// Manual conflict resolution.
	if err := ctx.Git().Merge("master", gitutil.ResetOnFailureOpt(false)); err == nil {
		t.Fatalf("merge applied cleanly when it shouldn't")
	}
	assertFilesNotCommitted(t, ctx, []string{"A", "B"})
	assertFileContent(t, ctx, "B", "This is file B")

	if err := ctx.Run().WriteFile("A", []byte("This is file A. Don't tread on me."), 0644); err != nil {
		t.Fatalf("%v", err)
	}

	if err := ctx.Git().Add("A"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Git().Add("B"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Git().CommitWithMessage("Conflict resolution"); err != nil {
		t.Fatalf("%v", err)
	}

	// Retry review.
	reviewA, err = newReview(ctx, gerrit.CLOpts{Remote: gerritAPath})
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}

	if err := reviewA.run(); err != nil {
		t.Fatalf("run() failed: %v", err)
	}

	chdir(t, ctx, gerritAPath)
	expectedRef := gerrit.Reference(reviewA.CLOpts)
	if err := ctx.Git().CheckoutBranch(expectedRef); err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := ctx.Run().Stat("B"); err != nil {
		t.Fatalf("file B was not present in Gerrit: %v", err)
	}
}

// TestCLSync checks the operation of the "v23 cl sync" command.
func TestCLSync(t *testing.T) {
	ctx, cwd, root, _, _, _ := setupTest(t, true)
	defer teardownTest(t, ctx, cwd, root)

	// Create some dependent CLs.
	if err := newCL(ctx, []string{"feature1"}); err != nil {
		t.Fatalf("%v", err)
	}
	if err := newCL(ctx, []string{"feature2"}); err != nil {
		t.Fatalf("%v", err)
	}

	// Add the "test" file to the master.
	if err := ctx.Git().CheckoutBranch("master"); err != nil {
		t.Fatalf("%v", err)
	}
	commitFiles(t, ctx, []string{"test"})

	// Sync the dependent CLs.
	if err := ctx.Git().CheckoutBranch("feature2"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := syncCL(ctx); err != nil {
		t.Fatalf("%v", err)
	}

	// Check that the "test" file exists in the dependent CLs.
	for _, branch := range []string{"feature1", "feature2"} {
		if err := ctx.Git().CheckoutBranch(branch); err != nil {
			t.Fatalf("%v", err)
		}
		if _, err := ctx.Run().Stat("test"); err != nil {
			t.Fatalf("%v", err)
		}
	}
}
