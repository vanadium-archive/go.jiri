// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/gerrit"
	"v.io/x/devtools/internal/gitutil"
	"v.io/x/devtools/internal/project"
	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

const (
	commitMessageFileName  = ".gerrit_commit_message"
	dependencyPathFileName = ".dependency_path"
)

var (
	autosubmitFlag   bool
	ccsFlag          string
	draftFlag        bool
	editFlag         bool
	hostFlag         string
	forceFlag        bool
	presubmitFlag    string
	remoteBranchFlag string
	reviewersFlag    string
	setTopicFlag     bool
	topicFlag        string
	uncommittedFlag  bool
)

// Special labels stored in the commit message.
var (
	// Auto submit label.
	autosubmitLabelRE *regexp.Regexp = regexp.MustCompile("AutoSubmit")

	// Change-Ids start with 'I' and are followed by 40 characters of hex.
	changeIDRE *regexp.Regexp = regexp.MustCompile("Change-Id: (I[0123456789abcdefABCDEF]{40})")

	// Presubmit test label.
	// PresubmitTest: <type>
	presubmitTestLabelRE *regexp.Regexp = regexp.MustCompile(`PresubmitTest:\s*(.*)`)
)

// init carries out the package initialization.
func init() {
	cmdCLCleanup.Flags.BoolVar(&forceFlag, "f", false, "Ignore unmerged changes.")
	cmdCLCleanup.Flags.StringVar(&remoteBranchFlag, "remote-branch", "master", "Name of the remote branch the CL pertains to.")
	cmdCLMail.Flags.BoolVar(&autosubmitFlag, "autosubmit", false, "Automatically submit the changelist when feasiable.")
	cmdCLMail.Flags.StringVar(&ccsFlag, "cc", "", "Comma-seperated list of emails or LDAPs to cc.")
	cmdCLMail.Flags.BoolVar(&draftFlag, "d", false, "Send a draft changelist.")
	cmdCLMail.Flags.BoolVar(&editFlag, "edit", true, "Open an editor to edit the commit message.")
	cmdCLMail.Flags.StringVar(&hostFlag, "host", project.VanadiumGerritHost(), "Gerrit host to use.")
	cmdCLMail.Flags.StringVar(&presubmitFlag, "presubmit", string(gerrit.PresubmitTestTypeAll),
		fmt.Sprintf("The type of presubmit tests to run. Valid values: %s.", strings.Join(gerrit.PresubmitTestTypes(), ",")))
	cmdCLMail.Flags.StringVar(&remoteBranchFlag, "remote-branch", "master", "Name of the remote branch the CL pertains to.")
	cmdCLMail.Flags.StringVar(&reviewersFlag, "r", "", "Comma-seperated list of emails or LDAPs to request review.")
	cmdCLMail.Flags.BoolVar(&setTopicFlag, "set-topic", true, "Set Gerrit CL topic.")
	cmdCLMail.Flags.StringVar(&topicFlag, "topic", "", "CL topic, defaults to <username>-<branchname>.")
	cmdCLMail.Flags.BoolVar(&uncommittedFlag, "check-uncommitted", true, "Check that no uncommitted changes exist.")
	cmdCLSync.Flags.StringVar(&remoteBranchFlag, "remote-branch", "master", "Name of the remote branch the CL pertains to.")
}

func getCommitMessageFileName(ctx *tool.Context, branch string) (string, error) {
	topLevel, err := ctx.Git().TopLevel()
	if err != nil {
		return "", err
	}
	return filepath.Join(topLevel, project.MetadataDirName(), branch, commitMessageFileName), nil
}

func getDependencyPathFileName(ctx *tool.Context, branch string) (string, error) {
	topLevel, err := ctx.Git().TopLevel()
	if err != nil {
		return "", err
	}
	return filepath.Join(topLevel, project.MetadataDirName(), branch, dependencyPathFileName), nil
}

func getDependentCLs(ctx *tool.Context, branch string) ([]string, error) {
	file, err := getDependencyPathFileName(ctx, branch)
	if err != nil {
		return nil, err
	}
	data, err := ctx.Run().ReadFile(file)
	var branches []string
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		if branch != remoteBranchFlag {
			branches = []string{remoteBranchFlag}
		}
	} else {
		branches = strings.Split(strings.TrimSpace(string(data)), "\n")
	}
	return branches, nil
}

