// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"v.io/jiri"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/jiri/util"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/set"
)

var (
	branchesFlag        bool
	cleanupBranchesFlag bool
	noPristineFlag      bool
	checkDirtyFlag      bool
	showNameFlag        bool
	formatFlag          string
)

func init() {
	cmdProjectClean.Flags.BoolVar(&cleanupBranchesFlag, "branches", false, "Delete all non-master branches.")
	cmdProjectList.Flags.BoolVar(&branchesFlag, "branches", false, "Show project branches.")
	cmdProjectList.Flags.BoolVar(&noPristineFlag, "nopristine", false, "If true, omit pristine projects, i.e. projects with a clean master branch and no other branches.")
	cmdProjectShellPrompt.Flags.BoolVar(&checkDirtyFlag, "check-dirty", true, "If false, don't check for uncommitted changes or untracked files. Setting this option to false is dangerous: dirty master branches will not appear in the output.")
	cmdProjectShellPrompt.Flags.BoolVar(&showNameFlag, "show-name", false, "Show the name of the current repo.")
	cmdProjectInfo.Flags.StringVar(&formatFlag, "f", "{{.Project.Name}}", "The go template for the fields to display.")
	tool.InitializeProjectFlags(&cmdProjectPoll.Flags)

}

// cmdProject represents the "jiri project" command.
var cmdProject = &cmdline.Command{
	Name:     "project",
	Short:    "Manage the jiri projects",
	Long:     "Manage the jiri projects.",
	Children: []*cmdline.Command{cmdProjectClean, cmdProjectInfo, cmdProjectList, cmdProjectShellPrompt, cmdProjectPoll},
}

// cmdProjectClean represents the "jiri project clean" command.
var cmdProjectClean = &cmdline.Command{
	Runner:   jiri.RunnerFunc(runProjectClean),
	Name:     "clean",
	Short:    "Restore jiri projects to their pristine state",
	Long:     "Restore jiri projects back to their master branches and get rid of all the local branches and changes.",
	ArgsName: "<project ...>",
	ArgsLong: "<project ...> is a list of projects to clean up.",
}

func runProjectClean(jirix *jiri.X, args []string) (e error) {
	localProjects, err := project.LocalProjects(jirix, project.FullScan)
	if err != nil {
		return err
	}
	var projects project.Projects
	if len(args) > 0 {
		for _, arg := range args {
			p, err := localProjects.FindUnique(arg)
			if err != nil {
				fmt.Fprintf(jirix.Stderr(), "Error finding local project %q: %v.\n", p.Name, err)
			} else {
				projects[p.Key()] = p
			}
		}
	} else {
		projects = localProjects
	}
	if err := project.CleanupProjects(jirix, projects, cleanupBranchesFlag); err != nil {
		return err
	}
	return nil
}

// cmdProjectList represents the "jiri project list" command.
var cmdProjectList = &cmdline.Command{
	Runner: jiri.RunnerFunc(runProjectList),
	Name:   "list",
	Short:  "List existing jiri projects and branches",
	Long:   "Inspect the local filesystem and list the existing projects and branches.",
}

// runProjectList generates a listing of local projects.
func runProjectList(jirix *jiri.X, _ []string) error {
	states, err := project.GetProjectStates(jirix, noPristineFlag)
	if err != nil {
		return err
	}
	var keys project.ProjectKeys
	for key := range states {
		keys = append(keys, key)
	}
	sort.Sort(keys)

	for _, key := range keys {
		state := states[key]
		if noPristineFlag {
			pristine := len(state.Branches) == 1 && state.CurrentBranch == "master" && !state.HasUncommitted && !state.HasUntracked
			if pristine {
				continue
			}
		}
		fmt.Fprintf(jirix.Stdout(), "name=%q remote=%q path=%q\n", state.Project.Name, state.Project.Remote, state.Project.Path)
		if branchesFlag {
			for _, branch := range state.Branches {
				s := "  "
				if branch.Name == state.CurrentBranch {
					s += "* "
				}
				s += branch.Name
				if branch.HasGerritMessage {
					s += " (exported to gerrit)"
				}
				fmt.Fprintf(jirix.Stdout(), "%v\n", s)
			}
		}
	}
	return nil
}

// cmdProjectInfo represents the "jiri project info" command.
var cmdProjectInfo = &cmdline.Command{
	Runner: jiri.RunnerFunc(runProjectInfo),
	Name:   "info",
	Short:  "Provided structured input for existing jiri projects and branches",
	Long: `
Inspect the local filesystem and provide structured info on the existing projects
and branches. Projects are specified using regular expressions that are matched
against project keys. If no command line arguments are provided the project
that the contains the current directory is used. The information to be
displayed is specified using a go template, supplied via the -f flag, that is
executed against the v.io/jiri/project.ProjectState structure. This structure
currently has the following fields: ` + fmt.Sprintf("%#v", project.ProjectState{}),
	ArgsName: "<project-keys>...",
	ArgsLong: "<project-keys>... a list of project keys, as regexps, to apply the specified format to",
}

