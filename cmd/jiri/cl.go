// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"v.io/jiri"
	"v.io/jiri/collect"
	"v.io/jiri/gerrit"
	"v.io/jiri/gitutil"
	"v.io/jiri/profiles/profilescmdline"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/x/lib/cmdline"
)

const (
	commitMessageFileName     = ".gerrit_commit_message"
	dependencyPathFileName    = ".dependency_path"
	multiPartMetaDataFileName = "multipart_index"
)

var (
	autosubmitFlag        bool
	ccsFlag               string
	draftFlag             bool
	editFlag              bool
	forceFlag             bool
	hostFlag              string
	messageFlag           string
	commitMessageBodyFlag string
	presubmitFlag         string
	remoteBranchFlag      string
	reviewersFlag         string
	setTopicFlag          bool
	topicFlag             string
	uncommittedFlag       bool
	verifyFlag            bool
	currentProjectFlag    bool
	cleanupMultiPartFlag  bool
)

// Special labels stored in the commit message.
var (
	// Auto submit label.
	autosubmitLabelRE *regexp.Regexp = regexp.MustCompile("AutoSubmit")

	// Change-Ids start with 'I' and are followed by 40 characters of hex.
	changeIDRE *regexp.Regexp = regexp.MustCompile("Change-Id: (I[0123456789abcdefABCDEF]{40})")

	// MultiPart messages are of the form: MultiPart: <n>/<m>
	multiPartRE *regexp.Regexp = regexp.MustCompile(`(?m)^MultiPart: \d+/\d+$`)

	// Presubmit test label.
	// PresubmitTest: <type>
	presubmitTestLabelRE *regexp.Regexp = regexp.MustCompile(`PresubmitTest:\s*(.*)`)

	noChangesRE *regexp.Regexp = regexp.MustCompile(`! \[remote rejected\] HEAD -> refs/(for|drafts)/\S+ \(no new changes\)`)
)

// init carries out the package initialization.
func init() {
	cmdCLMail = newCmdCLMail()
	cmdCL = newCmdCL()
	cmdCLCleanup.Flags.BoolVar(&forceFlag, "f", false, `Ignore unmerged changes.`)
	cmdCLCleanup.Flags.StringVar(&remoteBranchFlag, "remote-branch", "master", `Name of the remote branch the CL pertains to, without the leading "origin/".`)
	cmdCLMail.Flags.BoolVar(&autosubmitFlag, "autosubmit", false, `Automatically submit the changelist when feasible.`)
	cmdCLMail.Flags.StringVar(&ccsFlag, "cc", "", `Comma-seperated list of emails or LDAPs to cc.`)
	cmdCLMail.Flags.BoolVar(&draftFlag, "d", false, `Send a draft changelist.`)
	cmdCLMail.Flags.BoolVar(&editFlag, "edit", true, `Open an editor to edit the CL description.`)
	cmdCLMail.Flags.StringVar(&hostFlag, "host", "", `Gerrit host to use.  Defaults to gerrit host specified in manifest.`)
	cmdCLMail.Flags.StringVar(&messageFlag, "m", "", `CL description.`)
	cmdCLMail.Flags.StringVar(&commitMessageBodyFlag, "commit-message-body-file", "", `file containing the body of the CL description, that is, text without a ChangeID, MultiPart etc.`)
	cmdCLMail.Flags.StringVar(&presubmitFlag, "presubmit", string(gerrit.PresubmitTestTypeAll),
		fmt.Sprintf("The type of presubmit tests to run. Valid values: %s.", strings.Join(gerrit.PresubmitTestTypes(), ",")))
	cmdCLMail.Flags.StringVar(&remoteBranchFlag, "remote-branch", "master", `Name of the remote branch the CL pertains to, without the leading "origin/".`)
	cmdCLMail.Flags.StringVar(&reviewersFlag, "r", "", `Comma-seperated list of emails or LDAPs to request review.`)
	cmdCLMail.Flags.BoolVar(&setTopicFlag, "set-topic", true, `Set Gerrit CL topic.`)
	cmdCLMail.Flags.StringVar(&topicFlag, "topic", "", `CL topic, defaults to <username>-<branchname>.`)
	cmdCLMail.Flags.BoolVar(&uncommittedFlag, "check-uncommitted", true, `Check that no uncommitted changes exist.`)
	cmdCLMail.Flags.BoolVar(&verifyFlag, "verify", true, `Run pre-push git hooks.`)
	cmdCLMail.Flags.BoolVar(&currentProjectFlag, "current-project-only", true, `Run mail in the current project only.`)
	cmdCLMail.Flags.BoolVar(&cleanupMultiPartFlag, "clean-multipart-metadata", false, `Cleanup the metadata associated with multipart CLs pertaining the MultiPart: x/y message without mailing any CLs.`)
	cmdCLSync.Flags.StringVar(&remoteBranchFlag, "remote-branch", "master", `Name of the remote branch the CL pertains to, without the leading "origin/".`)
}

func getCommitMessageFileName(jirix *jiri.X, branch string) (string, error) {
	topLevel, err := gitutil.New(jirix.NewSeq()).TopLevel()
	if err != nil {
		return "", err
	}
	return filepath.Join(topLevel, jiri.ProjectMetaDir, branch, commitMessageFileName), nil
}