// cmdCL represents the "v23 cl" command.
var cmdCL = &cmdline.Command{
	Name:     "cl",
	Short:    "Manage project changelists",
	Long:     "Manage project changelists.",
	Children: []*cmdline.Command{cmdCLCleanup, cmdCLMail, cmdCLNew, cmdCLSync},
}

// cmdCLCleanup represents the "v23 cl cleanup" command.
//
// TODO(jsimsa): Replace this with a "submit" command that talks to
// Gerrit to submit the CL and then (optionally) removes it locally.
var cmdCLCleanup = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runCLCleanup),
	Name:   "cleanup",
	Short:  "Clean up changelists that have been merged",
	Long: `
Command "cleanup" checks that the given branches have been merged into
the corresponding remote branch. If a branch differs from the
corresponding remote branch, the command reports the difference and
stops. Otherwise, it deletes the given branches.
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
	if err := ctx.Git().CheckoutBranch(remoteBranchFlag); err != nil {
		return err
	}
	checkoutOriginalBranch := true
	defer collect.Error(func() error {
		if checkoutOriginalBranch {
			return ctx.Git().CheckoutBranch(originalBranch)
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
	if err := ctx.Git().CheckoutBranch(branch); err != nil {
		return err
	}
	if !forceFlag {
		trackingBranch := "origin/" + remoteBranchFlag
		if err := ctx.Git().Merge(trackingBranch); err != nil {
			return err
		}
		files, err := ctx.Git().ModifiedFiles(trackingBranch, branch)
		if err != nil {
			return err
		}
		if len(files) != 0 {
			return fmt.Errorf("unmerged changes in\n%s", strings.Join(files, "\n"))
		}
	}
	if err := ctx.Git().CheckoutBranch(remoteBranchFlag); err != nil {
		return err
	}
	if err := ctx.Git().DeleteBranch(branch, gitutil.ForceOpt(true)); err != nil {
		return err
	}
	reviewBranch := branch + "-REVIEW"
	if ctx.Git().BranchExists(reviewBranch) {
		if err := ctx.Git().DeleteBranch(reviewBranch, gitutil.ForceOpt(true)); err != nil {
			return err
		}
	}
	// Delete branch metadata.
	topLevel, err := ctx.Git().TopLevel()
	if err != nil {
		return err
	}
	metadataDir := filepath.Join(topLevel, project.MetadataDirName())
	if err := ctx.Run().RemoveAll(filepath.Join(metadataDir, branch)); err != nil {
		return err
	}
	// Remove the branch from all dependency paths.
	fileInfos, err := ctx.Run().ReadDir(metadataDir)
	if err != nil {
		return err
	}
	for _, fileInfo := range fileInfos {
		if !fileInfo.IsDir() {
			continue
		}
		file, err := getDependencyPathFileName(ctx, fileInfo.Name())
		if err != nil {
			return err
		}
		data, err := ctx.Run().ReadFile(file)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			continue
		}
		branches := strings.Split(string(data), "\n")
		for i, tmpBranch := range branches {
			if branch == tmpBranch {
				data := []byte(strings.Join(append(branches[:i], branches[i+1:]...), "\n"))
				if err := ctx.Run().WriteFile(file, data, os.FileMode(0644)); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

func runCLCleanup(env *cmdline.Env, args []string) error {
	if len(args) == 0 {
		return env.UsageErrorf("cleanup requires at least one argument")
	}
	ctx := tool.NewContextFromEnv(env)
	return cleanupCL(ctx, args)
}

// cmdCLMail represents the "v23 cl mail" command.
var cmdCLMail = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runCLMail),
	Name:   "mail",
	Short:  "Mail a changelist for review",
	Long: `
