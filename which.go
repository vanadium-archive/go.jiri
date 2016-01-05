// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"v.io/x/lib/cmdline"
)

var cmdWhich = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runWhich),
	Name:   "which",
	Short:  "Show path to the jiri tool",
	Long: `
Which behaves similarly to the unix commandline tool.  It is useful in
determining whether the jiri binary is being run directly, or run via the jiri
shim script.

If the binary is being run directly, the output looks like this:

  # binary
  /path/to/binary/jiri

If the script is being run, the output looks like this:

  # script
  /path/to/script/jiri
`,
}

func runWhich(env *cmdline.Env, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(env.Stdout, "# binary")
		path, err := exec.LookPath(os.Args[0])
		if err != nil {
			return err
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		fmt.Fprintln(env.Stdout, abs)
		return nil
	}
	// TODO(toddw): Look up the path to each argument.  This will only be helpful
	// after the profiles are moved back into the main jiri tool.
	return fmt.Errorf("unexpected arguments")
}
