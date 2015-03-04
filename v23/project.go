package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"v.io/x/devtools/lib/util"
	"v.io/x/lib/cmdline"
)

var (
	branchesFlag   bool
	noPristineFlag bool
	checkDirtyFlag bool
	showNameFlag   bool
)

func init() {
	cmdProjectPoll.Flags.StringVar(&manifestFlag, "manifest", "", "Name of the project manifest.")
	cmdProjectList.Flags.BoolVar(&branchesFlag, "branches", false, "Show project branches.")
	cmdProjectList.Flags.BoolVar(&noPristineFlag, "nopristine", false, "If true, omit pristine projects, i.e. projects with a clean master branch and no other branches.")
	cmdProjectShellPrompt.Flags.BoolVar(&checkDirtyFlag, "check_dirty", true, "If false, don't check for uncommitted changes or untracked files. Setting this option to false is dangerous: dirty master branches will not appear in the output.")
	cmdProjectShellPrompt.Flags.BoolVar(&showNameFlag, "show_name", false, "Show the name of the current repo.")
}

// cmdProject represents the "v23 project" command.
var cmdProject = &cmdline.Command{
	Name:     "project",
	Short:    "Manage the vanadium projects",
	Long:     "Manage the vanadium projects.",
	Children: []*cmdline.Command{cmdProjectList, cmdProjectShellPrompt, cmdProjectPoll},
}

// cmdProjectList represents the "v23 project list" command.
var cmdProjectList = &cmdline.Command{
	Run:   runProjectList,
	Name:  "list",
	Short: "List existing vanadium projects and branches",
	Long:  "Inspect the local filesystem and list the existing projects and branches.",
}

type repoState struct {
	project        util.Project
	branches       []string
	currentBranch  string
	hasUncommitted bool
	hasUntracked   bool
}

func fillRepoState(ctx *util.Context, rs *repoState, checkDirty bool, ch chan<- error) {
	// TODO(sadovsky): Create a common interface for Git and Hg.
	var err error
	switch rs.project.Protocol {
	case "git":
		scm := ctx.Git(util.RootDirOpt(rs.project.Path))
		rs.branches, rs.currentBranch, err = scm.GetBranches()
		if err != nil {
			ch <- err
			return
		}
		if checkDirty {
			rs.hasUncommitted, err = scm.HasUncommittedChanges()
			if err != nil {
				ch <- err
				return
			}
			rs.hasUntracked, err = scm.HasUntrackedFiles()
			if err != nil {
				ch <- err
				return
			}
		}
	case "hg":
		scm := ctx.Hg(util.RootDirOpt(rs.project.Path))
		rs.branches, rs.currentBranch, err = scm.GetBranches()
		if err != nil {
			ch <- err
			return
		}
		if checkDirty {
			// TODO(sadovsky): Extend hgutil so that we can populate these fields
			// correctly.
			rs.hasUncommitted = false
			rs.hasUntracked = false
		}
	default:
		ch <- util.UnsupportedProtocolErr(rs.project.Protocol)
		return
	}
	ch <- nil
}

// runProjectList generates a listing of local projects.
func runProjectList(command *cmdline.Command, _ []string) error {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	projects, err := util.LocalProjects(ctx)
	if err != nil {
		return err
	}
	names := []string{}
	for name := range projects {
		names = append(names, name)
	}
	sort.Strings(names)

	// Note, goroutine-based parallel forEach is clumsy. :(
	repoStates := make([]repoState, len(names))
	sem := make(chan error, len(names))
	for i, name := range names {
		rs := &repoStates[i]
		rs.project = projects[name]
		go fillRepoState(ctx, rs, noPristineFlag, sem)
	}
	for _ = range names {
		err := <-sem
		if err != nil {
			return err
		}
	}

	for i, name := range names {
		rs := &repoStates[i]
		if noPristineFlag {
			pristine := len(rs.branches) == 1 && rs.currentBranch == "master" && !rs.hasUncommitted && !rs.hasUntracked
			if pristine {
				continue
			}
		}
		fmt.Fprintf(ctx.Stdout(), "project=%q path=%q\n", path.Base(name), rs.project.Path)
		if branchesFlag {
			for _, branch := range rs.branches {
				if branch == rs.currentBranch {
					fmt.Fprintf(ctx.Stdout(), "  * %v\n", branch)
				} else {
					fmt.Fprintf(ctx.Stdout(), "  %v\n", branch)
				}
			}
		}
	}
	return nil
}

