// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"v.io/x/devtools/internal/project"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/set"
)

var (
	branchesFlag        bool
	cleanupBranchesFlag bool
	noPristineFlag      bool
	checkDirtyFlag      bool
	showNameFlag        bool
)

func init() {
	cmdProjectClean.Flags.BoolVar(&cleanupBranchesFlag, "branches", false, "Delete all non-master branches.")
	cmdProjectList.Flags.BoolVar(&branchesFlag, "branches", false, "Show project branches.")
	cmdProjectList.Flags.BoolVar(&noPristineFlag, "nopristine", false, "If true, omit pristine projects, i.e. projects with a clean master branch and no other branches.")
	cmdProjectShellPrompt.Flags.BoolVar(&checkDirtyFlag, "check-dirty", true, "If false, don't check for uncommitted changes or untracked files. Setting this option to false is dangerous: dirty master branches will not appear in the output.")
	cmdProjectShellPrompt.Flags.BoolVar(&showNameFlag, "show-name", false, "Show the name of the current repo.")

	tool.InitializeProjectFlags(&cmdProjectPoll.Flags)

}

// cmdProject represents the "v23 project" command.
var cmdProject = &cmdline.Command{
	Name:     "project",
	Short:    "Manage the vanadium projects",
	Long:     "Manage the vanadium projects.",
	Children: []*cmdline.Command{cmdProjectClean, cmdProjectList, cmdProjectShellPrompt, cmdProjectPoll},
}

// cmdProjectClean represents the "v23 project clean" command.
var cmdProjectClean = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runProjectClean),
	Name:     "clean",
	Short:    "Restore vanadium projects to their pristine state",
	Long:     "Restore vanadium projects back to their master branches and get rid of all the local branches and changes.",
	ArgsName: "<project ...>",
	ArgsLong: "<project ...> is a list of projects to clean up.",
}

func runProjectClean(env *cmdline.Env, args []string) (e error) {
	ctx := tool.NewContextFromEnv(env)
	localProjects, err := project.LocalProjects(ctx)
	if err != nil {
		return err
	}
	projects := map[string]project.Project{}
	if len(args) > 0 {
		for _, arg := range args {
			if p, ok := localProjects[arg]; ok {
				projects[p.Name] = p
			} else {
				fmt.Fprintf(ctx.Stderr(), "Local project %q not found.\n", p.Name)
			}
		}
	} else {
		projects = localProjects
	}
	if err := project.CleanupProjects(ctx, projects, cleanupBranchesFlag); err != nil {
		return err
	}
	return nil
}

// cmdProjectList represents the "v23 project list" command.
var cmdProjectList = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runProjectList),
	Name:   "list",
	Short:  "List existing vanadium projects and branches",
	Long:   "Inspect the local filesystem and list the existing projects and branches.",
}

type branchState struct {
	name             string
	hasGerritMessage bool
}

type projectState struct {
	project        project.Project
	branches       []branchState
	currentBranch  string
	hasUncommitted bool
	hasUntracked   bool
}

func setProjectState(ctx *tool.Context, state *projectState, checkDirty bool, ch chan<- error) {
	var err error
	switch state.project.Protocol {
	case "git":
		scm := ctx.Git(tool.RootDirOpt(state.project.Path))
		var branches []string
		branches, state.currentBranch, err = scm.GetBranches()
		if err != nil {
			ch <- err
			return
		}
		for _, branch := range branches {
			file := filepath.Join(state.project.Path, project.MetadataDirName(), branch, commitMessageFileName)
			hasFile := true
			if _, err := ctx.Run().Stat(file); err != nil {
				if !os.IsNotExist(err) {
					ch <- err
					return
				}
				hasFile = false
			}
			state.branches = append(state.branches, branchState{
				name:             branch,
				hasGerritMessage: hasFile,
			})
		}
		if checkDirty {
			state.hasUncommitted, err = scm.HasUncommittedChanges()
			if err != nil {
				ch <- err
				return
			}
			state.hasUntracked, err = scm.HasUntrackedFiles()
			if err != nil {
				ch <- err
				return
			}
		}
	default:
		ch <- project.UnsupportedProtocolErr(state.project.Protocol)
		return
	}
	ch <- nil
}