func getDependencyPathFileName(jirix *jiri.X, branch string) (string, error) {
	topLevel, err := gitutil.New(jirix.NewSeq()).TopLevel()
	if err != nil {
		return "", err
	}
	return filepath.Join(topLevel, jiri.ProjectMetaDir, branch, dependencyPathFileName), nil
}

func getDependentCLs(jirix *jiri.X, branch string) ([]string, error) {
	file, err := getDependencyPathFileName(jirix, branch)
	if err != nil {
		return nil, err
	}
	data, err := jirix.NewSeq().ReadFile(file)
	var branches []string
	if err != nil {
		if !runutil.IsNotExist(err) {
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

// cmdCL represents the "jiri cl" command.
var cmdCL *cmdline.Command

// Use a factory to avoid an initialization loop between between the
// Runner function and the ParsedFlags field in the Command.
func newCmdCL() *cmdline.Command {
	return &cmdline.Command{
		Name:     "cl",
		Short:    "Manage changelists for multiple projects",
		Long:     "Manage changelists for multiple projects.",
		Children: []*cmdline.Command{cmdCLCleanup, cmdCLMail, cmdCLNew, cmdCLSync},
	}
}

// cmdCLCleanup represents the "jiri cl cleanup" command.
//
// TODO(jsimsa): Replace this with a "submit" command that talks to
// Gerrit to submit the CL and then (optionally) removes it locally.
var cmdCLCleanup = &cmdline.Command{
	Runner: jiri.RunnerFunc(runCLCleanup),
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

func cleanupCL(jirix *jiri.X, branches []string) (e error) {
	git := gitutil.New(jirix.NewSeq())
	originalBranch, err := git.CurrentBranchName()
	if err != nil {
		return err
	}
	stashed, err := git.Stash()
	if err != nil {
		return err
	}
	if stashed {
		defer collect.Error(func() error { return git.StashPop() }, &e)
	}
	if err := git.CheckoutBranch(remoteBranchFlag); err != nil {
		return err
	}
	checkoutOriginalBranch := true
	defer collect.Error(func() error {
		if checkoutOriginalBranch {
			return git.CheckoutBranch(originalBranch)
		}
		return nil
	}, &e)
	if err := git.FetchRefspec("origin", remoteBranchFlag); err != nil {
		return err
	}
	s := jirix.NewSeq()
	for _, branch := range branches {
		cleanupFn := func() error { return cleanupBranch(jirix, branch) }
		if err := s.Call(cleanupFn, "Cleaning up branch: %s", branch).Done(); err != nil {
			return err
		}
		if branch == originalBranch {
			checkoutOriginalBranch = false
		}
	}
	return nil
}

func cleanupBranch(jirix *jiri.X, branch string) error {
	git := gitutil.New(jirix.NewSeq())
	if err := git.CheckoutBranch(branch); err != nil {
		return err
	}
	if !forceFlag {
		trackingBranch := "origin/" + remoteBranchFlag
		if err := git.Merge(trackingBranch); err != nil {
			return err
		}
		files, err := git.ModifiedFiles(trackingBranch, branch)
		if err != nil {
			return err
		}
		if len(files) != 0 {
			return fmt.Errorf("unmerged changes in\n%s", strings.Join(files, "\n"))
		}
	}
	if err := git.CheckoutBranch(remoteBranchFlag); err != nil {
		return err
	}
	if err := git.DeleteBranch(branch, gitutil.ForceOpt(true)); err != nil {
		return err
	}
	reviewBranch := branch + "-REVIEW"
	if git.BranchExists(reviewBranch) {
		if err := git.DeleteBranch(reviewBranch, gitutil.ForceOpt(true)); err != nil {
			return err
		}
	}
	// Delete branch metadata.
	topLevel, err := git.TopLevel()
	if err != nil {
		return err
	}
	s := jirix.NewSeq()
	// Remove the branch from all dependency paths.
	metadataDir := filepath.Join(topLevel, jiri.ProjectMetaDir)
	fileInfos, err := s.RemoveAll(filepath.Join(metadataDir, branch)).
		ReadDir(metadataDir)
	if err != nil {
		return err
	}
	for _, fileInfo := range fileInfos {
		if !fileInfo.IsDir() {
			continue
		}
		file, err := getDependencyPathFileName(jirix, fileInfo.Name())
		if err != nil {
			return err
		}
		data, err := s.ReadFile(file)
		if err != nil {
			if !runutil.IsNotExist(err) {
				return err
			}
			continue
		}
		branches := strings.Split(string(data), "\n")
		for i, tmpBranch := range branches {
			if branch == tmpBranch {
				data := []byte(strings.Join(append(branches[:i], branches[i+1:]...), "\n"))
				if err := s.WriteFile(file, data, os.FileMode(0644)).Done(); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

func runCLCleanup(jirix *jiri.X, args []string) error {
	if len(args) == 0 {
		return jirix.UsageErrorf("cleanup requires at least one argument")
	}
	return cleanupCL(jirix, args)
}

// cmdCLMail represents the "jiri cl mail" command.
var cmdCLMail *cmdline.Command

// Use a factory to avoid an initialization loop between between the
// Runner function and the ParsedFlags field in the Command.
func newCmdCLMail() *cmdline.Command {
	return &cmdline.Command{
		Runner: jiri.RunnerFunc(runCLMail),
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

// currentProject returns the Project containing the current working directory.
// The current working directory must be inside JIRI_ROOT.
func currentProject(jirix *jiri.X) (project.Project, error) {
	dir, err := os.Getwd()
	if err != nil {
		return project.Project{}, fmt.Errorf("os.Getwd() failed: %v", err)
	}

	// Walk up the path until we find a project at that path, or hit the jirix.Root.
	// Note that we can't just compare path prefixes because of soft links.
	for dir != jirix.Root && dir != string(filepath.Separator) {
		p, err := project.ProjectAtPath(jirix, dir)
		if err != nil {
			dir = filepath.Dir(dir)
			continue
		}
		return p, nil
	}
	return project.Project{}, fmt.Errorf("directory %q is not contained in a project", dir)
}

type multiPart struct {
	clean, current bool
	currentKey     project.ProjectKey
	states         map[project.ProjectKey]*project.ProjectState
	keys           project.ProjectKeys
}

// initForMultiPart determines the actions to be taken
// based on command line flags and project state.
func initForMultiPart(jirix *jiri.X) (*multiPart, error) {
	mp := &multiPart{}
	mp.clean = cleanupMultiPartFlag
	if currentProjectFlag {
		mp.current = true
		return mp, nil
	}
	if mp.clean {
		states, keys, err := projectStates(jirix, true)
		if err != nil {
			return nil, err
		}
		mp.states = states
		mp.keys = keys
		return mp, nil
	}
	states, keys, err := projectStates(jirix, false)
	if err != nil {
		return nil, err
	}
	current, err := currentProject(jirix)
	if err != nil {
		return nil, err
	}
	mp.currentKey = current.Key()
	if len(keys) == 1 {
		filename := filepath.Join(states[keys[0]].Project.Path, jiri.ProjectMetaDir, multiPartMetaDataFileName)
		os.Remove(filename)
		if mp.currentKey == states[keys[0]].Project.Key() {
			mp.current = true
			return mp, nil
		}
	}
	mp.states = states
	mp.keys = keys
	return mp, nil
}

// projectStates returns a map with all projects that are on the same
// current branch as the current project, as well as a slice of their
// project keys sorted lexicographically. Unless "allowdirty" is true,
// an error is returned if any matching project has uncommitted changes.
// The keys are returned, sorted, to avoid the caller having to recreate
// the them by iterating over the map.
func projectStates(jirix *jiri.X, allowdirty bool) (map[project.ProjectKey]*project.ProjectState, project.ProjectKeys, error) {
	git := gitutil.New(jirix.NewSeq())
	branch, err := git.CurrentBranchName()
	if err != nil {
		return nil, nil, err
	}
	states, err := project.GetProjectStates(jirix, false)
	if err != nil {
		return nil, nil, err
	}
	uncommitted := []string{}
	var keys project.ProjectKeys
	for _, s := range states {
		if s.CurrentBranch == branch {
			key := s.Project.Key()
			fullState, err := project.GetProjectState(jirix, key, true)
			if err != nil {
				return nil, nil, err
			}
			if !allowdirty && fullState.HasUncommitted {
				uncommitted = append(uncommitted, string(key))
			} else {
				keys = append(keys, key)
			}
		}
	}
	if len(uncommitted) > 0 {
		return nil, nil, fmt.Errorf("the following projects have uncommitted changes: %s", strings.Join(uncommitted, ", "))
	}
	members := map[project.ProjectKey]*project.ProjectState{}
	for _, key := range keys {
		members[key] = states[key]
	}
	if len(members) == 0 {
		return nil, nil, nil
	}
	sort.Sort(keys)
	return members, keys, nil
}

func (mp *multiPart) writeMultiPartMetadata(jirix *jiri.X) error {
	total := len(mp.states)
	index := 1
	s := jirix.NewSeq()
	for _, key := range mp.keys {
		state := mp.states[key]
		filename := filepath.Join(state.Project.Path, jiri.ProjectMetaDir, multiPartMetaDataFileName)
		if total < 2 {
			os.Remove(filename)
			continue
		}
		msg := fmt.Sprintf("MultiPart: %d/%d\n", index, total)
		if err := s.WriteFile(filename, []byte(msg), os.FileMode(0644)).Done(); err != nil {
			return err
		}
		index++
	}
	return nil
}

func (mp *multiPart) cleanMultiPartMetadata(jirix *jiri.X) error {
	s := jirix.NewSeq()
	for _, state := range mp.states {
		filename := filepath.Join(state.Project.Path, jiri.ProjectMetaDir, multiPartMetaDataFileName)
		ok, err := s.IsFile(filename)
		if err != nil {
			return err
		}
		if ok {
			if err := s.Remove(filename).Done(); err != nil {
				return err
			}
		}
	}
	return nil
}

func (mp *multiPart) commandline(excludeKey project.ProjectKey, args []string) []string {
	keyflag := "--projects="
	for _, k := range mp.keys {
		if k == excludeKey {
			continue
		}
		keyflag += string(k) + ","
	}
	keyflag = strings.TrimSuffix(keyflag, ",")
	clargs := []string{
		"runp",
		"--interactive",
		keyflag,
		"jiri",
		"cl",
		"mail",
		"--current-project-only=true",
	}
	return append(clargs, args...)
}

// clMailMultiFlags extracts flags from the invocation of cl mail
// that should be passed on to the sub invocations of cl mail when
// operating across multiple repos.
// These are:
// -autosubmit, -cc, -d, -edit, -host, -m, -presubmit, remote-branch, -r,
// -set-topic, -topic, -check-uncommitted and -verify,
func clMailMultiFlags() []string {
	flags := []string{}
	stringFlag := func(name, value string) {
		if profilescmdline.IsFlagSet(cmdCLMail.ParsedFlags, name) {
			flags = append(flags, fmt.Sprintf("--%s=%s", name, value))
		}
	}
	boolFlag := func(name string, value bool) {
		if profilescmdline.IsFlagSet(cmdCLMail.ParsedFlags, name) {
			flags = append(flags, fmt.Sprintf("--%s=%t", name, value))
		}
	}

	// --edit is handled differently to other flags, if it is not
	// specifically set, the default is to run the editor once
	// and then reuse that message for the other parts of a multipart
	// CL - that is, set -edit=false for the other repos. If edit
	// is specifically set then that setting is used for all repos.
	// So using --edit=true allows for a different CL message in
	// each repo of a multipart CL.
	if profilescmdline.IsFlagSet(cmdCLMail.ParsedFlags, "edit") {
		// if --edit is set on the command line, use that value
		// for all subcommands
		flags = append(flags, fmt.Sprintf("--edit=%t", editFlag))
	} else {
		// if --edit is not set on the command line, use --edit=false
		// for subcommands.
		flags = append(flags, "--edit=false")
	}

	boolFlag("autosubmit", autosubmitFlag)
	stringFlag("cc", ccsFlag)
	boolFlag("d", draftFlag)
	stringFlag("host", hostFlag)
	stringFlag("m", messageFlag)
	stringFlag("presubmit", presubmitFlag)
	stringFlag("remote-branch", remoteBranchFlag)
	stringFlag("r", reviewersFlag)
	boolFlag("set-topic", setTopicFlag)
	boolFlag("check-uncommitted", uncommittedFlag)
	boolFlag("verify", verifyFlag)
	return flags
}

// runCLMail is a wrapper that sets up and runs a review instance across
// multiple projects.
func runCLMail(jirix *jiri.X, args []string) error {
	mp, err := initForMultiPart(jirix)
	if err != nil {
		return err
	}
	if mp.clean {
		if err := mp.cleanMultiPartMetadata(jirix); err != nil {
			return err
		}
		return nil
	}
	if mp.current {
		return runCLMailCurrent(jirix, []string{})
	}
	// multipart mode
	if err := mp.writeMultiPartMetadata(jirix); err != nil {
		mp.cleanMultiPartMetadata(jirix)
		return err
	}
	if err := runCLMailCurrent(jirix, []string{}); err != nil {
		return err
	}
	git := gitutil.New(jirix.NewSeq())
	branch, err := git.CurrentBranchName()
	if err != nil {
		return err
	}
	initialMessage, err := strippedGerritCommitMessage(jirix, branch)
	if err != nil {
		return err
	}
	s := jirix.NewSeq()
	tmp, err := s.TempFile("", branch+"-")
	if err != nil {
		return err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()
	if _, err := io.WriteString(tmp, initialMessage); err != nil {
		return err
	}
	// Use Capture to make sure that all output from the subcommands is
	// sent to stdout/stderr.
	flags := clMailMultiFlags()
	flags = append(flags, "--commit-message-body-file="+tmp.Name())
	return s.Capture(jirix.Stdout(), jirix.Stderr()).Last("jiri", mp.commandline(mp.currentKey, append(flags, args...))...)
}

func runCLMailCurrent(jirix *jiri.X, _ []string) error {
	// Check that working dir exist on remote branch.  Otherwise checking out
	// remote branch will break the users working dir.
	git := gitutil.New(jirix.NewSeq())
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	topLevel, err := git.TopLevel()
	if err != nil {
		return err
	}
	relWd, err := filepath.Rel(topLevel, wd)
	if err != nil {
		return err
	}
	if !git.DirExistsOnBranch(relWd, remoteBranchFlag) {
		return fmt.Errorf("directory %q does not exist on branch %q.\nPlease run 'jiri cl mail' from root directory of this repo.", relWd, remoteBranchFlag)
	}

	// Sanity checks for the <presubmitFlag> flag.
	if !checkPresubmitFlag() {
		return jirix.UsageErrorf("invalid value for the -presubmit flag. Valid values: %s.",
			strings.Join(gerrit.PresubmitTestTypes(), ","))
	}

	p, err := currentProject(jirix)
	if err != nil {
		return err
	}

	host := hostFlag
	if host == "" {
		if p.GerritHost == "" {
			return fmt.Errorf("No gerrit host found.  Please use the '--host' flag, or add a 'gerrithost' attribute for project %q.", p.Name)
		}
		host = p.GerritHost
	}
	hostUrl, err := url.Parse(host)
	if err != nil {
		return fmt.Errorf("invalid Gerrit host %q: %v", host, err)
	}
	projectRemoteUrl, err := url.Parse(p.Remote)
	if err != nil {
		return fmt.Errorf("invalid project remote: %v", p.Remote, err)
	}
	gerritRemote := *hostUrl
	gerritRemote.Path = projectRemoteUrl.Path

	// Create and run the review.
	review, err := newReview(jirix, p, gerrit.CLOpts{
		Autosubmit:   autosubmitFlag,
		Ccs:          parseEmails(ccsFlag),
		Draft:        draftFlag,
		Edit:         editFlag,
		Remote:       gerritRemote.String(),
		Host:         hostUrl,
		Presubmit:    gerrit.PresubmitTestType(presubmitFlag),
		RemoteBranch: remoteBranchFlag,
		Reviewers:    parseEmails(reviewersFlag),
		Verify:       verifyFlag,
	})
	if err != nil {
		return err
	}
	if confirmed, err := review.confirmFlagChanges(); err != nil {
		return err
	} else if !confirmed {
		return nil
	}
	err = review.run()
	// Ignore the error that is returned when there are no differences
	// between the local and gerrit branches.
	if err != nil && noChangesRE.MatchString(err.Error()) {
		return nil
	}
	return err
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
func checkDependents(jirix *jiri.X) (e error) {
	originalBranch, err := gitutil.New(jirix.NewSeq()).CurrentBranchName()
	if err != nil {
		return err
	}
	branches, err := getDependentCLs(jirix, originalBranch)
	if err != nil {
		return err
	}
	for i := 1; i < len(branches); i++ {
		file, err := getCommitMessageFileName(jirix, branches[i])
		if err != nil {
			return err
		}
		if _, err := jirix.NewSeq().Stat(file); err != nil {
			if !runutil.IsNotExist(err) {
				return err
			}
			return fmt.Errorf(`Failed to export the branch %q to Gerrit because its ancestor %q has not been exported to Gerrit yet.
The following steps are needed before the operation can be retried:
$ git checkout %v
$ jiri cl mail
$ git checkout %v
# retry the original command
`, originalBranch, branches[i], branches[i], originalBranch)
		}
	}

	return nil
}

type review struct {
	jirix        *jiri.X
	reviewBranch string
	project      project.Project
	gerrit.CLOpts
}

func newReview(jirix *jiri.X, project project.Project, opts gerrit.CLOpts) (*review, error) {
	// Sync all CLs in the sequence of dependent CLs ending in the
	// current branch.
	if err := syncCL(jirix); err != nil {
		return nil, err
	}

	// Make sure that all CLs in the above sequence (possibly except for
	// the current branch) have been exported to Gerrit. This is needed
	// to make sure we have commit messages for all but the last CL.
	//
	// NOTE: The alternative here is to prompt the user for multiple
	// commit messages, which seems less user friendly.
	if err := checkDependents(jirix); err != nil {
		return nil, err
	}

	branch, err := gitutil.New(jirix.NewSeq()).CurrentBranchName()
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
		jirix:        jirix,
		project:      project,
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
	file, err := getCommitMessageFileName(review.jirix, review.CLOpts.Branch)
	if err != nil {
		return false, err
	}
	bytes, err := review.jirix.NewSeq().ReadFile(file)
	if err != nil {
		if runutil.IsNotExist(err) {
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
	git := gitutil.New(review.jirix.NewSeq())
	if err := git.CheckoutBranch(review.CLOpts.Branch); err != nil {
		return err
	}
	if git.BranchExists(review.reviewBranch) {
		if err := git.DeleteBranch(review.reviewBranch, gitutil.ForceOpt(true)); err != nil {
			return err
		}
	}
	if stashed {
		if err := git.StashPop(); err != nil {
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
	git := gitutil.New(review.jirix.NewSeq())
	// Create the review branch.
	if err := git.FetchRefspec("origin", review.CLOpts.RemoteBranch); err != nil {
		return err
	}
	if git.BranchExists(review.reviewBranch) {
		if err := git.DeleteBranch(review.reviewBranch, gitutil.ForceOpt(true)); err != nil {
			return err
		}
	}
	upstream := "origin/" + review.CLOpts.RemoteBranch
	if err := git.CreateBranchWithUpstream(review.reviewBranch, upstream); err != nil {
		return err
	}
	if err := git.CheckoutBranch(review.reviewBranch); err != nil {
		return err
	}
	// Register a cleanup handler in case of subsequent errors.
	cleanup := true
	defer collect.Error(func() error {
		if !cleanup {
			return git.CheckoutBranch(review.CLOpts.Branch)
		}
		git.CheckoutBranch(review.CLOpts.Branch, gitutil.ForceOpt(true))
		git.DeleteBranch(review.reviewBranch, gitutil.ForceOpt(true))
		return nil
	}, &e)

	// Report an error if the CL is empty.
	hasDiff, err := git.BranchesDiffer(review.CLOpts.Branch, review.reviewBranch)
	if err != nil {
		return err
	}
	if !hasDiff {
		return emptyChangeError(struct{}{})
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
	branches, err := getDependentCLs(review.jirix, review.CLOpts.Branch)
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
	git := gitutil.New(review.jirix.NewSeq())
	for i := 1; i < len(branches); i++ {
		// We want to merge the <branches[i]> branch on top of the review
		// branch, forcing all conflicts to be reviewed in favor of the
		// <branches[i]> branch. Unfortunately, git merge does not offer a
		// strategy that would do that for us. The solution implemented
		// here is based on:
		//
		// http://stackoverflow.com/questions/173919/is-there-a-theirs-version-of-git-merge-s-ours
		if err := git.Merge(branches[i], gitutil.SquashOpt(true), gitutil.StrategyOpt("ours")); err != nil {
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
		// this was not the case, consecutive invocations of "jiri cl mail"
		// could fail if some, but not all, of the dependent CLs submitted
		// to Gerrit have changed.
		output, err := git.Log(branches[i], branches[i]+"^", "%ad%n%cd")
		if err != nil {
			return err
		}
		if len(output) < 1 || len(output[0]) < 2 {
			return fmt.Errorf("unexpected output length: %v", output)
		}
		authorDate := gitutil.AuthorDateOpt(output[0][0])
		committer := gitutil.CommitterDateOpt(output[0][1])
		git = gitutil.New(review.jirix.NewSeq(), authorDate, committer)
		if i < len(branches)-1 {
			file, err := getCommitMessageFileName(review.jirix, branches[i])
			if err != nil {
				return err
			}
			message, err := review.jirix.NewSeq().ReadFile(file)
			if err != nil {
				return err
			}
			if err := git.CommitWithMessage(string(message)); err != nil {
				return err
			}
		} else {
			committer := git.NewCommitter(review.CLOpts.Edit)
			if err := committer.Commit(message); err != nil {
				return err
			}
		}
		tmpBranch := review.reviewBranch + "-" + branches[i] + "-TMP"
		if err := git.CreateBranch(tmpBranch); err != nil {
			return err
		}
		defer collect.Error(func() error {
			return git.DeleteBranch(tmpBranch, gitutil.ForceOpt(true))
		}, &e)
		if err := git.Reset(branches[i]); err != nil {
			return err
		}
		if err := git.Reset(tmpBranch, gitutil.ModeOpt("soft")); err != nil {
			return err
		}
		if err := git.CommitAmend(); err != nil {
			return err
		}
	}
	return nil
}

func (review *review) readMultiPart() string {
	s := review.jirix.NewSeq()
	filename := filepath.Join(review.project.Path, jiri.ProjectMetaDir, multiPartMetaDataFileName)
	mpart, err := s.ReadFile(filename)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(mpart))
}

// strippedGerritCommitMessage returns the commit message stripped of variable
// meta-data such as multipart messages, or change IDs:
func strippedGerritCommitMessage(jirix *jiri.X, branch string) (string, error) {
	filename, err := getCommitMessageFileName(jirix, branch)
	if err != nil {
		return "", err
	}
	msg, err := jirix.NewSeq().ReadFile(filename)
	if err != nil {
		return "", err
	}
	// Strip "MultiPart ..." from the commit messages.
	strippedMessages := multiPartRE.ReplaceAllLiteralString(string(msg), "")
	// Strip "Change-Id: ..." from the commit messages.
	strippedMessages = changeIDRE.ReplaceAllLiteralString(strippedMessages, "")
	return strippedMessages, nil
}

// defaultCommitMessage creates the default commit message from the
// list of commits on the branch.
func (review *review) defaultCommitMessage() (string, error) {
	commitMessages := ""
	var err error
	if commitMessageBodyFlag != "" {
		msg, tmpErr := ioutil.ReadFile(commitMessageBodyFlag)
		commitMessages = string(msg)
		err = tmpErr
	} else {
		commitMessages, err = gitutil.New(review.jirix.NewSeq()).CommitMessages(review.CLOpts.Branch, review.reviewBranch)
	}
	if err != nil {
		return "", err
	}
	// Strip "MultiPart ..." from the commit messages.
	strippedMessages := multiPartRE.ReplaceAllLiteralString(commitMessages, "")
	// Strip "Change-Id: ..." from the commit messages.
	strippedMessages = changeIDRE.ReplaceAllLiteralString(strippedMessages, "")
	// Add comment markers (#) to every line.
	commentedMessages := "# " + strings.Replace(strippedMessages, "\n", "\n# ", -1)
	message := defaultMessageHeader + commentedMessages
	if multipart := review.readMultiPart(); multipart != "" {
		message = message + "\n" + multipart + "\n"
	}
	return message, nil
}

// ensureChangeID makes sure that the last commit contains a Change-Id, and
// returns an error if it does not.
func (review *review) ensureChangeID() error {
	latestCommitMessage, err := gitutil.New(review.jirix.NewSeq()).LatestCommitMessage()
	if err != nil {
		return err
	}
	changeID := changeIDRE.FindString(latestCommitMessage)
	if changeID == "" {
		return noChangeIDError(struct{}{})
	}
	return nil
}

// processLabelsAndCommitFile adds/removes labels for the given commit
// message and merges in the contents of the initial-message-file.
func (review *review) processLabelsAndCommitFile(message string) string {
	// Find the Change-ID and MultiPart lines.
	changeIDLine := changeIDRE.FindString(message)
	multiPartLine := multiPartRE.FindString(message)

	if commitMessageBodyFlag != "" {
		if msg, err := ioutil.ReadFile(commitMessageBodyFlag); err == nil {
			message = string(msg)
		}
	}

	// Strip existing labels and change-ID.
	message = autosubmitLabelRE.ReplaceAllLiteralString(message, "")
	message = presubmitTestLabelRE.ReplaceAllLiteralString(message, "")
	message = changeIDRE.ReplaceAllLiteralString(message, "")
	message = multiPartRE.ReplaceAllLiteralString(message, "")

	// Insert labels and change-ID back.
	if review.CLOpts.Autosubmit {
		message += fmt.Sprintf("AutoSubmit\n")
	}
	if review.CLOpts.Presubmit != gerrit.PresubmitTestTypeAll {
		message += fmt.Sprintf("PresubmitTest: %s\n", review.CLOpts.Presubmit)
	}
	if multiPartLine != "" && !strings.HasSuffix(message, "\n") {
		message += "\n"
	} else {
		if multipart := review.readMultiPart(); multipart != "" {
			if !strings.HasSuffix(message, "\n") {
				message += "\n"
			}
			multiPartLine = multipart
		}
	}
	message += multiPartLine
	if changeIDLine != "" && !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	message += changeIDLine
	return message
}

// run implements checks that the review passes all local checks
// and then mails it to Gerrit.
func (review *review) run() (e error) {
	git := gitutil.New(review.jirix.NewSeq())
	if uncommittedFlag {
		changes, err := git.FilesWithUncommittedChanges()
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
	stashed, err := git.Stash()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return review.cleanup(stashed) }, &e)
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	topLevel, err := git.TopLevel()
	if err != nil {
		return err
	}
	s := review.jirix.NewSeq()
	if err := s.Chdir(topLevel).Done(); err != nil {
		return err
	}
	defer collect.Error(func() error { return review.jirix.NewSeq().Chdir(wd).Done() }, &e)

	file, err := getCommitMessageFileName(review.jirix, review.CLOpts.Branch)
	if err != nil {
		return err
	}

	message := messageFlag
	if message == "" {
		// Message was not passed in flag.  Attempt to read it from file.
		data, err := s.ReadFile(file)
		if err != nil {
			if !runutil.IsNotExist(err) {
				return err
			}
		} else {
			message = string(data)
		}
	}

	// Add/remove labels to/from the commit message before asking users
	// to edit it. We do this only when this is not the initial commit
	// where the message is empty.
	//
	// For the initial commit, the labels will be processed after the
	// message is edited by users, which happens in the
	// updateReviewMessage method.
	if message != "" {
		message = review.processLabelsAndCommitFile(message)
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
	if err := review.ensureChangeID(); err != nil {
		return err
	}
	if err := gerrit.Push(review.jirix.NewSeq(), review.CLOpts); err != nil {
		return gerritError(err.Error())
	}
	return nil
}

// getChangeID reads the commit message and extracts the change-Id
func (review *review) getChangeID() (string, error) {
	file, err := getCommitMessageFileName(review.jirix, review.CLOpts.Branch)
	if err != nil {
		return "", err
	}
	bytes, err := review.jirix.NewSeq().ReadFile(file)
	if err != nil {
		return "", err
	}
	changeID := changeIDRE.FindSubmatch(bytes)
	if changeID == nil || len(changeID) < 2 {
		return "", fmt.Errorf("could not find Change-Id in:\n%s", bytes)
	}
	return string(changeID[1]), nil
}

// setTopic sets the topic for the CL corresponding to the branch the
// review was created for.
func (review *review) setTopic() error {
	changeID, err := review.getChangeID()
	if err != nil {
		return err
	}
	host := review.CLOpts.Host
	if host.Scheme != "http" && host.Scheme != "https" {
		return fmt.Errorf("Cannot set topic for gerrit host %q. Please use a host url with 'https' scheme or run with '--set-topic=false'.", host.String())
	}
	if err := review.jirix.Gerrit(host).SetTopic(changeID, review.CLOpts); err != nil {
		return fmt.Errorf("failed to set topic for %v, %#v: %v", changeID, review.CLOpts, err)
	}
	return nil
}

// updateReviewMessage writes the commit message to the given file.
func (review *review) updateReviewMessage(file string) error {
	git := gitutil.New(review.jirix.NewSeq())
	if err := git.CheckoutBranch(review.reviewBranch); err != nil {
		return err
	}
	newMessage, err := git.LatestCommitMessage()
	if err != nil {
		return err
	}
	// update MultiPart metadata.
	mpart := review.readMultiPart()
	newMessage = multiPartRE.ReplaceAllLiteralString(newMessage, mpart)
	s := review.jirix.NewSeq()
	// For the initial commit where the commit message file doesn't exist,
	// add/remove labels after users finish editing the commit message.
	//
	// This behavior is consistent with how Change-ID is added for the
	// initial commit so we don't confuse users.
	if _, err := s.Stat(file); err != nil {
		if runutil.IsNotExist(err) {
			newMessage = review.processLabelsAndCommitFile(newMessage)
			if err := git.CommitAmendWithMessage(newMessage); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	topLevel, err := git.TopLevel()
	if err != nil {
		return err
	}
	newMetadataDir := filepath.Join(topLevel, jiri.ProjectMetaDir, review.CLOpts.Branch)
	if err := s.MkdirAll(newMetadataDir, os.FileMode(0755)).
		WriteFile(file, []byte(newMessage), 0644).Done(); err != nil {
		return err
	}
	return nil
}

// cmdCLNew represents the "jiri cl new" command.
var cmdCLNew = &cmdline.Command{
	Runner: jiri.RunnerFunc(runCLNew),
	Name:   "new",
	Short:  "Create a new local branch for a changelist",
	Long: fmt.Sprintf(`
Command "new" creates a new local branch for a changelist. In
particular, it forks a new branch with the given name from the current
branch and records the relationship between the current branch and the
new branch in the %v metadata directory. The information recorded in
the %v metadata directory tracks dependencies between CLs and is used
by the "jiri cl sync" and "jiri cl mail" commands.
`, jiri.ProjectMetaDir, jiri.ProjectMetaDir),
	ArgsName: "<name>",
	ArgsLong: "<name> is the changelist name.",
}

func runCLNew(jirix *jiri.X, args []string) error {
	if got, want := len(args), 1; got != want {
		return jirix.UsageErrorf("unexpected number of arguments: got %v, want %v", got, want)
	}
	return newCL(jirix, args)
}

func newCL(jirix *jiri.X, args []string) error {
	git := gitutil.New(jirix.NewSeq())
	topLevel, err := git.TopLevel()
	if err != nil {
		return err
	}
	originalBranch, err := git.CurrentBranchName()
	if err != nil {
		return err
	}

	// Create a new branch using the current branch.
	newBranch := args[0]
	if err := git.CreateAndCheckoutBranch(newBranch); err != nil {
		return err
	}

	// Register a cleanup handler in case of subsequent errors.
	cleanup := true
	defer func() {
		if cleanup {
			git.CheckoutBranch(originalBranch, gitutil.ForceOpt(true))
			git.DeleteBranch(newBranch, gitutil.ForceOpt(true))
		}
	}()

	s := jirix.NewSeq()
	// Record the dependent CLs for the new branch. The dependent CLs
	// are recorded in a <dependencyPathFileName> file as a
	// newline-separated list of branch names.
	branches, err := getDependentCLs(jirix, originalBranch)
	if err != nil {
		return err
	}
	branches = append(branches, originalBranch)
	newMetadataDir := filepath.Join(topLevel, jiri.ProjectMetaDir, newBranch)
	if err := s.MkdirAll(newMetadataDir, os.FileMode(0755)).Done(); err != nil {
		return err
	}
	file, err := getDependencyPathFileName(jirix, newBranch)
	if err != nil {
		return err
	}
	if err := s.WriteFile(file, []byte(strings.Join(branches, "\n")), os.FileMode(0644)).Done(); err != nil {
		return err
	}

	cleanup = false
	return nil
}

// cmdCLSync represents the "jiri cl sync" command.
var cmdCLSync = &cmdline.Command{
	Runner: jiri.RunnerFunc(runCLSync),
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
`, jiri.ProjectMetaDir),
}

func runCLSync(jirix *jiri.X, _ []string) error {
	return syncCL(jirix)
}

func syncCL(jirix *jiri.X) (e error) {
	git := gitutil.New(jirix.NewSeq())
	stashed, err := git.Stash()
	if err != nil {
		return err
	}
	if stashed {
		defer collect.Error(func() error { return git.StashPop() }, &e)
	}

	// Register a cleanup handler in case of subsequent errors.
	forceOriginalBranch := true
	originalBranch, err := git.CurrentBranchName()
	if err != nil {
		return err
	}
	originalWd, err := os.Getwd()
	if err != nil {
		return err
	}

	defer func() {
		if forceOriginalBranch {
			git.CheckoutBranch(originalBranch, gitutil.ForceOpt(true))
		}
		jirix.NewSeq().Chdir(originalWd)
	}()

	s := jirix.NewSeq()
	// Switch to an existing directory in master so we can run commands.
	topLevel, err := git.TopLevel()
	if err != nil {
		return err
	}
	if err := s.Chdir(topLevel).Done(); err != nil {
		return err
	}

	// Identify the dependents CLs leading to (and including) the
	// current branch.
	branches, err := getDependentCLs(jirix, originalBranch)
	if err != nil {
		return err
	}
	branches = append(branches, originalBranch)

	// Sync from upstream.
	if err := git.CheckoutBranch(branches[0]); err != nil {
		return err
	}
	if err := git.Pull("origin", branches[0]); err != nil {
		return err
	}

	// Bring all CLs in the sequence of dependent CLs leading to the
	// current branch up to date with the <remoteBranchFlag> branch.
	for i := 1; i < len(branches); i++ {
		if err := git.CheckoutBranch(branches[i]); err != nil {
			return err
		}
		if err := git.Merge(branches[i-1]); err != nil {
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

	forceOriginalBranch = false
	return nil
}
