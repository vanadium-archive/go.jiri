// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"v.io/jiri/runutil"
)

// PlatformSpecificGitArgs returns a git command line with platform specific,
// if any, modifications. The code is duplicated here because of the dependency
// structure in the jiri tool.
// TODO(cnicolaou,bprosnitz): remove this once ssl certs are installed.
func platformSpecificGitArgs(args ...string) []string {
	if os.Getenv("FNL_SYSTEM") != "" {
		// TODO(bprosnitz) Remove this after certificates are installed on FNL
		// Disable SSL verification because certificates are not present on FNL.func
		return append([]string{"-c", "http.sslVerify=false"}, args...)
	}
	return args
}

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
	s       runutil.Sequence
	opts    map[string]string
	rootDir string
}

type gitOpt interface {
	gitOpt()
}
type AuthorDateOpt string
type CommitterDateOpt string
type RootDirOpt string

func (AuthorDateOpt) gitOpt()    {}
func (CommitterDateOpt) gitOpt() {}
func (RootDirOpt) gitOpt()       {}

// New is the Git factory.
func New(s runutil.Sequence, opts ...gitOpt) *Git {
	rootDir := ""
	env := map[string]string{}
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case AuthorDateOpt:
			env["GIT_AUTHOR_DATE"] = string(typedOpt)
		case CommitterDateOpt:
			env["GIT_COMMITTER_DATE"] = string(typedOpt)
		case RootDirOpt:
			rootDir = string(typedOpt)
		}
	}
	return &Git{
		s:       s,
		opts:    env,
		rootDir: rootDir,
	}
}

// Add adds a file to staging.
func (g *Git) Add(file string) error {
	return g.run("add", file)
}

// AddRemote adds a new remote with the given name and path.
func (g *Git) AddRemote(name, path string) error {
	return g.run("remote", "add", name, path)
}

// BranchExists tests whether a branch with the given name exists in
// the local repository.
func (g *Git) BranchExists(branch string) bool {
	return g.run("show-branch", branch) == nil
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

// CheckoutBranch checks out the given branch.
func (g *Git) CheckoutBranch(branch string, opts ...CheckoutOpt) error {
	args := []string{"checkout"}
	force := false
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case ForceOpt:
			force = bool(typedOpt)
		}
	}
	if force {
		args = append(args, "-f")
	}
	args = append(args, branch)
	return g.run(args...)
}

// Clone clones the given repository to the given local path.
func (g *Git) Clone(repo, path string) error {
	return g.run("clone", repo, path)
}

// CloneRecursive clones the given repository recursively to the given local path.
func (g *Git) CloneRecursive(repo, path string) error {
	return g.run("clone", "--recursive", repo, path)
}

// Commit commits all files in staging with an empty message.
func (g *Git) Commit() error {
	return g.run("commit", "--allow-empty", "--allow-empty-message", "--no-edit")
}

// CommitAmend amends the previous commit with the currently staged
// changes. Empty commits are allowed.
func (g *Git) CommitAmend() error {
	return g.run("commit", "--amend", "--allow-empty", "--no-edit")
}

// CommitAmendWithMessage amends the previous commit with the
// currently staged changes, and the given message. Empty commits are
// allowed.
func (g *Git) CommitAmendWithMessage(message string) error {
	return g.run("commit", "--amend", "--allow-empty", "-m", message)
}

// CommitAndEdit commits all files in staging and allows the user to
// edit the commit message.
func (g *Git) CommitAndEdit() error {
	args := []string{"commit", "--allow-empty"}
	return g.runInteractive(args...)
}

// CommitFile commits the given file with the given commit message.
func (g *Git) CommitFile(fileName, message string) error {
	if err := g.Add(fileName); err != nil {
		return err
	}
	return g.CommitWithMessage(message)
}

// CommitMessages returns the concatenation of all commit messages on
// <branch> that are not also on <baseBranch>.
func (g *Git) CommitMessages(branch, baseBranch string) (string, error) {
	out, err := g.runOutput("log", "--no-merges", baseBranch+".."+branch)
	if err != nil {
		return "", err
	}
	return strings.Join(out, "\n"), nil
}

// CommitNoVerify commits all files in staging with the given
// message and skips all git-hooks.
func (g *Git) CommitNoVerify(message string) error {
	return g.run("commit", "--allow-empty", "--allow-empty-message", "--no-verify", "-m", message)
}

// CommitWithMessage commits all files in staging with the given
// message.
func (g *Git) CommitWithMessage(message string) error {
	return g.run("commit", "--allow-empty", "--allow-empty-message", "-m", message)
}