Command "mail" squashes all commits of a local branch into a single
"changelist" and mails this changelist to Gerrit as a single
commit. First time the command is invoked, it generates a Change-Id
for the changelist, which is appended to the commit
message. Consecutive invocations of the command use the same Change-Id
by default, informing Gerrit that the incomming commit is an update of
an existing changelist.
`,
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
	ctx := tool.NewContextFromEnv(env)

	// Sanity checks for the <presubmitFlag> flag.
	if !checkPresubmitFlag() {
		return env.UsageErrorf("invalid value for the -presubmit flag. Valid values: %s.",
			strings.Join(gerrit.PresubmitTestTypes(), ","))
	}

	// Sync all CLs in the sequence of dependent CLs ending in the
	// current branch.
	if err := syncCL(ctx); err != nil {
		return err
	}

	// Make sure that all CLs in the above sequence (possibly except for
	// the current branch) have been exported to Gerrit. This is needed
	// to make sure we have commit messages for all but the last CL.
	//
	// NOTE: The alternative here is to prompt the user for multiple
	// commit messages, which seems less user friendly.
	if err := checkDependents(ctx); err != nil {
		return err
	}

	// Create and run the review.

	review, err := newReview(ctx, gerrit.CLOpts{
		Autosubmit:   autosubmitFlag,
		Ccs:          parseEmails(ccsFlag),
		Draft:        draftFlag,
		Edit:         editFlag,
		Host:         hostFlag,
		Presubmit:    gerrit.PresubmitTestType(presubmitFlag),
		RemoteBranch: remoteBranchFlag,
		Reviewers:    parseEmails(reviewersFlag),
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

// parseEmails input a list of comma separated tokens and outputs a
// list of email addresses. The tokens can either be email addresses
// or Google LDAPs in which case the suffix @google.com is appended to
// them to turn them into email addresses.
func parseEmails(value string) []string {
	var emails []string
	tokens := strings.Split(value, ",")
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if !strings.Contains(token, "@") {
			token += "@google.com"
		}
		emails = append(emails, token)
	}
	return emails
}

// checkDependents makes sure that all CLs in the sequence of
// dependent CLs leading to (but not including) the current branch
// have been exported to Gerrit.
func checkDependents(ctx *tool.Context) (e error) {
	originalBranch, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return err
	}
	branches, err := getDependentCLs(ctx, originalBranch)
	if err != nil {
		return err
	}
	for i := 1; i < len(branches); i++ {
		file, err := getCommitMessageFileName(ctx, branches[i])
		if err != nil {
			return err
		}
		if _, err := ctx.Run().Stat(file); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			return fmt.Errorf(`Failed to export the branch %q to Gerrit because its ancestor %q has not been exported to Gerrit yet.
