// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package gitutil provides Go wrappers for various Git commands.
package gitutil

// TODO(sadovsky): This package should only be accessed via
// Context. We should make it internal somehow (e.g. via GO.PACKAGE,
// or Go 1.4's "internal").

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"v.io/x/devtools/internal/runutil"
)

const Force = true

type GitError struct {
	args        []string
	output      string
	errorOutput string
}

func Error(output, errorOutput string, args ...string) GitError {
	return GitError{
		args:        args,
		output:      output,
		errorOutput: errorOutput,
	}
}

func (ge GitError) Error() string {
	result := "'git "
	result += strings.Join(ge.args, " ")
	result += "' failed:\n"
	result += ge.errorOutput
	return result
}

type Git struct {
	r       *runutil.Run
	rootDir string
}

// New is the Git factory.
func New(r *runutil.Run, rootDir string) *Git {
	return &Git{
		r:       r,
		rootDir: rootDir,
	}
}

// Add adds a file to staging.
func (g *Git) Add(fileName string) error {
	return g.run("add", fileName)
}

// AddRemote adds a new remote with the given name and path.
func (g *Git) AddRemote(name, path string) error {
	return g.run("remote", "add", name, path)
}

// BranchExists tests whether a branch with the given name exists in the local
// repository.
func (g *Git) BranchExists(branchName string) bool {
	if err := g.run("show-branch", branchName); err != nil {
		return false
	}
	return true
}

// BranchesDiffer tests whether two branches have any changes between them.
func (g *Git) BranchesDiffer(branch1, branch2 string) (bool, error) {
	out, err := g.runOutput("--no-pager", "diff", "--name-only", branch1+".."+branch2)
	if err != nil {
		return false, err
	}
	// If output is empty, then there is no difference.
	if len(out) == 0 {
		return false, nil
	}
	// Otherwise there is a difference.
	return true, nil
}

// CheckoutBranch checks out a branch.
func (g *Git) CheckoutBranch(branchName string, force bool) error {
	if force {
		return g.run("checkout", "-f", branchName)
	} else {
		return g.run("checkout", branchName)
	}
}

// Clone clones the given repository to the given local path.
func (g *Git) Clone(repo, path string) error {
	return g.run("clone", repo, path)
}

// Commit commits all files in staging with an empty message.
func (g *Git) Commit() error {
	return g.run("commit", "--allow-empty", "--allow-empty-message", "--no-edit")
}

// CommitAmend amends the previous commit with the currently staged changes,
// and the given message.  Empty commits are allowed.
func (g *Git) CommitAmend(message string) error {
	return g.run("commit", "--amend", "--allow-empty", "-m", message)
}

// CommitAndEdit commits all files in staging and allows the user to edit the
// commit message.
func (g *Git) CommitAndEdit() error {
	args := []string{"commit", "--allow-empty"}
	var stderr bytes.Buffer
	// In order for the editing to work correctly with
	// terminal-based editors, notably "vim", use os.Stdout.
	opts := g.r.Opts()
	opts.Stdout = os.Stdout
	opts.Stderr = &stderr
	return g.commandWithOpts(opts, args...)
}

// CommitFile commits the given file with the given commit message.
func (g *Git) CommitFile(fileName, message string) error {
	if err := g.Add(fileName); err != nil {
		return err
	}
	return g.CommitWithMessage(message)
}

// CommitMessages returns the concatenation of all commit messages on <branch>
// that are not also on <baseBranch>.
func (g *Git) CommitMessages(branch, baseBranch string) (string, error) {
	out, err := g.runOutput("log", "--no-merges", baseBranch+".."+branch)
	if err != nil {
		return "", err
	}
	return strings.Join(out, "\n"), nil
}

// CommitWithMessage commits all files in staging with the given message.
func (g *Git) CommitWithMessage(message string) error {
	return g.run("commit", "--allow-empty", "--allow-empty-message", "-m", message)
}

// CommitWithMessage commits all files in staging and allows the user to edit
// the commit message. The given message will be used as the default.
func (g *Git) CommitWithMessageAndEdit(message string) error {
	args := []string{"commit", "--allow-empty", "-e", "-m", message}
	var stderr bytes.Buffer
	// In order for the editing to work correctly with
	// terminal-based editors, notably "vim", use os.Stdout.
	opts := g.r.Opts()
	opts.Stdout = os.Stdout
	opts.Stderr = &stderr
	return g.commandWithOpts(opts, args...)
}

