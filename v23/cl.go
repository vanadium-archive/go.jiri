// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/gerrit"
	"v.io/x/devtools/internal/gitutil"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

const commitMessageFile = ".gerrit_commit_message"

var (
	apiFlag         bool
	ccsFlag         string
	copyrightFlag   bool
	draftFlag       bool
	depcopFlag      bool
	editFlag        bool
	forceFlag       bool
	gofmtFlag       bool
	govetFlag       bool
	govetBinaryFlag string
	presubmitFlag   string
	reviewersFlag   string
	uncommittedFlag bool
)

// init carries out the package initialization.
func init() {
	cmdCLCleanup.Flags.BoolVar(&forceFlag, "f", false, "Ignore unmerged changes.")
	cmdCLMail.Flags.BoolVar(&apiFlag, "check-api", true, "Check for changes in the public Go API.")
	cmdCLMail.Flags.StringVar(&ccsFlag, "cc", "", "Comma-seperated list of emails or LDAPs to cc.")
	cmdCLMail.Flags.BoolVar(&copyrightFlag, "check-copyright", true, "Check copyright headers.")
	cmdCLMail.Flags.BoolVar(&depcopFlag, "check-godepcop", true, "Check that no godepcop violations exist.")
	cmdCLMail.Flags.BoolVar(&draftFlag, "d", false, "Send a draft changelist.")
	cmdCLMail.Flags.BoolVar(&editFlag, "edit", true, "Open an editor to edit the commit message.")
	cmdCLMail.Flags.BoolVar(&gofmtFlag, "check-gofmt", true, "Check that no go fmt violations exist.")
	cmdCLMail.Flags.BoolVar(&govetFlag, "check-govet", true, "Check that no go vet violations exist.")
	cmdCLMail.Flags.StringVar(&govetBinaryFlag, "go-vet-binary", "", "Specify the path to the go vet binary to use.")
	cmdCLMail.Flags.StringVar(&presubmitFlag, "presubmit", string(gerrit.PresubmitTestTypeAll),
		fmt.Sprintf("The type of presubmit tests to run. Valid values: %s.", strings.Join(gerrit.PresubmitTestTypes(), ",")))
	cmdCLMail.Flags.StringVar(&reviewersFlag, "r", "", "Comma-seperated list of emails or LDAPs to request review.")
	cmdCLMail.Flags.BoolVar(&uncommittedFlag, "check-uncommitted", true, "Check that no uncommitted changes exist.")
}

// cmdCL represents the "v23 cl" command.
var cmdCL = &cmdline.Command{
	Name:     "cl",
	Short:    "Manage vanadium changelists",
	Long:     "Manage vanadium changelists.",
	Children: []*cmdline.Command{cmdCLCleanup, cmdCLMail},
}

// cmdCLCleanup represents the "v23 cl cleanup" command.
//
// TODO(jsimsa): Make this part of the "submit" command".
var cmdCLCleanup = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runCLCleanup),
	Name:   "cleanup",
	Short:  "Clean up branches that have been merged",
	Long: `
The cleanup command checks that the given branches have been merged
into the master branch. If a branch differs from the master, it
reports the difference and stops. Otherwise, it deletes the branch.
`,
	ArgsName: "<branches>",
	ArgsLong: "<branches> is a list of branches to cleanup.",
}

func cleanup(ctx *tool.Context, branches []string) (e error) {
	originalBranch, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return err
	}
	stashed, err := ctx.Git().Stash()
	if err != nil {
		return err
	}
	if stashed {
		defer collect.Error(func() error { return ctx.Git().StashPop() }, &e)
	}
	if err := ctx.Git().CheckoutBranch("master", !gitutil.Force); err != nil {
		return err
	}
	checkoutOriginalBranch := true
	defer collect.Error(func() error {
		if checkoutOriginalBranch {
			return ctx.Git().CheckoutBranch(originalBranch, !gitutil.Force)
		}
		return nil
	}, &e)
	if err := ctx.Git().Pull("origin", "master"); err != nil {
		return err
	}
	for _, branch := range branches {
		cleanupFn := func() error { return cleanupBranch(ctx, branch) }
		if err := ctx.Run().Function(cleanupFn, "Cleaning up branch %q", branch); err != nil {
			return err
		}
		if branch == originalBranch {
			checkoutOriginalBranch = false
		}
	}
	return nil
}

