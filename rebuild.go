// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"

	"v.io/jiri/collect"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
)

// cmdRebuild represents the "jiri rebuild" command.
var cmdRebuild = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runRebuild),
	Name:   "rebuild",
	Short:  "Rebuild the jiri command line tool",
	Long:   "Rebuild the jiri command line tool.",
}

// Implements cmdRebuild.  This function is like project.buildTools except it only
// builds the jiri project.  We also don't update anything before we do so.
func runRebuild(env *cmdline.Env, args []string) (e error) {
	ctx := tool.NewContextFromEnv(env)
	localProjects, err := project.LocalProjects(ctx)
	if err != nil {
		return err
	}

	// Create a temporary directory in which jiri will be built.
	tmpDir, err := ctx.Run().TempDir("", "tmp-jiri-rebuild")
	if err != nil {
		return fmt.Errorf("TempDir() failed: %v", err)
	}

	// Make sure we cleanup the temp directory.
	defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)

	// Paranoid sanity checking.
	lp, ok := localProjects[project.JiriProject]
	if !ok {
		return fmt.Errorf("jiri project (%s) not found", project.JiriProject)
	}

	// Build jiri.
	err = project.BuildTool(ctx, tmpDir, project.JiriName, project.JiriPackage, lp)
	if err != nil {
		return err
	}

	// Install jiri.
	return project.InstallTools(ctx, tmpDir)
}