// runProjectInfo provides structured info on local projects.
func runProjectInfo(jirix *jiri.X, args []string) error {
	tmpl, err := template.New("info").Parse(formatFlag)
	if err != nil {
		return fmt.Errorf("failed to parse template %q: %v", formatFlag, err)
	}
	regexps := []*regexp.Regexp{}

	if len(args) > 0 {
		regexps = make([]*regexp.Regexp, len(args), len(args))
		for i, a := range args {
			re, err := regexp.Compile(a)
			if err != nil {
				return fmt.Errorf("failed to compile regexp %v: %v", a, err)
			}
			regexps[i] = re
		}
	}

	dirty := false
	for _, slow := range []string{"HasUncommitted", "HasUntracked"} {
		if strings.Contains(formatFlag, slow) {
			dirty = true
			break
		}
	}

	var states map[project.ProjectKey]*project.ProjectState
	var keys project.ProjectKeys
	if len(args) == 0 {
		currentProjectKey, err := project.CurrentProjectKey(jirix)
		if err != nil {
			return err
		}
		state, err := project.GetProjectState(jirix, currentProjectKey, true)
		if err != nil {
			return err
		}
		states = map[project.ProjectKey]*project.ProjectState{
			currentProjectKey: state,
		}
		keys = append(keys, currentProjectKey)
	} else {
		var err error
		states, err = project.GetProjectStates(jirix, dirty)
		if err != nil {
			return err
		}
		for key := range states {
			for _, re := range regexps {
				if re.MatchString(string(key)) {
					keys = append(keys, key)
					break
				}
			}
		}
	}
	sort.Sort(keys)

	for _, key := range keys {
		state := states[key]
		out := &bytes.Buffer{}
		if err = tmpl.Execute(out, state); err != nil {
			return jirix.UsageErrorf("invalid format")
		}
		fmt.Fprintln(jirix.Stdout(), out.String())
	}
	return nil
}

// cmdProjectShellPrompt represents the "jiri project shell-prompt" command.
var cmdProjectShellPrompt = &cmdline.Command{
	Runner: jiri.RunnerFunc(runProjectShellPrompt),
	Name:   "shell-prompt",
	Short:  "Print a succinct status of projects suitable for shell prompts",
	Long: `
Reports current branches of jiri projects (repositories) as well as an
indication of each project's status:
  *  indicates that a repository contains uncommitted changes
  %  indicates that a repository contains untracked files
`,
}

func runProjectShellPrompt(jirix *jiri.X, args []string) error {
	states, err := project.GetProjectStates(jirix, checkDirtyFlag)
	if err != nil {
		return err
	}
	var keys project.ProjectKeys
	for key := range states {
		keys = append(keys, key)
	}
	sort.Sort(keys)

	// Get the key of the current project.
	currentProjectKey, err := project.CurrentProjectKey(jirix)
	if err != nil {
		return err
	}
	var statuses []string
	for _, key := range keys {
		state := states[key]
		status := ""
		if checkDirtyFlag {
			if state.HasUncommitted {
				status += "*"
			}
			if state.HasUntracked {
				status += "%"
			}
		}
		short := state.CurrentBranch + status
		long := filepath.Base(states[key].Project.Name) + ":" + short
		if key == currentProjectKey {
			if showNameFlag {
				statuses = append([]string{long}, statuses...)
			} else {
				statuses = append([]string{short}, statuses...)
			}
		} else {
			pristine := state.CurrentBranch == "master"
			if checkDirtyFlag {
				pristine = pristine && !state.HasUncommitted && !state.HasUntracked
			}
			if !pristine {
				statuses = append(statuses, long)
			}
		}
	}
	fmt.Println(strings.Join(statuses, ","))
	return nil
}

// cmdProjectPoll represents the "jiri project poll" command.
var cmdProjectPoll = &cmdline.Command{
	Runner: jiri.RunnerFunc(runProjectPoll),
	Name:   "poll",
	Short:  "Poll existing jiri projects",
	Long: `
Poll jiri projects that can affect the outcome of the given tests
and report whether any new changes in these projects exist. If no
tests are specified, all projects are polled by default.
`,
	ArgsName: "<test ...>",
	ArgsLong: "<test ...> is a list of tests that determine what projects to poll.",
}

// runProjectPoll generates a description of changes that exist
// remotely but do not exist locally.
func runProjectPoll(jirix *jiri.X, args []string) error {
	projectSet := map[string]struct{}{}
	if len(args) > 0 {
		config, err := util.LoadConfig(jirix)
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
	update, err := project.PollProjects(jirix, projectSet)
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
		fmt.Fprintf(jirix.Stdout(), "%s\n", bytes)
	}
	return nil
}
