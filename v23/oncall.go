// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"time"

	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

// cmdOncall represents the "v23 oncall" command.
var cmdOncall = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runOncall),
	Name:   "oncall",
	Short:  "Manage vanadium oncall schedule",
	Long: `
Manage vanadium oncall schedule. If no subcommand is given, it shows the LDAP
of the current oncall.
`,
	Children: []*cmdline.Command{cmdOncallList},
}

// cmdOncallList represents the "v23 oncall list" command.
var cmdOncallList = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runOncallList),
	Name:   "list",
	Short:  "List available oncall schedule",
	Long:   "List available oncall schedule.",
}

func runOncall(env *cmdline.Env, _ []string) error {
	ctx := tool.NewContextFromEnv(env)
	shift, err := util.Oncall(ctx, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout(), "%s,%s\n", shift.Primary, shift.Secondary)
	return nil
}

func runOncallList(env *cmdline.Env, _ []string) error {
	ctx := tool.NewContextFromEnv(env)
	rotation, err := util.LoadOncallRotation(ctx)
	if err != nil {
		return err
	}
	// Print the schedule with the current oncall marked.
	layout := "Jan 2, 2006 3:04:05 PM"
	now := time.Now().Unix()
	foundOncall := false
	for i, shift := range rotation.Shifts {
		prefix := "   "
		if !foundOncall && i < len(rotation.Shifts)-1 {
			nextDate := rotation.Shifts[i+1].Date
			nextTimestamp, err := time.Parse(layout, nextDate)
			if err != nil {
				fmt.Fprintf(ctx.Stderr(), "Parse(%q, %v) failed: %v", layout, nextDate, err)
				continue
			}
			if now < nextTimestamp.Unix() {
				prefix = "-> "
				foundOncall = true
			}
		}
		if i == len(rotation.Shifts)-1 && !foundOncall {
			prefix = "-> "
		}
		fmt.Fprintf(ctx.Stdout(), "%s%25s: %s\n", prefix, shift.Date, shift.Primary)
	}
	return nil
}