// CommitWithMessage commits all files in staging and allows the user
// to edit the commit message. The given message will be used as the
// default.
func (g *Git) CommitWithMessageAndEdit(message string) error {
	args := []string{"commit", "--allow-empty", "-e", "-m", message}
	return g.runInteractive(args...)
}

// Committers returns a list of committers for the current repository
// along with the number of their commits.
func (g *Git) Committers() ([]string, error) {
	out, err := g.runOutput("shortlog", "-s", "-n", "-e")
	if err != nil {
		return nil, err
	}
	return out, nil
}

// CountCommits returns the number of commits on <branch> that are not
// on <base>.
func (g *Git) CountCommits(branch, base string) (int, error) {
	args := []string{"rev-list", "--count", branch}
	if base != "" {
		args = append(args, "^"+base)
	}
	args = append(args, "--")
	out, err := g.runOutput(args...)
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
func (g *Git) CreateBranch(branch string) error {
	return g.run("branch", branch)
}

// CreateAndCheckoutBranch creates a new branch with the given name
// and checks it out.
func (g *Git) CreateAndCheckoutBranch(branch string) error {
	return g.run("checkout", "-b", branch)
}

// CreateBranchWithUpstream creates a new branch and sets the upstream
// repository to the given upstream.
func (g *Git) CreateBranchWithUpstream(branch, upstream string) error {
	return g.run("branch", branch, upstream)
}

// CurrentBranchName returns the name of the current branch.
func (g *Git) CurrentBranchName() (string, error) {
	out, err := g.runOutput("rev-parse", "--abbrev-ref", "HEAD")
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
	return g.CurrentRevisionOfBranch("HEAD")
}

// CurrentRevisionOfBranch returns the current revision of the given branch.
func (g *Git) CurrentRevisionOfBranch(branch string) (string, error) {
	out, err := g.runOutput("rev-parse", branch)
	if err != nil {
		return "", err
	}
	if got, want := len(out), 1; got != want {
		return "", fmt.Errorf("unexpected length of %v: got %v, want %v", out, got, want)
	}
	return out[0], nil
}

// DeleteBranch deletes the given branch.
func (g *Git) DeleteBranch(branch string, opts ...DeleteBranchOpt) error {
	args := []string{"branch"}
	force := false
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case ForceOpt:
			force = bool(typedOpt)
		}
	}
	if force {
		args = append(args, "-D")
	} else {
		args = append(args, "-d")
	}
	args = append(args, branch)
	return g.run(args...)
}

// Fetch fetches refs and tags from the given remote.
func (g *Git) Fetch(remote string) error {
	return g.run("fetch", remote)
}

// FetchRefspec fetches refs and tags from the given remote for a particular refspec.
func (g *Git) FetchRefspec(remote, refspec string) error {
	return g.run("fetch", remote, refspec)
}

// FilesWithUncommittedChanges returns the list of files that have
// uncommitted changes.
func (g *Git) FilesWithUncommittedChanges() ([]string, error) {
	out, err := g.runOutput("diff", "--name-only", "--no-ext-diff")
	if err != nil {
		return nil, err
	}
	out2, err := g.runOutput("diff", "--cached", "--name-only", "--no-ext-diff")
	if err != nil {
		return nil, err
	}
	return append(out, out2...), nil
}

// GetBranches returns a slice of the local branches of the current
// repository, followed by the name of the current branch. The
// behavior can be customized by providing optional arguments
// (e.g. --merged).
func (g *Git) GetBranches(args ...string) ([]string, string, error) {
	args = append([]string{"branch"}, args...)
	out, err := g.runOutput(args...)
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
	return g.run("ls-files", file, "--error-unmatch") == nil
}

// LatestCommitMessage returns the latest commit message on the
// current branch.
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

// Merge merges all commits from <branch> to the current branch. If
// <squash> is set, then all merged commits are squashed into a single
// commit.
func (g *Git) Merge(branch string, opts ...MergeOpt) error {
	args := []string{"merge"}
	squash := false
	strategy := ""
	resetOnFailure := true
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case SquashOpt:
			squash = bool(typedOpt)
		case StrategyOpt:
			strategy = string(typedOpt)
		case ResetOnFailureOpt:
			resetOnFailure = bool(typedOpt)
		}
	}
	if squash {
		args = append(args, "--squash")
	} else {
		args = append(args, "--no-squash")
	}
	if strategy != "" {
		args = append(args, fmt.Sprintf("--strategy=%v", strategy))
	}
	args = append(args, branch)
	if out, err := g.runOutput(args...); err != nil {
		if resetOnFailure {
			if err2 := g.run("reset", "--merge"); err2 != nil {
				return fmt.Errorf("%v\nCould not git reset while recovering from error: %v", err, err2)
			}
		}
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
	if _, err := g.s.Stat(mergeFile); err != nil {
		if runutil.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ModifiedFiles returns a slice of filenames that have changed
// between <baseBranch> and <currentBranch>.
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
	// Starting with git 1.8, "git pull <remote> <branch>" does not
	// create the branch "<remote>/<branch>" locally. To avoid the need
	// to account for this, run "git pull", which fails but creates the
	// missing branch, for git 1.7 and older.
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
func (g *Git) Push(remote, branch string, opts ...PushOpt) error {
	args := []string{"push"}
	verify := true
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case VerifyOpt:
			verify = bool(typedOpt)
		}
	}
	if verify {
		args = append(args, "--verify")
	} else {
		args = append(args, "--no-verify")
	}
	args = append(args, remote, branch)
	return g.run(args...)
}