func getProjectStates(ctx *tool.Context, checkDirty bool) (map[string]*projectState, error) {
	projects, err := project.LocalProjects(ctx)
	if err != nil {
		return nil, err
	}
	states := make(map[string]*projectState, len(projects))
	sem := make(chan error, len(projects))
	for name, project := range projects {
		state := &projectState{
			project: project,
		}
		states[name] = state
		// ctx is not threadsafe, so we make a clone for each
		// goroutine.
		go setProjectState(ctx.Clone(tool.ContextOpts{}), state, checkDirty, sem)
	}
	for _ = range projects {
		err := <-sem
		if err != nil {
			return nil, err
		}
	}
	return states, nil
}

// runProjectList generates a listing of local projects.
func runProjectList(env *cmdline.Env, _ []string) error {
	ctx := tool.NewContextFromEnv(env)
	states, err := getProjectStates(ctx, noPristineFlag)
	if err != nil {
		return err
	}
	names := []string{}
	for name := range states {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		state := states[name]
		if noPristineFlag {
			pristine := len(state.branches) == 1 && state.currentBranch == "master" && !state.hasUncommitted && !state.hasUntracked
			if pristine {
				continue
			}
		}
		fmt.Fprintf(ctx.Stdout(), "project=%q path=%q\n", path.Base(name), state.project.Path)
		if branchesFlag {
			for _, branch := range state.branches {
				s := "  "
				if branch.name == state.currentBranch {
					s += "* "
				}
				s += branch.name
				if branch.hasGerritMessage {
					s += " (exported to gerrit)"
				}
				fmt.Fprintf(ctx.Stdout(), "%v\n", s)
			}
		}
	}
	return nil
}

// cmdProjectShellPrompt represents the "v23 project shell-prompt" command.
var cmdProjectShellPrompt = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runProjectShellPrompt),
	Name:   "shell-prompt",
	Short:  "Print a succinct status of projects, suitable for shell prompts",
	Long: `
Reports current branches of vanadium projects (repositories) as well as an
indication of each project's status:
  *  indicates that a repository contains uncommitted changes
  %  indicates that a repository contains untracked files
`,
}

func runProjectShellPrompt(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env)

	states, err := getProjectStates(ctx, checkDirtyFlag)
	if err != nil {
		return err
	}
	names := []string{}
	for name := range states {
		names = append(names, name)
	}
	sort.Strings(names)

	// Get the name of the current project.
	currentProjectName, err := project.CurrentProjectName(ctx)
	if err != nil {
		return err
	}
	var statuses []string
	for _, name := range names {
		state := states[name]
		status := ""
		if checkDirtyFlag {
			if state.hasUncommitted {
				status += "*"
			}
			if state.hasUntracked {
				status += "%"
			}
		}
		short := state.currentBranch + status
		long := filepath.Base(name) + ":" + short
		if name == currentProjectName {
			if showNameFlag {
				statuses = append([]string{long}, statuses...)
			} else {
				statuses = append([]string{short}, statuses...)
			}
		} else {
			pristine := state.currentBranch == "master"
			if checkDirtyFlag {
				pristine = pristine && !state.hasUncommitted && !state.hasUntracked
			}
			if !pristine {
				statuses = append(statuses, long)
			}
		}
	}
	fmt.Println(strings.Join(statuses, ","))
	return nil
}

// cmdProjectPoll represents the "v23 project poll" command.
var cmdProjectPoll = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runProjectPoll),
	Name:   "poll",
	Short:  "Poll existing vanadium projects",
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
func runProjectPoll(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env)
	projectSet := map[string]struct{}{}
	if len(args) > 0 {
		config, err := util.LoadConfig(ctx)
		if err != nil {
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
			set.String.Union(projectSet, set.String.FromSlice(projects))
		}
	}
	update, err := project.PollProjects(ctx, projectSet)
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
		fmt.Fprintf(env.Stdout, "%s\n", bytes)
	}
	return nil
}
