// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $V23_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go .

package main

import (
	"fmt"
	"runtime"

	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	tool.InitializeRunFlags(&cmdRoot.Flags)
}

func main() {
	cmdline.Main(cmdRoot)
}

// cmdRoot represents the root of the v23 tool.
var cmdRoot = &cmdline.Command{
	Name:  "v23",
	Short: "Multi-purpose tool for Vanadium development",
	Long: `
Command v23 is a multi-purpose tool for Vanadium development.
`,
	LookPath: true,
	Children: []*cmdline.Command{
		cmdCL,
		cmdContributors,
		cmdOncall,
		cmdProject,
		cmdSnapshot,
		cmdUpdate,
		cmdVersion,
	},
}

// cmdVersion represents the "v23 version" command.
var cmdVersion = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runVersion),
	Name:   "version",
	Short:  "Print version",
	Long:   "Print version of the v23 tool.",
}

func runVersion(env *cmdline.Env, _ []string) error {
	fmt.Fprintf(env.Stdout, "v23 tool version %v\n", tool.Version)
	return nil
}