func cleanupBranch(ctx *tool.Context, branch string) error {
	if err := ctx.Git().CheckoutBranch(branch, !gitutil.Force); err != nil {
		return err
	}
	if !forceFlag {
		if err := ctx.Git().Merge("master", false); err != nil {
			return err
		}
		files, err := ctx.Git().ModifiedFiles("master", branch)
		if err != nil {
			return err
		}
		// A feature branch is considered merged with
		// the master, when there are no differences
		// or the only difference is the gerrit commit
		// message file.
		if len(files) != 0 && (len(files) != 1 || files[0] != commitMessageFile) {
			return fmt.Errorf("unmerged changes in\n%s", strings.Join(files, "\n"))
		}
	}
	if err := ctx.Git().CheckoutBranch("master", !gitutil.Force); err != nil {
		return err
	}
	if err := ctx.Git().DeleteBranch(branch, gitutil.Force); err != nil {
		return err
	}
	reviewBranch := branch + "-REVIEW"
	if ctx.Git().BranchExists(reviewBranch) {
		if err := ctx.Git().DeleteBranch(reviewBranch, gitutil.Force); err != nil {
			return err
		}
	}
	return nil
}

func runCLCleanup(env *cmdline.Env, args []string) error {
	if len(args) == 0 {
		return env.UsageErrorf("cleanup requires at least one argument")
	}
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	return cleanup(ctx, args)
}

// cmdCLMail represents the "v23 cl mail" command.
var cmdCLMail = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runCLMail),
	Name:   "mail",
	Short:  "Mail a changelist based on the current branch to Gerrit for review",
	Long: `
Squashes all commits of a local branch into a single "changelist" and
mails this changelist to Gerrit as a single commit. First time the
command is invoked, it generates a Change-Id for the changelist, which
is appended to the commit message. Consecutive invocations of the
command use the same Change-Id by default, informing Gerrit that the
incomming commit is an update of an existing changelist.
`,
}

type apiError struct {
	apiCheckOutput string
	project        string
}

func (a apiError) Error() string {
	result := "changelist changes the public Go API without updating the corresponding .api file(s)\n\n"
	result += "For a detailed account of these changes, run 'v23 api check " + a.project + "'\n"
	result += "If these changes are intentional, run 'v23 api fix " + a.project + "'\n"
	result += "to update the corresponding .api files. Then add the updated .api files to\n"
	result += "your changelist and re-run the mail command.\n\n"
	result += a.apiCheckOutput
	return result
}

type changeConflictError string

func (s changeConflictError) Error() string {
	result := "changelist conflicts with the remote master branch\n\n"
	result += "To resolve this problem, run 'v23 update; git merge master',\n"
	result += "resolve the conflicts identified below, and then try again.\n"
	result += string(s)
	return result
}

type copyrightError string

func (s copyrightError) Error() string {
	result := "changelist does not adhere to the copyright conventions\n\n"
	result += "To resolve this problem, run 'v23 copyright fix <project>' to\n"
	result += "fix the following violations:\n"
	result += string(s)
	return result
}

type emptyChangeError struct{}

func (_ emptyChangeError) Error() string {
	return "current branch has no commits"
}

type gerritError string

func (s gerritError) Error() string {
	result := "sending code review failed\n\n"
	result += string(s)
	return result
}

type goDependencyError string

func (s goDependencyError) Error() string {
	result := "changelist introduces dependency violations\n\n"
	result += "To resolve this problem, fix the following violations:\n"
	result += string(s)
	return result
}

type goFormatError string

func (s goFormatError) Error() string {
	result := "changelist does not adhere to the Go formatting conventions\n\n"
	result += "To resolve this problem, run 'gofmt -w' for the following file(s):\n"
	result += string(s)
	return result
}

type goVetError []string

func (s goVetError) Error() string {
	result := "changelist contains 'go vet' violation(s)\n\n"
	result += "To resolve this problem, fix the following violations:\n"
	result += "  " + strings.Join(s, "\n  ")
	return result
}

type noChangeIDError struct{}

func (_ noChangeIDError) Error() string {
	result := "changelist is missing a Change-ID"
	return result
}

type uncommittedChangesError []string

func (s uncommittedChangesError) Error() string {
	result := "uncommitted local changes in files:\n"
	result += "  " + strings.Join(s, "\n  ")
	return result
}

