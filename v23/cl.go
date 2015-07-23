// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
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

const commitMessageFileName = ".gerrit_commit_message"

var (
	autosubmitFlag   bool
	apiFlag          bool
	ccsFlag          string
	copyrightFlag    bool
	draftFlag        bool
	depcopFlag       bool
	editFlag         bool
	forceFlag        bool
	gofmtFlag        bool
	govetFlag        bool
	govetBinaryFlag  string
	presubmitFlag    string
	remoteBranchFlag string
	reviewersFlag    string
	topicFlag        string
	uncommittedFlag  bool
)

// Special labels stored in the commit message.
var (
	// Auto submit label.
	autosubmitLabelRE *regexp.Regexp = regexp.MustCompile("AutoSubmit")

	// Change-Ids start with 'I' and are followed by 40 characters of hex.
	changeIDRE *regexp.Regexp = regexp.MustCompile("Change-Id: I[0123456789abcdefABCDEF]{40}")

	// Presubmit test label.
	// PresubmitTest: <type>
	presubmitTestLabelRE *regexp.Regexp = regexp.MustCompile(`PresubmitTest:\s*(.*)`)
)

// init carries out the package initialization.
func init() {
	cmdCLCleanup.Flags.BoolVar(&forceFlag, "f", false, "Ignore unmerged changes.")
	cmdCLCleanup.Flags.StringVar(&remoteBranchFlag, "remote-branch", "master", "Name of the remote branch the CL pertains to.")
	cmdCLMail.Flags.BoolVar(&apiFlag, "check-api", true, "Check for changes in the public Go API.")
	cmdCLMail.Flags.BoolVar(&autosubmitFlag, "autosubmit", false, "Automatically submit the changelist when feasiable.")
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
	cmdCLMail.Flags.StringVar(&remoteBranchFlag, "remote-branch", "master", "Name of the remote branch the CL pertains to.")
	cmdCLMail.Flags.StringVar(&reviewersFlag, "r", "", "Comma-seperated list of emails or LDAPs to request review.")
	cmdCLMail.Flags.StringVar(&topicFlag, "topic", "", "CL topic, defaults to <username>-<branchname>.")
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

func cleanupCL(ctx *tool.Context, branches []string) (e error) {
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
	if err := ctx.Git().Fetch("origin", remoteBranchFlag); err != nil {
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
		trackingBranch := "origin/" + remoteBranchFlag
		if err := ctx.Git().Merge(trackingBranch, false); err != nil {
			return err
		}
		files, err := ctx.Git().ModifiedFiles(trackingBranch, branch)
		if err != nil {
			return err
		}
		// A feature branch is considered merged with
		// the master, when there are no differences
		// or the only difference is the gerrit commit
		// message file.
		if len(files) != 0 && (len(files) != 1 || files[0] != commitMessageFileName) {
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
	return cleanupCL(ctx, args)
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

func (e apiError) Error() string {
	result := "changelist changes the public Go API without updating the corresponding .api file(s)\n\n"
	result += "For a detailed account of these changes, run 'v23 api check " + e.project + "'\n"
	result += "If these changes are intentional, run 'v23 api fix " + e.project + "'\n"
	result += "to update the corresponding .api files. Then add the updated .api files to\n"
	result += "your changelist and re-run the mail command.\n\n"
	result += e.apiCheckOutput
	return result
}

type changeConflictError struct {
	localBranch  string
	message      string
	remoteBranch string
}

func (e changeConflictError) Error() string {
	result := "changelist conflicts with the remote " + e.remoteBranch + " branch\n\n"
	result += "To resolve this problem, run 'git pull origin " + e.remoteBranch + ":" + e.localBranch + "',\n"
	result += "resolve the conflicts identified below, and then try again.\n"
	result += e.message
	return result
}

type copyrightError struct {
	message string
	project string
}

func (e copyrightError) Error() string {
	result := "changelist does not adhere to the copyright conventions\n\n"
	result += "To resolve this problem, run 'v23 copyright fix " + e.project + "' to\n"
	result += "fix the following violations:\n"
	result += e.message
	return result
}

type emptyChangeError struct{}

func (_ emptyChangeError) Error() string {
	return "current branch has no commits"
}

type gerritError string

func (e gerritError) Error() string {
	result := "sending code review failed\n\n"
	result += string(e)
	return result
}

type goDependencyError string

func (e goDependencyError) Error() string {
	result := "changelist introduces dependency violations\n\n"
	result += "To resolve this problem, fix the following violations:\n"
	result += string(e)
	return result
}

type goFormatError string

func (e goFormatError) Error() string {
	result := "changelist does not adhere to the Go formatting conventions\n\n"
	result += "To resolve this problem, run 'gofmt -w' for the following file(s):\n"
	result += string(e)
	return result
}

type goVetError []string

func (e goVetError) Error() string {
	result := "changelist contains 'go vet' violation(s)\n\n"
	result += "To resolve this problem, fix the following violations:\n"
	result += "  " + strings.Join(e, "\n  ")
	return result
}

type noChangeIDError struct{}

func (_ noChangeIDError) Error() string {
	result := "changelist is missing a Change-ID"
	return result
}

type uncommittedChangesError []string

func (e uncommittedChangesError) Error() string {
	result := "uncommitted local changes in files:\n"
	result += "  " + strings.Join(e, "\n  ")
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
	review, err := newReview(ctx, gerrit.CLOpts{
		Autosubmit:   autosubmitFlag,
		Ccs:          ccsFlag,
		Draft:        draftFlag,
		Edit:         editFlag,
		Presubmit:    gerrit.PresubmitTestType(presubmitFlag),
		RemoteBranch: remoteBranchFlag,
		Reviewers:    reviewersFlag,
		Topic:        topicFlag,
	})
	if err != nil {
		return err
	}
	if confirmed, err := review.confirmFlagChanges(); err != nil {
		return err
	} else if !confirmed {
		return nil
	}
	return review.run()
}

type review struct {
	ctx          *tool.Context
	reviewBranch string
	gerrit.CLOpts
}

func newReview(ctx *tool.Context, opts gerrit.CLOpts) (*review, error) {
	branch, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return nil, err
	}
	opts.Branch = branch
	if opts.Topic == "" {
		opts.Topic = fmt.Sprintf("%s-%s", os.Getenv("USER"), branch) // use <username>-<branchname> as the default
	}
	if opts.Presubmit == gerrit.PresubmitTestType("") {
		opts.Presubmit = gerrit.PresubmitTestTypeAll // use gerrit.PresubmitTestTypeAll as the default
	}
	if opts.RemoteBranch == "" {
		opts.RemoteBranch = "master" // use master as the default
	}
	return &review{
		ctx:          ctx,
		reviewBranch: branch + "-REVIEW",
		CLOpts:       opts,
	}, nil
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

// confirmFlagChanges asks users to confirm if any of the
// presubmit and autosubmit flags changes.
func (review *review) confirmFlagChanges() (bool, error) {
	file, err := review.getCommitMessageFileName()
	if err != nil {
		return false, err
	}
	bytes, err := review.ctx.Run().ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	content := string(bytes)
	changes := []string{}

	// Check presubmit label change.
	prevPresubmitType := string(gerrit.PresubmitTestTypeAll)
	matches := presubmitTestLabelRE.FindStringSubmatch(content)
	if matches != nil {
		prevPresubmitType = matches[1]
	}
	if presubmitFlag != prevPresubmitType {
		changes = append(changes, fmt.Sprintf("- presubmit=%s to presubmit=%s", prevPresubmitType, presubmitFlag))
	}

	// Check autosubmit label change.
	prevAutosubmit := autosubmitLabelRE.MatchString(content)
	if autosubmitFlag != prevAutosubmit {
		changes = append(changes, fmt.Sprintf("- autosubmit=%v to autosubmit=%v", prevAutosubmit, autosubmitFlag))

	}

	if len(changes) > 0 {
		fmt.Printf("Changes:\n%s\n", strings.Join(changes, "\n"))
		fmt.Print("Are you sure you want to make the above changes? y/N:")
		var response string
		if _, err := fmt.Scanf("%s\n", &response); err != nil || response != "y" {
			return false, nil
		}
	}
	return true, nil
}

// checkGoFormat checks if the code to be submitted needs to be
// formatted with "go fmt".
func (review *review) checkGoFormat() error {
	goFiles, err := review.modifiedGoFiles()
	if err != nil || len(goFiles) == 0 {
		return err
	}
	// Check if the formatting differs from gofmt.
	var out bytes.Buffer
	opts := review.ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	args := []string{"run", "gofmt", "-l"}
	args = append(args, goFiles...)
	if err := review.ctx.Run().CommandWithOpts(opts, "v23", args...); err != nil {
		return err
	}
	if out.Len() != 0 {
		return goFormatError(out.String())
	}
	return nil
}

// checkGoVet checks if the code to submitted has any "go vet" violations.
func (review *review) checkGoVet() (e error) {
	vetBin, cleanup, err := review.buildGoVetBinary()
	if err != nil {
		return err
	}
	defer collect.Error(cleanup, &e)

	goFiles, err := review.modifiedGoFiles()
	if err != nil || len(goFiles) == 0 {
		return err
	}
	// Check if the files generate any "go vet" errors.
	var out bytes.Buffer
	opts := review.ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	args := []string{"run", vetBin, "--composites=false"}
	// We vet one file at a time, because go vet only allows vetting of files of
	// the same package per invocation.
	var vetErrors []string
	for _, file := range goFiles {
		err := review.ctx.Run().CommandWithOpts(opts, "v23", append(args, file)...)
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

func (review *review) buildGoVetBinary() (vetBin string, cleanup func() error, e error) {
	vetBin, cleanup = govetBinaryFlag, func() error { return nil }
	if len(vetBin) == 0 {
		// Build the go vet binary.
		tmpDir, err := review.ctx.Run().TempDir("", "")
		if err != nil {
			return "", cleanup, err
		}
		cleanup = func() error { return review.ctx.Run().RemoveAll(tmpDir) }
		vetBin = filepath.Join(tmpDir, "vet")
		if err := review.ctx.Run().Command("v23", "go", "build", "-o", vetBin, "golang.org/x/tools/cmd/vet"); err != nil {
			cleanup()
			return "", cleanup, err
		}
	}
	return vetBin, cleanup, nil
}

// modifiedGoFiles returns the modified go files in the change to be submitted.
func (review *review) modifiedGoFiles() ([]string, error) {
	files, err := review.ctx.Git().ModifiedFiles(review.CLOpts.RemoteBranch, review.CLOpts.Branch)
	if err != nil {
		return nil, err
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("Getwd() failed: %v", err)
	}
	topLevel, err := review.ctx.Git().TopLevel()
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
func (review *review) checkCopyright() error {
	name, err := util.CurrentProjectName(review.ctx)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("current project is not a 'v23' project")
	}
	// Check if the copyright check should be invoked for the current
	// project.
	config, err := util.LoadConfig(review.ctx)
	if err != nil {
		return err
	}
	if _, ok := config.CopyrightCheckProjects()[name]; !ok {
		return nil
	}
	// Check the copyright headers and licensing files.
	var out bytes.Buffer
	if err := copyrightHelper(review.ctx.Stdout(), &out, []string{name}, false); err != nil {
		return err
	}
	if out.Len() != 0 {
		return copyrightError{
			message: out.String(),
			project: name,
		}
	}
	return nil
}

// checkDependencies checks if the code to be submitted meets the
// dependency constraints.
func (review *review) checkGoDependencies() error {
	var out bytes.Buffer
	opts := review.ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := review.ctx.Run().CommandWithOpts(opts, "v23", "run", "godepcop", "check", "v.io/..."); err != nil {
		return goDependencyError(out.String())
	}
	return nil
}

// checkGoAPI checks if the public Go API has changed.
func (review *review) checkGoAPI() error {
	name, err := util.CurrentProjectName(review.ctx)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("current project is not a 'v23' project")
	}
	// Check if the api check should be invoked for the current project.
	config, err := util.LoadConfig(review.ctx)
	if err != nil {
		return err
	}
	if _, ok := config.APICheckProjects()[name]; !ok {
		return nil
	}
	var out bytes.Buffer
	if err := doAPICheck(&out, review.ctx.Stderr(), []string{name}, false); err != nil {
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
func (review *review) cleanup(stashed bool) error {
	if err := review.ctx.Git().CheckoutBranch(review.CLOpts.Branch, !gitutil.Force); err != nil {
		return err
	}
	if review.ctx.Git().BranchExists(review.reviewBranch) {
		if err := review.ctx.Git().DeleteBranch(review.reviewBranch, gitutil.Force); err != nil {
			return err
		}
	}
	if stashed {
		if err := review.ctx.Git().StashPop(); err != nil {
			return err
		}
	}
	return nil
}

// createReviewBranch creates a clean review branch from master and
// squashes the commits into one, with the supplied message.
func (review *review) createReviewBranch(message string) error {
	if err := review.ctx.Git().Fetch("origin", review.CLOpts.RemoteBranch); err != nil {
		return err
	}
	if review.ctx.Git().BranchExists(review.reviewBranch) {
		if err := review.ctx.Git().DeleteBranch(review.reviewBranch, gitutil.Force); err != nil {
			return err
		}
	}
	upstream := "origin/" + review.CLOpts.RemoteBranch
	if err := review.ctx.Git().CreateBranchWithUpstream(review.reviewBranch, upstream); err != nil {
		return err
	}
	if !review.ctx.DryRun() {
		hasDiff, err := review.ctx.Git().BranchesDiffer(review.CLOpts.Branch, review.reviewBranch)
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
		message, err = review.defaultCommitMessage()
		if err != nil {
			return err
		}
	}
	if err := review.ctx.Git().CheckoutBranch(review.reviewBranch, !gitutil.Force); err != nil {
		return err
	}
	if err := review.ctx.Git().Merge(review.CLOpts.Branch, true); err != nil {
		return changeConflictError{
			localBranch:  review.CLOpts.Branch,
			remoteBranch: review.CLOpts.RemoteBranch,
			message:      err.Error(),
		}
	}
	committer := review.ctx.Git().NewCommitter(review.CLOpts.Edit)
	if err := committer.Commit(message); err != nil {
		return err
	}
	return nil
}

// defaultCommitMessage creates the default commit message from the
// list of commits on the branch.
func (review *review) defaultCommitMessage() (string, error) {
	commitMessages, err := review.ctx.Git().CommitMessages(review.CLOpts.Branch, review.reviewBranch)
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
func (review *review) ensureChangeID() error {
	latestCommitMessage, err := review.ctx.Git().LatestCommitMessage()
	if err != nil {
		return err
	}
	changeID := changeIDRE.FindString(latestCommitMessage)
	if changeID == "" {
		return noChangeIDError(struct{}{})
	}
	return nil
}

// processLabels adds/removes labels for the given commit message.
func (review *review) processLabels(message string) string {
	// Find the Change-ID line.
	changeIDLine := changeIDRE.FindString(message)

	// Strip existing labels and change-ID.
	message = autosubmitLabelRE.ReplaceAllLiteralString(message, "")
	message = presubmitTestLabelRE.ReplaceAllLiteralString(message, "")
	message = changeIDRE.ReplaceAllLiteralString(message, "")

	// Insert labels and change-ID back.
	if review.CLOpts.Autosubmit {
		message += fmt.Sprintf("AutoSubmit\n")
	}
	if review.CLOpts.Presubmit != gerrit.PresubmitTestTypeAll {
		message += fmt.Sprintf("PresubmitTest: %s\n", review.CLOpts.Presubmit)
	}
	if changeIDLine != "" && !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	message += changeIDLine

	return message
}

// run implements checks that the review passes all local checks
// and then mails it to Gerrit.
func (review *review) run() (e error) {
	if uncommittedFlag {
		changes, err := review.ctx.Git().FilesWithUncommittedChanges()
		if err != nil {
			return err
		}
		if len(changes) != 0 {
			return uncommittedChangesError(changes)
		}
	}
	if gofmtFlag {
		if err := review.checkGoFormat(); err != nil {
			return err
		}
	}
	if govetFlag {
		if err := review.checkGoVet(); err != nil {
			return err
		}
	}
	if apiFlag {
		if err := review.checkGoAPI(); err != nil {
			return err
		}
	}
	if depcopFlag {
		if err := review.checkGoDependencies(); err != nil {
			return err
		}
	}
	if copyrightFlag {
		if err := review.checkCopyright(); err != nil {
			return err
		}
	}
	if review.CLOpts.Branch == "master" {
		return fmt.Errorf("cannot do a review from the 'master' branch.")
	}
	stashed, err := review.ctx.Git().Stash()
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer collect.Error(func() error { return review.ctx.Run().Chdir(wd) }, &e)
	topLevel, err := review.ctx.Git().TopLevel()
	if err != nil {
		return err
	}
	if err := review.ctx.Run().Chdir(topLevel); err != nil {
		return err
	}
	defer collect.Error(func() error { return review.cleanup(stashed) }, &e)
	message := ""
	filename, err := review.getCommitMessageFileName()
	if err != nil {
		return err
	}
	data, err := review.ctx.Run().ReadFile(filename)
	if err == nil {
		message = string(data)
	} else if !os.IsNotExist(err) {
		return err
	}
	// Add/remove labels to/from the commit message before asking users to
	// edit it. We do this only when this is not the initial commit
	// where the message is empty.
	//
	// For the initial commit, the labels will be processed after the message is
	// edited by users, which happens in the updateReviewMessage method.
	if message != "" {
		message = review.processLabels(message)
	}
	if err := review.createReviewBranch(message); err != nil {
		return err
	}
	if err := review.updateReviewMessage(filename); err != nil {
		return err
	}
	if err := review.send(); err != nil {
		return err
	}
	return nil
}

// send mails the current branch out for review.
func (review *review) send() error {
	if !review.ctx.DryRun() {
		if err := review.ensureChangeID(); err != nil {
			return err
		}
	}
	if err := gerrit.Push(review.ctx.Run(), review.CLOpts); err != nil {
		return gerritError(err.Error())
	}
	return nil
}

// updateReviewMessage writes the commit message to the specified
// file. It then adds that file to the original branch, and makes sure
// it is not on the review branch.
func (review *review) updateReviewMessage(filename string) error {
	if err := review.ctx.Git().CheckoutBranch(review.reviewBranch, !gitutil.Force); err != nil {
		return err
	}
	newMessage, err := review.ctx.Git().LatestCommitMessage()
	if err != nil {
		return err
	}
	// For the initial commit where the commit message file doesn't exist,
	// add/remove labels after users finish editing the commit message.
	//
	// This behavior is consistent with how Change-ID is added for the
	// initial commit so we don't confuse users.
	if _, err := os.Stat(filename); err != nil {
		if os.IsNotExist(err) {
			newMessage = review.processLabels(newMessage)
			if err := review.ctx.Git().CommitAmend(newMessage); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if err := review.ctx.Git().CheckoutBranch(review.CLOpts.Branch, !gitutil.Force); err != nil {
		return err
	}
	if err := review.ctx.Run().WriteFile(filename, []byte(newMessage), 0644); err != nil {
		return fmt.Errorf("WriteFile(%v, %v) failed: %v", filename, newMessage, err)
	}
	if err := review.ctx.Git().CommitFile(filename, "Update gerrit commit message."); err != nil {
		return err
	}
	// Delete the commit message from review branch.
	if err := review.ctx.Git().CheckoutBranch(review.reviewBranch, !gitutil.Force); err != nil {
		return err
	}
	if _, err := os.Stat(filename); err == nil {
		if err := review.ctx.Git().Remove(filename); err != nil {
			return err
		}
		if err := review.ctx.Git().CommitAmend(newMessage); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

// getCommitMessageFileName returns the name of the file that will get
// used for the Gerrit commit message.
func (review *review) getCommitMessageFileName() (string, error) {
	topLevel, err := review.ctx.Git().TopLevel()
	if err != nil {
		return "", err
	}
	return filepath.Join(topLevel, commitMessageFileName), nil
}