The following steps are needed before the operation can be retried:
$ git checkout %v
$ v23 cl mail
$ git checkout %v
# retry the original command
`, originalBranch, branches[i], branches[i], originalBranch)
		}
	}

	return nil
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
	for _, t := range gerrit.PresubmitTestTypes() {
		if presubmitFlag == t {
			return true
		}
	}
	return false
}

// confirmFlagChanges asks users to confirm if any of the
// presubmit and autosubmit flags changes.
func (review *review) confirmFlagChanges() (bool, error) {
	file, err := getCommitMessageFileName(review.ctx, review.CLOpts.Branch)
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

// cleanup cleans up after the review.
func (review *review) cleanup(stashed bool) error {
	if err := review.ctx.Git().CheckoutBranch(review.CLOpts.Branch); err != nil {
		return err
	}
	if review.ctx.Git().BranchExists(review.reviewBranch) {
		if err := review.ctx.Git().DeleteBranch(review.reviewBranch, gitutil.ForceOpt(true)); err != nil {
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

// createReviewBranch creates a clean review branch from the remote
// branch this CL pertains to and then iterates over the sequence of
// dependent CLs leading to the current branch, creating one commit
// per CL by squashing all commits of each individual CL. The commit
// message for all but that last CL is derived from their
// <commitMessageFileName>, while the <message> argument is used as
// the commit message for the last commit.
func (review *review) createReviewBranch(message string) (e error) {
	// Create the review branch.
	if err := review.ctx.Git().Fetch("origin", review.CLOpts.RemoteBranch); err != nil {
		return err
	}
	if review.ctx.Git().BranchExists(review.reviewBranch) {
		if err := review.ctx.Git().DeleteBranch(review.reviewBranch, gitutil.ForceOpt(true)); err != nil {
			return err
		}
	}
	upstream := "origin/" + review.CLOpts.RemoteBranch
	if err := review.ctx.Git().CreateBranchWithUpstream(review.reviewBranch, upstream); err != nil {
		return err
	}
	if err := review.ctx.Git().CheckoutBranch(review.reviewBranch); err != nil {
		return err
	}
	// Register a cleanup handler in case of subsequent errors.
	cleanup := true
	defer collect.Error(func() error {
		if !cleanup {
			return review.ctx.Git().CheckoutBranch(review.CLOpts.Branch)
		}
		review.ctx.Git().CheckoutBranch(review.CLOpts.Branch, gitutil.ForceOpt(true))
		review.ctx.Git().DeleteBranch(review.reviewBranch, gitutil.ForceOpt(true))
		return nil
	}, &e)

	// Report an error if the CL is empty.
	if !review.ctx.DryRun() {
		hasDiff, err := review.ctx.Git().BranchesDiffer(review.CLOpts.Branch, review.reviewBranch)
		if err != nil {
			return err
		}
		if !hasDiff {
			return emptyChangeError(struct{}{})
		}
	}

	// If <message> is empty, replace it with the default message.
	if len(message) == 0 {
		var err error
		message, err = review.defaultCommitMessage()
		if err != nil {
			return err
		}
	}

	// Iterate over all dependent CLs leading to (and including) the
	// current branch, creating one commit in the review branch per CL
	// by squashing all commits of each individual CL.
	branches, err := getDependentCLs(review.ctx, review.CLOpts.Branch)
	if err != nil {
		return err
	}
	branches = append(branches, review.CLOpts.Branch)
	if err := review.squashBranches(branches, message); err != nil {
		return err
	}

	cleanup = false
	return nil
}

// squashBranches iterates over the given list of branches, creating
// one commit per branch in the current branch by squashing all
// commits of each individual branch.
//
// TODO(jsimsa): Consider using "git rebase --onto" to avoid having to
// deal with merge conflicts.
func (review *review) squashBranches(branches []string, message string) (e error) {
	for i := 1; i < len(branches); i++ {
		// We want to merge the <branches[i]> branch on top of the review
		// branch, forcing all conflicts to be reviewed in favor of the
		// <branches[i]> branch. Unfortunately, git merge does not offer a
		// strategy that would do that for us. The solution implemented
		// here is based on:
		//
		// http://stackoverflow.com/questions/173919/is-there-a-theirs-version-of-git-merge-s-ours
		if err := review.ctx.Git().Merge(branches[i], gitutil.SquashOpt(true), gitutil.StrategyOpt("ours")); err != nil {
			return changeConflictError{
				localBranch:  branches[i],
				remoteBranch: review.CLOpts.RemoteBranch,
				message:      err.Error(),
			}
		}
		// Fetch the timestamp of the last commit of <branches[i]> and use
		// it to create the squashed commit. This is needed to make sure
		// that the commit hash of the squashed commit stays the same as
		// long as the squashed sequence of commits does not change. If
		// this was not the case, consecutive invocations of "v23 cl mail"
		// could fail if some, but not all, of the dependent CLs submitted
		// to Gerrit have changed.
		output, err := review.ctx.Git().Log(branches[i], branches[i]+"^", "%ad%n%cd")
		if err != nil {
			return err
		}
		if len(output) < 1 || len(output[0]) < 2 {
			return fmt.Errorf("unexpected output length: %v", output)
		}
		authorDate := tool.AuthorDateOpt(output[0][0])
		committerDate := tool.CommitterDateOpt(output[0][1])
		if i < len(branches)-1 {
			file, err := getCommitMessageFileName(review.ctx, branches[i])
			if err != nil {
				return err
			}
			message, err := review.ctx.Run().ReadFile(file)
			if err != nil {
				return err
			}
			if err := review.ctx.Git(authorDate, committerDate).CommitWithMessage(string(message)); err != nil {
				return err
			}
		} else {
			committer := review.ctx.Git(authorDate, committerDate).NewCommitter(review.CLOpts.Edit)
			if err := committer.Commit(message); err != nil {
				return err
			}
		}
		tmpBranch := review.reviewBranch + "-" + branches[i] + "-TMP"
		if err := review.ctx.Git().CreateBranch(tmpBranch); err != nil {
			return err
		}
		defer collect.Error(func() error {
			return review.ctx.Git().DeleteBranch(tmpBranch, gitutil.ForceOpt(true))
		}, &e)
		if err := review.ctx.Git().Reset(branches[i]); err != nil {
			return err
		}
		if err := review.ctx.Git().Reset(tmpBranch, gitutil.ModeOpt("soft")); err != nil {
			return err
		}
		if err := review.ctx.Git(authorDate, committerDate).CommitAmend(); err != nil {
			return err
		}
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
	if review.CLOpts.Branch == remoteBranchFlag {
		return fmt.Errorf("cannot do a review from the %q branch.", remoteBranchFlag)
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
	file, err := getCommitMessageFileName(review.ctx, review.CLOpts.Branch)
	if err != nil {
		return err
	}
	data, err := review.ctx.Run().ReadFile(file)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// CLs exported to Gerrit prior to v.io/c/13993 might have the
		// commit message file stored in a different location. Check if
		// the file exists there, and if so, read it into memory and then
		// get rid of it.
		file := filepath.Join(topLevel, commitMessageFileName)
		data, err := review.ctx.Run().ReadFile(file)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		} else {
			message = string(data)
			if err := review.ctx.Git().Remove(file); err != nil {
				return err
			}
			if err := review.ctx.Git().CommitWithMessage("removing outdated commit message file"); err != nil {
				return err
			}
		}
	} else {
		message = string(data)
	}
	// Add/remove labels to/from the commit message before asking users
	// to edit it. We do this only when this is not the initial commit
	// where the message is empty.
	//
	// For the initial commit, the labels will be processed after the
	// message is edited by users, which happens in the
	// updateReviewMessage method.
	if message != "" {
		message = review.processLabels(message)
	}
	if err := review.createReviewBranch(message); err != nil {
		return err
	}
	if err := review.updateReviewMessage(file); err != nil {
		return err
	}
	if err := review.send(); err != nil {
		return err
	}
	if setTopicFlag {
		if err := review.setTopic(); err != nil {
			return err
		}
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

// setTopic sets the topic for the CL corresponding to the branch the
// review was created for.
func (review *review) setTopic() error {
	// Read the commit message and extract the Change-Id.
	file, err := getCommitMessageFileName(review.ctx, review.CLOpts.Branch)
	if err != nil {
		return err
	}
	bytes, err := review.ctx.Run().ReadFile(file)
	if err != nil {
		return err
	}
	changeID := changeIDRE.FindSubmatch(bytes)
	if changeID == nil || len(changeID) < 2 {
		return fmt.Errorf("could not find Change-Id in:\n%s", bytes)
	}
	if err := review.ctx.Gerrit(review.CLOpts.Host).SetTopic(string(changeID[1]), review.CLOpts); err != nil {
		return err
	}
	return nil
}

// updateReviewMessage writes the commit message to the given file.
func (review *review) updateReviewMessage(file string) error {
	if err := review.ctx.Git().CheckoutBranch(review.reviewBranch); err != nil {
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
	if _, err := review.ctx.Run().Stat(file); err != nil {
		if os.IsNotExist(err) {
			newMessage = review.processLabels(newMessage)
			if err := review.ctx.Git().CommitAmendWithMessage(newMessage); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	topLevel, err := review.ctx.Git().TopLevel()
	if err != nil {
		return err
	}
	newMetadataDir := filepath.Join(topLevel, project.MetadataDirName(), review.CLOpts.Branch)
	if err := review.ctx.Run().MkdirAll(newMetadataDir, os.FileMode(0755)); err != nil {
		return err
	}
	if err := review.ctx.Run().WriteFile(file, []byte(newMessage), 0644); err != nil {
		return err
	}
	return nil
}

// cmdCLNew represents the "v23 cl new" command.
var cmdCLNew = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runCLNew),
	Name:   "new",
	Short:  "Create a new local branch for a changelist",
	Long: fmt.Sprintf(`
