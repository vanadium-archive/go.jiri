package main

import (
	"fmt"
	"time"

	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

// cmdBuildCop represents the "v23 buildcop" command.
var cmdBuildCop = &cmdline.Command{
	Run:   runBuildCop,
	Name:  "buildcop",
	Short: "Manage vanadium build cop schedule",
	Long: `
Manage vanadium build cop schedule. If no subcommand is given, it shows the LDAP
of the current build cop.
`,
	Children: []*cmdline.Command{cmdBuildCopList},
}

// cmdBuildCopList represents the "v23 buildcop list" command.
var cmdBuildCopList = &cmdline.Command{
	Run:   runBuildCopList,
	Name:  "list",
	Short: "List available build cop schedule",
	Long:  "List available build cop schedule.",
}

func runBuildCop(command *cmdline.Command, _ []string) error {
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	buildcop, err := util.BuildCop(ctx, time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(ctx.Stdout(), "%s\n", buildcop)
	return nil
}

func runBuildCopList(command *cmdline.Command, _ []string) error {
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	rotation, err := util.LoadBuildCopRotation(ctx)
	if err != nil {
		return err
	}
	// Print the schedule with the current build cop marked.
	layout := "Jan 2, 2006 3:04:05 PM"
	now := time.Now().Unix()
	foundBuildCop := false
	for i, shift := range rotation.Shifts {
		prefix := "   "
		if !foundBuildCop && i < len(rotation.Shifts)-1 {
			nextDate := rotation.Shifts[i+1].Date
			nextTimestamp, err := time.Parse(layout, nextDate)
			if err != nil {
				fmt.Fprintf(ctx.Stderr(), "Parse(%q, %v) failed: %v", layout, nextDate, err)
				continue
			}
			if now < nextTimestamp.Unix() {
				prefix = "-> "
				foundBuildCop = true
			}
		}
		if i == len(rotation.Shifts)-1 && !foundBuildCop {
			prefix = "-> "
		}
		fmt.Fprintf(ctx.Stdout(), "%s%25s: %s\n", prefix, shift.Date, shift.Primary)
	}
	return nil
}