var defaultMessageHeader = `
# Describe your changelist, specifying what package(s) your change
# pertains to, followed by a short summary and, in case of non-trivial
# changelists, provide a detailed description.
#
# For example:
#
# rpc/stream/proxy: add publish address
#
# The listen address is not always the same as the address that external
# users need to connect to. This CL adds a new argument to proxy.New()
# to specify the published address that clients should connect to.

# FYI, you are about to submit the following local commits for review:
#
`

// runCLMail is a wrapper that sets up and runs a review instance.
func runCLMail(env *cmdline.Env, _ []string) error {
	// Sanity checks for the presubmitFlag.
	if !checkPresubmitFlag() {
		return env.UsageErrorf("Invalid value for -presubmit flag. Valid values: %s.",
			strings.Join(gerrit.PresubmitTestTypes(), ","))
	}
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	opts := reviewOpts{
		ccs:       ccsFlag,
		draft:     draftFlag,
		edit:      editFlag,
		presubmit: gerrit.PresubmitTestType(presubmitFlag),
		reviewers: reviewersFlag,
	}
	review, err := newReview(ctx, opts)
	if err != nil {
		return err
	}

	// Ask users to confirm when they changed the presubmit flag.
	commitMessageFileName, err := review.getCommitMessageFilename()
	if err != nil {
		return err
	}
	bytes, err := ioutil.ReadFile(commitMessageFileName)
	if err == nil {
		prevPresubmitType := string(gerrit.PresubmitTestTypeAll)
		content := string(bytes)
		matches := presubmitTestLabelRE.FindStringSubmatch(content)
		if matches != nil {
			prevPresubmitType = matches[1]
		}
		if presubmitFlag != prevPresubmitType {
			fmt.Printf("Are you sure you want to change presubmit=%s to presubmit=%s? (y/n): ", prevPresubmitType, presubmitFlag)
			var response string
			if _, err := fmt.Scanf("%s\n", &response); err != nil || response != "y" {
				return nil
			}
		}
	}

	return review.run()
}

func checkPresubmitFlag() bool {
	validPresubmitTestTypes := gerrit.PresubmitTestTypes()
	for _, t := range validPresubmitTestTypes {
		if presubmitFlag == t {
			return true
		}
	}
	return false
}

// review holds the state of a review.
type review struct {
	// branch is the name of the git branch from which the review is created.
	branch string
	// ccs is the list of LDAPs or emails to cc on the review.
	ccs string
	// ctx is an instance of the command-line tool context.
	ctx *tool.Context
	// draft indicates whether to create a draft review.
	draft bool
	// edit indicates whether to edit the review message.
	edit bool
	// the type of presubmit tests to run.
	presubmit gerrit.PresubmitTestType
	// repo is the name of the gerrit repository.
	repo string
	// reviewBranch is the name of the temporary git branch used to send the review.
	reviewBranch string
	// reviewers is the list of LDAPs or emails to request a review from.
	reviewers string
}

type reviewOpts struct {
	ccs       string
	draft     bool
	edit      bool
	presubmit gerrit.PresubmitTestType
	repo      string
	reviewers string
}

// newReview is the review factory.
//
// TODO(jingjin): use optional arguments.
func newReview(ctx *tool.Context, opts reviewOpts) (*review, error) {
	branch, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return nil, err
	}
	reviewBranch := branch + "-REVIEW"
	if opts.presubmit == gerrit.PresubmitTestType("") {
		opts.presubmit = gerrit.PresubmitTestTypeAll // use gerrit.PresubmitTestTypeAll as the default
	}
	return &review{
		branch:       branch,
		ccs:          opts.ccs,
		ctx:          ctx,
		draft:        opts.draft,
		edit:         opts.edit,
		presubmit:    opts.presubmit,
		repo:         opts.repo,
		reviewBranch: reviewBranch,
		reviewers:    opts.reviewers,
	}, nil
}

// Change-Ids start with 'I' and are followed by 40 characters of hex.
var changeIDRE *regexp.Regexp = regexp.MustCompile("Change-Id: I[0123456789abcdefABCDEF]{40}")

// Presubmit test label.
// PresubmitTest: <type>
var presubmitTestLabelRE *regexp.Regexp = regexp.MustCompile(`PresubmitTest:\s*(.*)`)