Command "new" creates a new local branch for a changelist. In
particular, it forks a new branch with the given name from the current
branch and records the relationship between the current branch and the
new branch in the %v metadata directory. The information recorded in
the %v metadata directory tracks dependencies between CLs and is used
by the "v23 cl sync" and "v23 cl mail" commands.
`, project.MetadataDirName(), project.MetadataDirName()),
	ArgsName: "<name>",
	ArgsLong: "<name> is the changelist name.",
}

func runCLNew(env *cmdline.Env, args []string) error {
	if got, want := len(args), 1; got != want {
		return env.UsageErrorf("unexpected number of arguments: got %v, want %v", got, want)
	}
	ctx := tool.NewContextFromEnv(env)
	return newCL(ctx, args)
}

func newCL(ctx *tool.Context, args []string) error {
	topLevel, err := ctx.Git().TopLevel()
	if err != nil {
		return err
	}
	originalBranch, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return err
	}

	// Create a new branch using the current branch.
	newBranch := args[0]
	if err := ctx.Git().CreateAndCheckoutBranch(newBranch); err != nil {
		return err
	}

	// Register a cleanup handler in case of subsequent errors.
	cleanup := true
	defer func() {
		if cleanup {
			ctx.Git().CheckoutBranch(originalBranch, gitutil.ForceOpt(true))
			ctx.Git().DeleteBranch(newBranch, gitutil.ForceOpt(true))
		}
	}()

	// Record the dependent CLs for the new branch. The dependent CLs
	// are recorded in a <dependencyPathFileName> file as a
	// newline-separated list of branch names.
	branches, err := getDependentCLs(ctx, originalBranch)
	if err != nil {
		return err
	}
	branches = append(branches, originalBranch)
	newMetadataDir := filepath.Join(topLevel, project.MetadataDirName(), newBranch)
	if err := ctx.Run().MkdirAll(newMetadataDir, os.FileMode(0755)); err != nil {
		return err
	}
	file, err := getDependencyPathFileName(ctx, newBranch)
	if err != nil {
		return err
	}
	if err := ctx.Run().WriteFile(file, []byte(strings.Join(branches, "\n")), os.FileMode(0644)); err != nil {
		return err
	}

	cleanup = false
	return nil
}

// cmdCLSync represents the "v23 cl sync" command.
var cmdCLSync = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runCLSync),
	Name:   "sync",
	Short:  "Bring a changelist up to date",
	Long: fmt.Sprintf(`
