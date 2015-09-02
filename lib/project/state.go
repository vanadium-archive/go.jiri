// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
	"os"
	"path/filepath"

	"v.io/jiri/lib/tool"
)

type BranchState struct {
	HasGerritMessage bool
	Name             string
}

type ProjectState struct {
	Branches       []BranchState
	CurrentBranch  string
	HasUncommitted bool
	HasUntracked   bool
	Project        Project
}

func setProjectState(ctx *tool.Context, state *ProjectState, checkDirty bool, ch chan<- error) {
	var err error
	switch state.Project.Protocol {
	case "git":
		scm := ctx.Git(tool.RootDirOpt(state.Project.Path))
		var branches []string
		branches, state.CurrentBranch, err = scm.GetBranches()
		if err != nil {
			ch <- err
			return
		}
		for _, branch := range branches {
			file := filepath.Join(state.Project.Path, MetadataDirName(), branch, ".gerrit_commit_message")
			hasFile := true
			if _, err := ctx.Run().Stat(file); err != nil {
				if !os.IsNotExist(err) {
					ch <- err
					return
				}
				hasFile = false
			}
			state.Branches = append(state.Branches, BranchState{
				Name:             branch,
				HasGerritMessage: hasFile,
			})
		}
		if checkDirty {
			state.HasUncommitted, err = scm.HasUncommittedChanges()
			if err != nil {
				ch <- err
				return
			}
			state.HasUntracked, err = scm.HasUntrackedFiles()
			if err != nil {
				ch <- err
				return
			}
		}
	default:
		ch <- UnsupportedProtocolErr(state.Project.Protocol)
		return
	}
	ch <- nil
}

func GetProjectStates(ctx *tool.Context, checkDirty bool) (map[string]*ProjectState, error) {
	projects, err := LocalProjects(ctx)
	if err != nil {
		return nil, err
	}
	states := make(map[string]*ProjectState, len(projects))
	sem := make(chan error, len(projects))
	for name, project := range projects {
		state := &ProjectState{
			Project: project,
		}
		states[name] = state
		// ctx is not threadsafe, so we make a clone for each goroutine.
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
