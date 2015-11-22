// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"path/filepath"
	"time"

	"v.io/jiri/jiri"
	"v.io/jiri/project"
	"v.io/jiri/retry"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
)

var (
	gcFlag       bool
	attemptsFlag int
)

func init() {
	tool.InitializeProjectFlags(&cmdUpdate.Flags)

	cmdUpdate.Flags.BoolVar(&gcFlag, "gc", false, "Garbage collect obsolete repositories.")
	cmdUpdate.Flags.IntVar(&attemptsFlag, "attempts", 1, "Number of attempts before failing.")
}

// cmdUpdate represents the "jiri update" command.
var cmdUpdate = &cmdline.Command{
	Runner: jiri.RunnerFunc(runUpdate),
	Name:   "update",
	Short:  "Update all jiri tools and projects",
	Long: `
Updates all projects, builds the latest version of all tools, and installs the
resulting binaries into $JIRI_ROOT/devtools/bin. The sequence in which the
individual updates happen guarantees that we end up with a consistent set of
tools and source code. The set of projects and tools to update is described in
the manifest.

Run "jiri help manifest" for details on manifests.
`,
}

func runUpdate(jirix *jiri.X, _ []string) error {
	// Create a snapshot of the current state of all projects and
	// write it to the $JIRI_ROOT/.update_history folder.
	snapshotFile := filepath.Join(jirix.Root, ".update_history", time.Now().Format(time.RFC3339))
	if err := project.CreateSnapshot(jirix, snapshotFile); err != nil {
		return err
	}

	// Update all projects to their latest version.
	// Attempt <attemptsFlag> times before failing.
	updateFn := func() error {
		return project.UpdateUniverse(jirix, gcFlag)
	}
	return retry.Function(jirix.Context, updateFn, retry.AttemptsOpt(attemptsFlag))
}