Command "sync" brings the CL identified by the current branch up to
date with the branch tracking the remote branch this CL pertains
to. To do that, the command uses the information recorded in the %v
metadata directory to identify the sequence of dependent CLs leading
to the current branch. The command then iterates over this sequence
bringing each of the CLs up to date with its ancestor. The end result
of this process is that all CLs in the sequence are up to date with
the branch that tracks the remote branch this CL pertains to.

NOTE: It is possible that the command cannot automatically merge
changes in an ancestor into its dependent. When that occurs, the
command is aborted and prints instructions that need to be followed
before the command can be retried.
`, project.MetadataDirName()),
}

func runCLSync(env *cmdline.Env, _ []string) error {
	ctx := tool.NewContextFromEnv(env)
	return syncCL(ctx)
}

func syncCL(ctx *tool.Context) (e error) {
	stashed, err := ctx.Git().Stash()
	if err != nil {
		return err
	}
	if stashed {
		defer collect.Error(func() error { return ctx.Git().StashPop() }, &e)
	}

	// Register a cleanup handler in case of subsequent errors.
	cleanup := true
	originalBranch, err := ctx.Git().CurrentBranchName()
	if err != nil {
		return err
	}
	defer func() {
		if cleanup {
			ctx.Git().CheckoutBranch(originalBranch, gitutil.ForceOpt(true))
		}
	}()

	// Identify the dependents CLs leading to (and including) the
	// current branch.
	branches, err := getDependentCLs(ctx, originalBranch)
	if err != nil {
		return err
	}
	branches = append(branches, originalBranch)

	// Bring all CLs in the sequence of dependent CLs leading to the
	// current branch up to date with the <remoteBranchFlag> branch.
	for i := 1; i < len(branches); i++ {
		if err := ctx.Git().CheckoutBranch(branches[i]); err != nil {
			return err
		}
		if err := ctx.Git().Merge(branches[i-1]); err != nil {
			return fmt.Errorf(`Failed to automatically merge branch %v into branch %v: %v
The following steps are needed before the operation can be retried:
$ git checkout %v
$ git merge %v
# resolve all conflicts
$ git commit -a
$ git checkout %v
# retry the original operation
`, branches[i], branches[i-1], err, branches[i], branches[i-1], originalBranch)
		}
	}

	cleanup = false
	return nil
}