// checkGoFormat checks if the code to be submitted needs to be
// formatted with "go fmt".
func (r *review) checkGoFormat() error {
	goFiles, err := r.modifiedGoFiles()
	if err != nil || len(goFiles) == 0 {
		return err
	}
	// Check if the formatting differs from gofmt.
	var out bytes.Buffer
	opts := r.ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	args := []string{"run", "gofmt", "-l"}
	args = append(args, goFiles...)
	if err := r.ctx.Run().CommandWithOpts(opts, "v23", args...); err != nil {
		return err
	}
	if out.Len() != 0 {
		return goFormatError(out.String())
	}
	return nil
}

// checkGoVet checks if the code to submitted has any "go vet" violations.
func (r *review) checkGoVet() (e error) {
	vetBin, cleanup, err := buildGoVetBinary(r.ctx)
	if err != nil {
		return err
	}
	defer collect.Error(cleanup, &e)

	goFiles, err := r.modifiedGoFiles()
	if err != nil || len(goFiles) == 0 {
		return err
	}
	// Check if the files generate any "go vet" errors.
	var out bytes.Buffer
	opts := r.ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	args := []string{"run", vetBin, "--composites=false"}
	// We vet one file at a time, because go vet only allows vetting of files of
	// the same package per invocation.
	var vetErrors []string
	for _, file := range goFiles {
		err := r.ctx.Run().CommandWithOpts(opts, "v23", append(args, file)...)
		if err == nil {
			continue
		}
		// A non-zero exit status means we should have a go vet violation.
		exiterr, ok := err.(*exec.ExitError)
		if !ok {
			return err
		}
		status, ok := exiterr.Sys().(syscall.WaitStatus)
		if !ok {
			return err
		}
		if status.ExitStatus() != 0 {
			vetErrors = append(vetErrors, out.String())
		}
	}
	if len(vetErrors) > 0 {
		return goVetError(vetErrors)
	}
	return nil
}

func buildGoVetBinary(ctx *tool.Context) (vetBin string, cleanup func() error, e error) {
	vetBin = govetBinaryFlag
	cleanup = func() error { return nil }
	if len(vetBin) == 0 {
		// build the go vet binary.
		tmpDir, err := ctx.Run().TempDir("", "")
		if err != nil {
			return "", cleanup, err
		}
		cleanup = func() error { return ctx.Run().RemoveAll(tmpDir) }
		vetBin = filepath.Join(tmpDir, "vet")
		if err := ctx.Run().Command("v23", "go", "build", "-o", vetBin, "golang.org/x/tools/cmd/vet"); err != nil {
			cleanup()
			return "", cleanup, err
		}
	}
	return vetBin, cleanup, nil
}

// modifiedGoFiles returns the modified go files in the change to be submitted.
func (r *review) modifiedGoFiles() ([]string, error) {
	files, err := r.ctx.Git().ModifiedFiles("master", r.branch)
	if err != nil {
		return nil, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("Getwd() failed: %v", err)
	}
	topLevel, err := r.ctx.Git().TopLevel()
	if err != nil {
		return nil, err
	}
	goFiles := []string{}
	for _, file := range files {
		path := filepath.Join(topLevel, file)
		// Skip non-Go files.
		if !strings.HasSuffix(file, ".go") {
			continue
		}
		// Skip Go files deleted by the change.
		if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
			continue
		}
		// Skip Go files with a "testdata" component in the path.
		if strings.Contains(path, "testdata"+string(filepath.Separator)) {
			continue
		}
		relativeFile, err := filepath.Rel(wd, path)
		if err == nil {
			file = relativeFile
		} else {
			file = path
		}
		goFiles = append(goFiles, file)
	}
	return goFiles, nil
}

// checkCopyright checks if the submitted code introduces source code
// without proper copyright.
func (r *review) checkCopyright() error {
	name, err := util.CurrentProjectName(r.ctx)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("current project is not a 'v23' project")
	}
	// Check the copyright headers and licensing files.
	var out bytes.Buffer
	if err := copyrightHelper(r.ctx.Stdout(), &out, []string{name}, false); err != nil {
		return err
	}
	if out.Len() != 0 {
		return copyrightError(out.String())
	}
	return nil
}

// checkDependencies checks if the code to be submitted meets the
// dependency constraints.
func (r *review) checkGoDependencies() error {
	var out bytes.Buffer
	opts := r.ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := r.ctx.Run().CommandWithOpts(opts, "v23", "run", "godepcop", "check", "v.io/..."); err != nil {
		return goDependencyError(out.String())
	}
	return nil
}

