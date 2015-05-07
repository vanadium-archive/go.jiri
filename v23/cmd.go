// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"runtime"

	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

var (
	verboseFlag  bool
	dryRunFlag   bool
	colorFlag    bool
	manifestFlag string
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdRoot.Flags.BoolVar(&dryRunFlag, "n", false, "Show what commands will run but do not execute them.")
	cmdRoot.Flags.BoolVar(&colorFlag, "color", true, "Use color to format output.")
}

// root returns a command that represents the root of the v23 tool.
func root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the v23 tool.
var cmdRoot = &cmdline.Command{
	Name:  "v23",
	Short: "multi-purpose tool for Vanadium development",
	Long: `
Command v23 is a multi-purpose tool for Vanadium development.
`,
	Children: []*cmdline.Command{
		cmdApi,
		cmdBuildCop,
		cmdCL,
		cmdContributors,
		cmdCopyright,
		cmdEnv,
		cmdGo,
		cmdGoExt,
		cmdProfile,
		cmdProject,
		cmdRun,
		cmdSnapshot,
		cmdTest,
		cmdUpdate,
		cmdVersion,
	},
}

// cmdVersion represents the "v23 version" command.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the v23 tool.",
}

func runVersion(command *cmdline.Command, _ []string) error {
	fmt.Fprintf(command.Stdout(), "v23 tool version %v\n", tool.Version)
	return nil
}
