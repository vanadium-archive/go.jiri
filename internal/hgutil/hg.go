// Package hgutil provides Go wrappers for various Mercurial commands.
package hgutil

// TODO(sadovsky): This package should only be accessed via Context. We should
// make it internal somehow (e.g. via GO.PACKAGE, or Go 1.4's "internal").

import (
	"bytes"
	"fmt"
	"strings"

	"v.io/x/devtools/internal/runutil"
)

type HgError struct {
	args        []string
	output      string
	errorOutput string
}

func Error(output, errorOutput string, args ...string) HgError {
	return HgError{
		args:        args,
		output:      output,
		errorOutput: errorOutput,
	}
}

func (he HgError) Error() string {
	result := "'hg "
	result += strings.Join(he.args, " ")
	result += "' failed:\n"
	result += he.errorOutput
	return result
}

type Hg struct {
	r       *runutil.Run
	rootDir string
}

// New is the Hg factory.
func New(r *runutil.Run, rootDir string) *Hg {
	return &Hg{
		r:       r,
		rootDir: rootDir,
	}
}

// CheckoutBranch switches the current repository to the given branch.
func (h *Hg) CheckoutBranch(branch string) error {
	return h.run("update", branch)
}

// CheckoutRevision switches the revision for the current repository.
func (h *Hg) CheckoutRevision(revision string) error {
	return h.run("update", "-r", revision)
}

// Clone clones the given repository to the given local path.
func (h *Hg) Clone(repo, path string) error {
	return h.run("clone", repo, path)
}

// CurrentBranchName returns the name of the current branch.
func (h *Hg) CurrentBranchName() (string, error) {
	out, err := h.runOutputWithOpts(h.disableDryRun(), "branch")
	if err != nil {
		return "", err
	}
	if expected, got := 1, len(out); expected != got {
		return "", fmt.Errorf("unexpected length of %v: expected %v, got %v", out, expected, got)
	}
	return strings.Join(out, "\n"), nil
}

// GetBranches returns a slice of the local branches of the current
// repository, followed by the name of the current branch.
func (h *Hg) GetBranches() ([]string, string, error) {
	current, err := h.CurrentBranchName()
	if err != nil {
		return nil, "", err
	}
	out, err := h.runOutput("branches")
	if err != nil {
		return nil, "", err
	}
	branches := []string{}
	for _, branch := range out {
		branches = append(branches, strings.TrimSpace(branch))
	}
	return branches, current, nil
}

// Pull updates the current branch from the remote repository.
func (h *Hg) Pull() error {
	return h.run("pull", "-u")
}

// RepoName gets the name of the current repository.
func (h *Hg) RepoName() (string, error) {
	out, err := h.runOutputWithOpts(h.disableDryRun(), "paths", "default")
	if err != nil {
		return "", err
	}
	if expected, got := 1, len(out); expected != got {
		return "", fmt.Errorf("unexpected length of %v: expected %v, got %v", out, expected, got)
	}
	return out[0], nil
}

func (h *Hg) disableDryRun() runutil.Opts {
	opts := h.r.Opts()
	if opts.DryRun {
		// Disable the dry run option as this function has no
		// effect and doing so results in more informative
		// "dry run" output.
		opts.DryRun = false
		opts.Verbose = true
	}
	return opts
}

func (h *Hg) commandWithOpts(opts runutil.Opts, args ...string) error {
	// http://www.selenic.com/mercurial/hg.1.html
	if h.rootDir != "" {
		args = append([]string{"-R", h.rootDir}, args...)
	}
	if err := h.r.CommandWithOpts(opts, "hg", args...); err != nil {
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

func (h *Hg) run(args ...string) error {
	var stdout, stderr bytes.Buffer
	opts := h.r.Opts()
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	return h.commandWithOpts(opts, args...)
}

func (h *Hg) runOutput(args ...string) ([]string, error) {
	return h.runOutputWithOpts(h.r.Opts(), args...)
}

func (h *Hg) runOutputWithOpts(opts runutil.Opts, args ...string) ([]string, error) {
	var stdout, stderr bytes.Buffer
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	if err := h.commandWithOpts(opts, args...); err != nil {
		return nil, err
	}
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, nil
	} else {
		return strings.Split(output, "\n"), nil
	}
}