// checkGoAPI checks if the public Go API has changed.
func (r *review) checkGoAPI() error {
	name, err := util.CurrentProjectName(r.ctx)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("current project is not a 'v23' project")
	}
	config, err := util.LoadConfig(r.ctx)
	if err != nil {
		return err
	}
	if _, ok := config.APICheckProjects()[name]; !ok {
		// Skip the check for this project, it's not required.
		return nil
	}
	var out bytes.Buffer
	if err := doAPICheck(&out, r.ctx.Stderr(), []string{name}, false); err != nil {
		return err
	}
	if out.Len() != 0 {
		return apiError{
			apiCheckOutput: out.String(),
			project:        name,
		}
	}
	return nil
}

// cleanup cleans up after the review.
func (r *review) cleanup(stashed bool) error {
	if err := r.ctx.Git().CheckoutBranch(r.branch, !gitutil.Force); err != nil {
		return err
	}
	if r.ctx.Git().BranchExists(r.reviewBranch) {
		if err := r.ctx.Git().DeleteBranch(r.reviewBranch, gitutil.Force); err != nil {
			return err
		}
	}
	if stashed {
		if err := r.ctx.Git().StashPop(); err != nil {
			return err
		}
	}
	return nil
}

// createReviewBranch creates a clean review branch from master and
// squashes the commits into one, with the supplied message.
func (r *review) createReviewBranch(message string) error {
	if err := r.ctx.Git().Fetch("origin", "master"); err != nil {
		return err
	}
	if r.ctx.Git().BranchExists(r.reviewBranch) {
		if err := r.ctx.Git().DeleteBranch(r.reviewBranch, gitutil.Force); err != nil {
			return err
		}
	}
	upstream := "origin/master"
	if err := r.ctx.Git().CreateBranchWithUpstream(r.reviewBranch, upstream); err != nil {
		return err
	}
	if !r.ctx.DryRun() {
		hasDiff, err := r.ctx.Git().BranchesDiffer(r.branch, r.reviewBranch)
		if err != nil {
			return err
		}
		if !hasDiff {
			return emptyChangeError(struct{}{})
		}
	}
	// If message is empty, replace it with the default.
	if len(message) == 0 {
		var err error
		message, err = r.defaultCommitMessage()
		if err != nil {
			return err
		}
	}
	if err := r.ctx.Git().CheckoutBranch(r.reviewBranch, !gitutil.Force); err != nil {
		return err
	}
	if err := r.ctx.Git().Merge(r.branch, true); err != nil {
		return changeConflictError(err.Error())
	}
	c := r.ctx.Git().NewCommitter(r.edit)
	if err := c.Commit(message); err != nil {
		return err
	}
	return nil
}

// defaultCommitMessage creates the default commit message from the list of
// commits on the branch.
func (r *review) defaultCommitMessage() (string, error) {
	commitMessages, err := r.ctx.Git().CommitMessages(r.branch, r.reviewBranch)
	if err != nil {
		return "", err
	}
	// Strip "Change-Id: ..." from the commit messages.
	strippedMessages := changeIDRE.ReplaceAllLiteralString(commitMessages, "")
	// Add comment markers (#) to every line.
	commentedMessages := "# " + strings.Replace(strippedMessages, "\n", "\n# ", -1)
	message := defaultMessageHeader + commentedMessages
	return message, nil
}

// ensureChangeID makes sure that the last commit contains a Change-Id, and
// returns an error if it does not.
func (r *review) ensureChangeID() error {
	latestCommitMessage, err := r.ctx.Git().LatestCommitMessage()
	if err != nil {
		return err
	}
	changeID := changeIDRE.FindString(latestCommitMessage)
	if changeID == "" {
		return noChangeIDError(struct{}{})
	}
	return nil
}

// processPresubmitLabel adds/removes the "PresubmitTest" label for
// the given commit message.
func (r *review) processPresubmitLabel(message string) string {
	// Find the Change-ID line.
	changeIDLine := changeIDRE.FindString(message)

	// Strip existing presubmit label and change-ID.
	message = presubmitTestLabelRE.ReplaceAllLiteralString(message, "")
	message = changeIDRE.ReplaceAllLiteralString(message, "")

	// Insert presubmit label and change-ID back.
	if r.presubmit != gerrit.PresubmitTestTypeAll {
		message += fmt.Sprintf("PresubmitTest: %s\n", r.presubmit)
	}
	if changeIDLine != "" && !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	message += changeIDLine

	return message
}