// cmdProjectShellPrompt represents the "v23 project shell-prompt" command.
var cmdProjectShellPrompt = &cmdline.Command{
	Run:   runProjectShellPrompt,
	Name:  "shell-prompt",
	Short: "Print a succinct status of projects, suitable for shell prompts",
	Long: `
Reports current branches of vanadium projects (repositories) as well as an
indication of each project's status:
  *  indicates that a repository contains uncommitted changes
  %  indicates that a repository contains untracked files
`,
}

func runProjectShellPrompt(command *cmdline.Command, args []string) error {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	projects, err := util.LocalProjects(ctx)
	if err != nil {
		return err
	}
	names := []string{}
	for name := range projects {
		names = append(names, name)
	}
	sort.Strings(names)

	// Note, goroutine-based parallel forEach is clumsy. :(
	repoStates := make([]repoState, len(names))
	sem := make(chan error, len(names))
	for i, name := range names {
		rs := &repoStates[i]
		rs.project = projects[name]
		go fillRepoState(ctx, rs, checkDirtyFlag, sem)
	}
	for _ = range names {
		err := <-sem
		if err != nil {
			return err
		}
	}

	// Get the name of the current repository from .v23/metadata.v2, if applicable.
	currentProjectName, err := getRepoName(ctx)
	if err != nil {
		return err
	}
	var statuses []string
	for i, name := range names {
		rs := &repoStates[i]
		status := ""
		if checkDirtyFlag {
			if rs.hasUncommitted {
				status += "*"
			}
			if rs.hasUntracked {
				status += "%"
			}
		}
		short := rs.currentBranch + status
		long := filepath.Base(name) + ":" + short
		if name == currentProjectName {
			if showNameFlag {
				statuses = append([]string{long}, statuses...)
			} else {
				statuses = append([]string{short}, statuses...)
			}
		} else {
			pristine := rs.currentBranch == "master"
			if checkDirtyFlag {
				pristine = pristine && !rs.hasUncommitted && !rs.hasUntracked
			}
			if !pristine {
				statuses = append(statuses, long)
			}
		}
	}
	fmt.Println(strings.Join(statuses, ","))
	return nil
}

// getRepoName gets the name of a repo from the current directory by reading
// the .v23/metadata.v2 file located at the root of the repo.
func getRepoName(ctx *util.Context) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("Getwd() failed: %v", err)
	}
	for wd != "/" {
		curV23Dir := filepath.Join(wd, ".v23")
		if _, err := os.Stat(curV23Dir); err == nil {
			metadataFile := filepath.Join(curV23Dir, "metadata.v2")
			bytes, err := ctx.Run().ReadFile(metadataFile)
			if err != nil {
				return "", err
			}
			var project util.Project
			if err := xml.Unmarshal(bytes, &project); err != nil {
				return "", fmt.Errorf("Unmarshal() failed: %v", err)
			}
			return project.Name, nil
		}
		wd = filepath.Dir(wd)
	}
	return "", nil
}

// cmdProjectPoll represents the "v23 project poll" command.
var cmdProjectPoll = &cmdline.Command{
	Run:   runProjectPoll,
	Name:  "poll",
	Short: "Poll existing vanadium projects",
	Long: `
Poll vanadium projects that can affect the outcome of the given tests
and report whether any new changes in these projects exist. If no
tests are specified, all projects are polled by default.
`,
	ArgsName: "<test ...>",
	ArgsLong: "<test ...> is a list of tests that determine what projects to poll.",
}

// runProjectPoll generates a description of changes that exist
// remotely but do not exist locally.
func runProjectPoll(command *cmdline.Command, args []string) error {
	projectSet := map[string]struct{}{}
	if len(args) > 0 {
		var config util.Config
		if err := util.LoadConfig("common", &config); err != nil {
			return err
		}
		// Compute a map from tests to projects that can change the
		// outcome of the test.
		testProjects := map[string][]string{}
		for _, project := range config.Projects() {
			for _, test := range config.ProjectTests([]string{project}) {
				testProjects[test] = append(testProjects[test], project)
			}
		}
		for _, arg := range args {
			projects, ok := testProjects[arg]
			if !ok {
				return fmt.Errorf("failed to find any projects for test %q", arg)
			}
			for _, project := range projects {
				projectSet[project] = struct{}{}
			}
		}
	}
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	update, err := util.PollProjects(ctx, manifestFlag, projectSet)
	if err != nil {
		return err
	}

	// Remove projects with empty changes.
	for project := range update {
		if changes := update[project]; len(changes) == 0 {
			delete(update, project)
		}
	}

	// Print update if it is not empty.
	if len(update) > 0 {
		bytes, err := json.MarshalIndent(update, "", "  ")
		if err != nil {
			return fmt.Errorf("MarshalIndent() failed: %v", err)
		}
		fmt.Fprintf(command.Stdout(), "%s\n", bytes)
	}
	return nil
}
