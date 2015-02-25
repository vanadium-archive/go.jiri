package main

import (
	"fmt"
	"strings"
	"time"

	"v.io/lib/cmdline"
	"v.io/tools/lib/testutil"
	"v.io/tools/lib/util"
)

// cmdTest represents the "v23 test" command.
var cmdTest = &cmdline.Command{
	Name:     "test",
	Short:    "Manage vanadium tests",
	Long:     "Manage vanadium tests.",
	Children: []*cmdline.Command{cmdTestProject, cmdTestRun, cmdTestList, cmdV23Generate},
}

// cmdTestProject represents the "v23 test project" command.
var cmdTestProject = &cmdline.Command{
	Run:   runTestProject,
	Name:  "project",
	Short: "Run tests for a vanadium project",
	Long: `
Runs tests for a vanadium project that is by the remote URL specified as
the command-line argument. Projects hosted on googlesource.com, can be
specified using the basename of the URL (e.g. "vanadium.go.core" implies
"https://vanadium.googlesource.com/vanadium.go.core").
`,
	ArgsName: "<project>",
	ArgsLong: "<project> identifies the project for which to run tests.",
}

func runTestProject(command *cmdline.Command, args []string) error {
	if len(args) != 1 {
		return command.UsageErrorf("unexpected number of arguments")
	}
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	project := args[0]
	results, err := testutil.RunProjectTests(ctx, nil, []string{project}, optsFromFlags()...)
	if err != nil {
		return err
	}
	printSummary(ctx, results)
	for _, result := range results {
		if result.Status != testutil.TestPassed {
			return cmdline.ErrExitCode(2)
		}
	}
	return nil
}

// cmdTestRun represents the "v23 test run" command.
var cmdTestRun = &cmdline.Command{
	Run:      runTestRun,
	Name:     "run",
	Short:    "Run vanadium tests",
	Long:     "Run vanadium tests.",
	ArgsName: "<name...>",
	ArgsLong: "<name...> is a list names identifying the tests to run.",
}

func runTestRun(command *cmdline.Command, args []string) error {
	if len(args) == 0 {
		return command.UsageErrorf("unexpected number of arguments")
	}
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	results, err := testutil.RunTests(ctx, nil, args, optsFromFlags()...)
	if err != nil {
		return err
	}
	printSummary(ctx, results)
	for _, result := range results {
		if result.Status != testutil.TestPassed {
			return cmdline.ErrExitCode(2)
		}
	}
	return nil
}

func optsFromFlags() (opts []testutil.TestOpt) {
	if reportFlag {
		opt := testutil.PrefixOpt(time.Now().Format(time.RFC3339))
		opts = append(opts, opt)
	}
	pkgs := []string{}
	for _, pkg := range strings.Split(pkgsFlag, ",") {
		if len(pkg) > 0 {
			pkgs = append(pkgs, pkg)
		}
	}
	opts = append(opts, testutil.PkgsOpt(pkgs))
	return
}

func printSummary(ctx *util.Context, results map[string]*testutil.TestResult) {
	fmt.Fprintf(ctx.Stdout(), "SUMMARY:\n")
	for name, result := range results {
		fmt.Fprintf(ctx.Stdout(), "%v %s\n", name, result.Status)
		if len(result.ExcludedTests) > 0 {
			for pkg, tests := range result.ExcludedTests {
				fmt.Fprintf(ctx.Stdout(), "  excluded %d tests from packge %v: %v\n", len(tests), pkg, tests)
			}
		}
		if len(result.SkippedTests) > 0 {
			for pkg, tests := range result.SkippedTests {
				fmt.Fprintf(ctx.Stdout(), "  skipped %d tests from pacakge %v: %v\n", len(tests), pkg, tests)
			}
		}
	}
}

// cmdTestList represents the "v23 test list" command.
var cmdTestList = &cmdline.Command{
	Run:   runTestList,
	Name:  "list",
	Short: "List vanadium tests",
	Long:  "List vanadium tests.",
}

func runTestList(command *cmdline.Command, _ []string) error {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	testList, err := testutil.TestList()
	if err != nil {
		fmt.Fprintf(ctx.Stderr(), "%v\n", err)
		return err
	}
	for _, test := range testList {
		fmt.Fprintf(ctx.Stdout(), "%v\n", test)
	}
	return nil
}