// run implements checks that the review passes all local checks and
// then mails it to Gerrit.
func (r *review) run() (e error) {
	if uncommittedFlag {
		changes, err := r.ctx.Git().FilesWithUncommittedChanges()
		if err != nil {
			return err
		}
		if len(changes) != 0 {
			return uncommittedChangesError(changes)
		}
	}
	if gofmtFlag {
		if err := r.checkGoFormat(); err != nil {
			return err
		}
	}
	if govetFlag {
		if err := r.checkGoVet(); err != nil {
			return err
		}
	}
	if apiFlag {
		if err := r.checkGoAPI(); err != nil {
			return err
		}
	}
	if depcopFlag {
		if err := r.checkGoDependencies(); err != nil {
			return err
		}
	}
	if copyrightFlag {
		if err := r.checkCopyright(); err != nil {
			return err
		}
	}
	if r.branch == "master" {
		return fmt.Errorf("cannot do a review from the 'master' branch.")
	}
	filename, err := r.getCommitMessageFilename()
	if err != nil {
		return err
	}
	stashed, err := r.ctx.Git().Stash()
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer collect.Error(func() error { return r.ctx.Run().Chdir(wd) }, &e)
	topLevel, err := r.ctx.Git().TopLevel()
	if err != nil {
		return err
	}
	if err := r.ctx.Run().Chdir(topLevel); err != nil {
		return err
	}
	defer collect.Error(func() error { return r.cleanup(stashed) }, &e)
	message := ""
	data, err := ioutil.ReadFile(filename)
	if err == nil {
		message = string(data)
	} else if !os.IsNotExist(err) {
		return err
	}
	// Add/remove presubmit label to/from the commit message before
	// asking users to edit it. We do this only when this is not the
	// initial commit where the message is empty.
	//
	// For the initial commit, the presubmit label will be processed
	// after the message is edited by users, which happens in the
	// updateReviewMessage method.
	if message != "" {
		message = r.processPresubmitLabel(message)
	}
	if err := r.createReviewBranch(message); err != nil {
		return err
	}
	if err := r.updateReviewMessage(filename); err != nil {
		return err
	}
	if err := r.send(); err != nil {
		return err
	}
	return nil
}

// send mails the current branch out for review.
func (r *review) send() error {
	if !r.ctx.DryRun() {
		if err := r.ensureChangeID(); err != nil {
			return err
		}
	}
	if err := gerrit.Push(r.ctx.Run(), r.repo, r.draft, r.reviewers, r.ccs, r.branch); err != nil {
		return gerritError(err.Error())
	}
	return nil
}

// updateReviewMessage writes the commit message to the specified
// file. It then adds that file to the original branch, and makes sure
// it is not on the review branch.
func (r *review) updateReviewMessage(filename string) error {
	if err := r.ctx.Git().CheckoutBranch(r.reviewBranch, !gitutil.Force); err != nil {
		return err
	}
	newMessage, err := r.ctx.Git().LatestCommitMessage()
	if err != nil {
		return err
	}
	// For the initial commit where the commit message file doesn't
	// exist, add/remove presubmit label after users finish editing the
	// commit message.
	//
	// This behavior is consistent with how Change-ID is added for the
	// initial commit so we don't confuse users.
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			newMessage = r.processPresubmitLabel(newMessage)
			if err := r.ctx.Git().CommitAmend(newMessage); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if err := r.ctx.Git().CheckoutBranch(r.branch, !gitutil.Force); err != nil {
		return err
	}
	if err := r.ctx.Run().WriteFile(filename, []byte(newMessage), 0644); err != nil {
		return fmt.Errorf("WriteFile(%v, %v) failed: %v", filename, newMessage, err)
	}
	if err := r.ctx.Git().CommitFile(filename, "Update gerrit commit message."); err != nil {
		return err
	}
	// Delete the commit message from review branch.
	if err := r.ctx.Git().CheckoutBranch(r.reviewBranch, !gitutil.Force); err != nil {
		return err
	}
	if _, err := os.Stat(filename); err == nil {
		if err := r.ctx.Git().Remove(filename); err != nil {
			return err
		}
		if err := r.ctx.Git().CommitAmend(newMessage); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

// getCommitMessageFilename returns the name of the file that will get
// used for the Gerrit commit message.
func (r *review) getCommitMessageFilename() (string, error) {
	topLevel, err := r.ctx.Git().TopLevel()
	if err != nil {
		return "", err
	}
	return filepath.Join(topLevel, commitMessageFile), nil
}