// Committers returns a list of committers for the current repository
// along with the number of their commits.
func (g *Git) Committers() ([]string, error) {
	out, err := g.runOutputWithOpts(g.disableDryRun(), "shortlog", "-s", "-n", "-e")
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CountCommits returns the number of commits on <branch> that are not on <base>.
func (g *Git) CountCommits(branch, base string) (int, error) {
	args := []string{"rev-list", "--count", branch}
	if base != "" {
		args = append(args, "^"+base)
	}
	out, err := g.runOutputWithOpts(g.disableDryRun(), args...)
	if err != nil {
		return 0, err
	}
	if got, want := len(out), 1; got != want {
		return 0, fmt.Errorf("unexpected length of %v: got %v, want %v", out, got, want)
	}
	count, err := strconv.Atoi(out[0])
	if err != nil {
		return 0, fmt.Errorf("Atoi(%v) failed: %v", out[0], err)
	}
	return count, nil
}

// CreateBranch creates a new branch with the given name.
func (g *Git) CreateBranch(branchName string) error {
	return g.run("branch", branchName)
}

// CreateAndCheckoutBranch creates a branch with the given name and checks it
// out.
func (g *Git) CreateAndCheckoutBranch(branchName string) error {
	return g.run("checkout", "-b", branchName)
}

// CreateBranchWithUpstream creates a new branch and sets the upstream
// repository to the given upstream.
func (g *Git) CreateBranchWithUpstream(branchName, upstream string) error {
	return g.run("branch", branchName, upstream)
}

// CurrentBranchName returns the name of the current branch.
func (g *Git) CurrentBranchName() (string, error) {
	out, err := g.runOutputWithOpts(g.disableDryRun(), "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if got, want := len(out), 1; got != want {
		return "", fmt.Errorf("unexpected length of %v: got %v, want %v", out, got, want)
	}
	return out[0], nil
}

// CurrentRevision returns the current revision.
func (g *Git) CurrentRevision() (string, error) {
	out, err := g.runOutputWithOpts(g.disableDryRun(), "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if got, want := len(out), 1; got != want {
		return "", fmt.Errorf("unexpected length of %v: got %v, want %v", out, got, want)
	}
	return out[0], nil
}

// DeleteBranch deletes the given branch.
func (g *Git) DeleteBranch(branchName string, force bool) error {
	if force {
		return g.run("branch", "-D", branchName)
	} else {
		return g.run("branch", "-d", branchName)
	}
}

// Fetch fetches refs and tags from the given remote.
func (g *Git) Fetch(remote, branch string) error {
	return g.run("fetch", remote, branch)
}

// FilesWithUncommittedChanges returns the list of files that have
// uncommitted changes.
func (g *Git) FilesWithUncommittedChanges() ([]string, error) {
	out, err := g.runOutput("diff", "--name-only", "--no-ext-diff")
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GetBranches returns a slice of the local branches of the current
// repository, followed by the name of the current branch. The
// behavior can be customized by providing optional arguments
// (e.g. --merged).
func (g *Git) GetBranches(args ...string) ([]string, string, error) {
	args = append([]string{"branch"}, args...)
	out, err := g.runOutputWithOpts(g.disableDryRun(), args...)
	if err != nil {
		return nil, "", err
	}
	branches, current := []string{}, ""
	for _, branch := range out {
		if strings.HasPrefix(branch, "*") {
			branch = strings.TrimSpace(strings.TrimPrefix(branch, "*"))
			current = branch
		}
		branches = append(branches, strings.TrimSpace(branch))
	}
	return branches, current, nil
}

// HasUncommittedChanges checks whether the current branch contains
// any uncommitted changes.
func (g *Git) HasUncommittedChanges() (bool, error) {
	out, err := g.FilesWithUncommittedChanges()
	if err != nil {
		return false, err
	}
	return len(out) != 0, nil
}

// HasUntrackedFiles checks whether the current branch contains any
// untracked files.
func (g *Git) HasUntrackedFiles() (bool, error) {
	out, err := g.UntrackedFiles()
	if err != nil {
		return false, err
	}
	return len(out) != 0, nil
}

// Init initializes a new git repository.
func (g *Git) Init(path string) error {
	return g.run("init", path)
}

// IsFileCommitted tests whether the given file has been committed to
// the repository.
func (g *Git) IsFileCommitted(file string) bool {
	// Check if file is still in staging enviroment.
	if out, _ := g.runOutput("status", "--porcelain", file); len(out) > 0 {
		return false
	}
	// Check if file is unknown to git.
	if err := g.run("ls-files", file, "--error-unmatch"); err != nil {
		return false
	}
	return true
}

// LatestCommitMessage returns the latest commit message on the current branch.
func (g *Git) LatestCommitMessage() (string, error) {
	out, err := g.runOutput("log", "-n", "1", "--format=format:%B")
	if err != nil {
		return "", err
	}
	return strings.Join(out, "\n"), nil
}

// Log returns a list of commits on <branch> that are not on <base>,
// using the specified format.
func (g *Git) Log(branch, base, format string) ([][]string, error) {
	n, err := g.CountCommits(branch, base)
	if err != nil {
		return nil, err
	}
	result := [][]string{}
	for i := 0; i < n; i++ {
		skipArg := fmt.Sprintf("--skip=%d", i)
		formatArg := fmt.Sprintf("--format=%s", format)
		branchArg := fmt.Sprintf("%v..%v", base, branch)
		out, err := g.runOutput("log", "-1", skipArg, formatArg, branchArg)
		if err != nil {
			return nil, err
		}
		result = append(result, out)
	}
	return result, nil
}

// Merge merge all commits from <branch> to the current branch. If
// <squash> is set, then all merged commits are squashed into a single
// commit.
func (g *Git) Merge(branch string, squash bool) error {
	args := []string{"merge"}
	if squash {
		args = append(args, "--squash")
	}
	args = append(args, branch)
	if out, err := g.runOutput(args...); err != nil {
		g.run("reset", "--merge")
		return fmt.Errorf("%v\n%v", err, strings.Join(out, "\n"))
	}
	return nil
}

// MergeInProgress returns a boolean flag that indicates if a merge
// operation is in progress for the current repository.
func (g *Git) MergeInProgress() (bool, error) {
	repoRoot, err := g.TopLevel()
	if err != nil {
		return false, err
	}
	mergeFile := filepath.Join(repoRoot, ".git", "MERGE_HEAD")
	if _, err := os.Stat(mergeFile); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ModifiedFiles returns a slice of filenames that have changed between
// <baseBranch> and <currentBranch>.
func (g *Git) ModifiedFiles(baseBranch, currentBranch string) ([]string, error) {
	out, err := g.runOutput("diff", "--name-only", baseBranch+".."+currentBranch)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Pull pulls the given branch from the given remote.
func (g *Git) Pull(remote, branch string) error {
	if out, err := g.runOutput("pull", remote, branch); err != nil {
		g.run("reset", "--merge")
		return fmt.Errorf("%v\n%v", err, strings.Join(out, "\n"))
	}
	major, minor, err := g.Version()
	if err != nil {
		return err
	}
	// Starting with git 1.8, "git pull <remote> <branch>" does not create the
	// branch "<remote>/<branch>" locally. To avoid the need to account for this,
	// run "git pull", which fails but creates the missing branch, for git 1.7 and
	// older.
	if major < 2 && minor < 8 {
		// This command is expected to fail (with desirable side effects).
		// Use exec.Command instead of runner to prevent this failure from
		// showing up in the console and confusing people.
		command := exec.Command("git", "pull")
		command.Run()
	}
	return nil
}

// Push pushes the given branch to the given remote.
func (g *Git) Push(remote, branch string) error {
	return g.run("push", remote, branch)
}

// RebaseAbort aborts an in-progress rebase operation.
func (g *Git) RebaseAbort() error {
	return g.run("rebase", "--abort")
}

// Remove removes the given file.
func (g *Git) Remove(fileName string) error {
	return g.run("rm", fileName)
}

// RemoveUntrackedFiles removes untracked files and directories.
func (g *Git) RemoveUntrackedFiles() error {
	return g.run("clean", "-d", "-f")
}

// RepoName gets the name of the current repository.
func (g *Git) RepoName() (string, error) {
	out, err := g.runOutputWithOpts(g.disableDryRun(), "config", "--get", "remote.origin.url")
	if err != nil {
		return "", err
	}
	if got, want := len(out), 1; got != want {
		return "", fmt.Errorf("unexpected length of %v: got %v, want %v", out, got, want)
	}
	return out[0], nil
}

// Reset resets the current branch to the target, discarding any
// uncommitted changes.
func (g *Git) Reset(target string) error {
	return g.run("reset", "--hard", target)
}

// Stash attempts to stash any unsaved changes. It returns true if anything was
// actually stashed, otherwise false. An error is returned if the stash command
// fails.
func (g *Git) Stash() (bool, error) {
	oldSize, err := g.StashSize()
	if err != nil {
		return false, err
	}
	if err := g.run("stash", "save"); err != nil {
		return false, err
	}
	newSize, err := g.StashSize()
	if err != nil {
		return false, err
	}
	return newSize > oldSize, nil
}

// StashSize returns the size of the stash stack.
func (g *Git) StashSize() (int, error) {
	out, err := g.runOutput("stash", "list")
	if err != nil {
		return 0, err
	}
	// If output is empty, then stash is empty.
	if len(out) == 0 {
		return 0, nil
	}
	// Otherwise, stash size is the length of the output.
	return len(out), nil
}

// StashPop pops the stash into the current working tree.
func (g *Git) StashPop() error {
	return g.run("stash", "pop")
}

// TopLevel returns the top level path of the current repository.
func (g *Git) TopLevel() (string, error) {
	// TODO(sadovsky): If g.rootDir is set, perhaps simply return that?
	out, err := g.runOutputWithOpts(g.disableDryRun(), "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.Join(out, "\n"), nil
}

// TrackedFiles returns the list of files that are tracked.
func (g *Git) TrackedFiles() ([]string, error) {
	out, err := g.runOutput("ls-files")
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UntrackedFiles returns the list of files that are not tracked.
func (g *Git) UntrackedFiles() ([]string, error) {
	out, err := g.runOutput("ls-files", "--others", "--directory", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Version returns the major and minor git version.
func (g *Git) Version() (int, int, error) {
	out, err := g.runOutputWithOpts(g.disableDryRun(), "version")
	if err != nil {
		return 0, 0, err
	}
	if got, want := len(out), 1; got != want {
		return 0, 0, fmt.Errorf("unexpected length of %v: got %v, want %v", out, got, want)
	}
	words := strings.Split(out[0], " ")
	if got, want := len(words), 3; got < want {
		return 0, 0, fmt.Errorf("unexpected length of %v: got %v, want at least %v", words, got, want)
	}
	version := strings.Split(words[2], ".")
	if got, want := len(version), 3; got < want {
		return 0, 0, fmt.Errorf("unexpected length of %v: got %v, want at least %v", version, got, want)
	}
	major, err := strconv.Atoi(version[0])
	if err != nil {
		return 0, 0, fmt.Errorf("failed parsing %q to integer", major)
	}
	minor, err := strconv.Atoi(version[1])
	if err != nil {
		return 0, 0, fmt.Errorf("failed parsing %q to integer", minor)
	}
	return major, minor, nil
}

func (g *Git) disableDryRun() runutil.Opts {
	opts := g.r.Opts()
	if opts.DryRun {
		// Disable the dry run option as this function has no
		// effect and doing so results in more informative
		// "dry run" output.
		opts.DryRun = false
		opts.Verbose = true
	}
	return opts
}

func (g *Git) commandWithOpts(opts runutil.Opts, args ...string) error {
	// http://git-scm.com/docs/git
	//
	if g.rootDir != "" {
		opts.Dir = g.rootDir
	}
	if err := g.r.CommandWithOpts(opts, "git", args...); err != nil {
		stdout, stderr := "", ""
		buf, ok := opts.Stdout.(*bytes.Buffer)
		if ok {
			stdout = buf.String()
		}
		buf, ok = opts.Stderr.(*bytes.Buffer)
		if ok {
			stderr = buf.String()
		}
		return Error(stdout, stderr, args...)
	}
	return nil
}

func (g *Git) run(args ...string) error {
	var stdout, stderr bytes.Buffer
	opts := g.r.Opts()
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	return g.commandWithOpts(opts, args...)
}

func (g *Git) runOutput(args ...string) ([]string, error) {
	return g.runOutputWithOpts(g.r.Opts(), args...)
}

func (g *Git) runOutputWithOpts(opts runutil.Opts, args ...string) ([]string, error) {
	var stdout, stderr bytes.Buffer
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	if err := g.commandWithOpts(opts, args...); err != nil {
		return nil, err
	}
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, nil
	} else {
		return strings.Split(output, "\n"), nil
	}
}

// Committer encapsulates the process of create a commit.
type Committer struct {
	commit            func() error
	commitWithMessage func(message string) error
}

// Commit creates a commit.
func (c *Committer) Commit(message string) error {
	if len(message) == 0 {
		// No commit message supplied, let git supply one.
		return c.commit()
	}
	return c.commitWithMessage(message)
}

// NewCommitter is the Committer factory. The boolean <edit> flag
// determines whether the commit commands should prompt users to edit
// the commit message. This flag enables automated testing.
func (g *Git) NewCommitter(edit bool) *Committer {
	if edit {
		return &Committer{
			commit:            g.CommitAndEdit,
			commitWithMessage: g.CommitWithMessageAndEdit,
		}
	} else {
		return &Committer{
			commit:            g.Commit,
			commitWithMessage: g.CommitWithMessage,
		}
	}
}
