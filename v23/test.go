// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"strings"
	"time"

	"v.io/x/devtools/internal/testutil"
	"v.io/x/devtools/internal/tool"
	"v.io/x/lib/cmdline"
)

var (
	blessingsRootFlag string
	credDirFlag       string
	namespaceRootFlag string
	partFlag          int
	pkgsFlag          string
	reportFlag        bool
)

func init() {
	cmdTestRun.Flags.StringVar(&blessingsRootFlag, "blessings-root", "dev.v.io", "The blessings root.")
	cmdTestRun.Flags.StringVar(&credDirFlag, "v23.credentials", "", "Directory for vanadium credentials.")
	cmdTestRun.Flags.StringVar(&namespaceRootFlag, "v23.namespace.root", "/ns.dev.v.io:8101", "The namespace root.")
	cmdTestRun.Flags.IntVar(&partFlag, "part", -1, "Specify which part of the test to run.")
	cmdTestRun.Flags.StringVar(&pkgsFlag, "pkgs", "", "Comma-separated list of Go package expressions that identify a subset of tests to run; only relevant for Go-based tests")
	cmdTestRun.Flags.BoolVar(&reportFlag, "report", false, "Upload test report to Vanadium servers.")
}

// cmdTest represents the "v23 test" command.
var cmdTest = &cmdline.Command{
	Name:     "test",
	Short:    "Manage vanadium tests",
	Long:     "Manage vanadium tests.",
	Children: []*cmdline.Command{cmdTestGenerate, cmdTestProject, cmdTestRun, cmdTestList},
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
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
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
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
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
	if partFlag >= 0 {
		opt := testutil.PartOpt(partFlag)
		opts = append(opts, opt)
	}
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
	opts = append(opts, testutil.CredDirOpt(credDirFlag), testutil.BlessingsRootOpt(blessingsRootFlag), testutil.NamespaceRootOpt(namespaceRootFlag))
	return
}

func printSummary(ctx *tool.Context, results map[string]*testutil.TestResult) {
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
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:    &colorFlag,
		DryRun:   &dryRunFlag,
		Manifest: &manifestFlag,
		Verbose:  &verboseFlag,
	})
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
