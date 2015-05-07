// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os/exec"
	"syscall"

	"v.io/x/devtools/internal/envutil"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

// translateExitCode translates errors from the "os/exec" package that contain
// exit codes into cmdline.ErrExitCode errors.
func translateExitCode(err error) error {
	if exit, ok := err.(*exec.ExitError); ok {
		if wait, ok := exit.Sys().(syscall.WaitStatus); ok {
			if status := wait.ExitStatus(); wait.Exited() && status != 0 {
				return cmdline.ErrExitCode(status)
			}
		}
	}
	return err
}

// cmdEnv represents the "v23 env" command.
var cmdEnv = &cmdline.Command{
	Run:   runEnv,
	Name:  "env",
	Short: "Print vanadium environment variables",
	Long: `
Print vanadium environment variables.

If no arguments are given, prints all variables in NAME="VALUE" format,
each on a separate line ordered by name.  This format makes it easy to set
all vars by running the following bash command (or similar for other shells):
   eval $(v23 env)

If arguments are given, prints only the value of each named variable,
each on a separate line in the same order as the arguments.
`,
	ArgsName: "[name ...]",
	ArgsLong: "[name ...] is an optional list of variable names.",
}

func runEnv(command *cmdline.Command, args []string) error {
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	env, err := util.VanadiumEnvironment(ctx)
	if err != nil {
		return err
	}
	if len(args) > 0 {
		for _, name := range args {
			fmt.Fprintln(command.Stdout(), env.Get(name))
		}
		return nil
	}
	for _, entry := range envutil.ToQuotedSlice(env.DeltaMap()) {
		fmt.Fprintln(command.Stdout(), entry)
	}
	return nil
}

// cmdRun represents the "v23 run" command.
var cmdRun = &cmdline.Command{
	Run:      runRun,
	Name:     "run",
	Short:    "Run an executable using the vanadium environment",
	Long:     "Run an executable using the vanadium environment.",
	ArgsName: "<executable> [arg ...]",
	ArgsLong: `
<executable> [arg ...] is the executable to run and any arguments to pass
verbatim to the executable.
`,
}

func runRun(command *cmdline.Command, args []string) error {
	if len(args) == 0 {
		return command.UsageErrorf("no command to run")
	}
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	env, err := util.VanadiumEnvironment(ctx)
	if err != nil {
		return err
	}
	// For certain commands, vanadium uses specialized wrappers that do
	// more than just set up the vanadium environment. If the user is
	// trying to run any of these commands using the 'run' command,
	// warn the user that they might want to use the specialized wrapper.
	switch args[0] {
	case "go":
		fmt.Fprintln(command.Stderr(), `WARNING: using "v23 run go" instead of "v23 go" skips vdl generation`)
	}
	execCmd := exec.Command(args[0], args[1:]...)
	execCmd.Stdout = command.Stdout()
	execCmd.Stderr = command.Stderr()
	execCmd.Env = env.Slice()
	return translateExitCode(execCmd.Run())
}