// Rebase rebases to a particular upstream branch.
func (g *Git) Rebase(upstream string) error {
	return g.run("rebase", upstream)
}

// RebaseAbort aborts an in-progress rebase operation.
func (g *Git) RebaseAbort() error {
	return g.run("rebase", "--abort")
}

// Remove removes the given files.
func (g *Git) Remove(fileNames ...string) error {
	args := []string{"rm"}
	args = append(args, fileNames...)
	return g.run(args...)
}

// RemoteUrl gets the url of the remote with the given name.
func (g *Git) RemoteUrl(name string) (string, error) {
	configKey := fmt.Sprintf("remote.%s.url", name)
	out, err := g.runOutput("config", "--get", configKey)
	if err != nil {
		return "", err
	}
	if got, want := len(out), 1; got != want {
		return "", fmt.Errorf("RemoteUrl: unexpected length of remotes %v: got %v, want %v", out, got, want)
	}
	return out[0], nil
}

// RemoveUntrackedFiles removes untracked files and directories.
func (g *Git) RemoveUntrackedFiles() error {
	return g.run("clean", "-d", "-f")
}

// Reset resets the current branch to the target, discarding any
// uncommitted changes.
func (g *Git) Reset(target string, opts ...ResetOpt) error {
	args := []string{"reset"}
	mode := "hard"
	for _, opt := range opts {
		switch typedOpt := opt.(type) {
		case ModeOpt:
			mode = string(typedOpt)
		}
	}
	args = append(args, fmt.Sprintf("--%v", mode), target, "--")
	return g.run(args...)
}

// SetRemoteUrl sets the url of the remote with given name to the given url.
func (g *Git) SetRemoteUrl(name, url string) error {
	return g.run("remote", "set-url", name, url)
}

// Stash attempts to stash any unsaved changes. It returns true if
// anything was actually stashed, otherwise false. An error is
// returned if the stash command fails.
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
	out, err := g.runOutput("rev-parse", "--show-toplevel")
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
	out, err := g.runOutput("version")
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

func (g *Git) run(args ...string) error {
	var stdout, stderr bytes.Buffer
	capture := func(s runutil.Sequence) runutil.Sequence { return s.Capture(&stdout, &stderr) }
	if err := g.runWithFn(capture, args...); err != nil {
		return Error(stdout.String(), stderr.String(), args...)
	}
	return nil
}

func trimOutput(o string) []string {
	output := strings.TrimSpace(o)
	if len(output) == 0 {
		return nil
	}
	return strings.Split(output, "\n")
}

func (g *Git) runOutput(args ...string) ([]string, error) {
	var stdout, stderr bytes.Buffer
	fn := func(s runutil.Sequence) runutil.Sequence { return s.Capture(&stdout, &stderr) }
	if err := g.runWithFn(fn, args...); err != nil {
		return nil, Error(stdout.String(), stderr.String(), args...)
	}
	return trimOutput(stdout.String()), nil
}

func (g *Git) runInteractive(args ...string) error {
	var stderr bytes.Buffer
	// In order for the editing to work correctly with
	// terminal-based editors, notably "vim", use os.Stdout.
	capture := func(s runutil.Sequence) runutil.Sequence { return s.Capture(os.Stdout, &stderr) }
	if err := g.runWithFn(capture, args...); err != nil {
		return Error("", stderr.String(), args...)
	}
	return nil
}

func (g *Git) runWithFn(fn func(s runutil.Sequence) runutil.Sequence, args ...string) error {
	g.s.Dir(g.rootDir)
	args = platformSpecificGitArgs(args...)
	if fn == nil {
		fn = func(s runutil.Sequence) runutil.Sequence { return s }
	}
	return fn(g.s).Env(g.opts).Last("git", args...)
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
