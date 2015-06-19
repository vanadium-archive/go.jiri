// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"runtime"
	"strings"

	"v.io/x/devtools/internal/test"
	"v.io/x/devtools/internal/tool"
	v23test "v.io/x/devtools/v23/internal/test"
	"v.io/x/lib/cmdline"
)

var (
	adminCredDirFlag     string
	blessingsRootFlag    string
	cleanGoFlag          bool
	namespaceRootFlag    string
	numWorkersFlag       int
	outputDirFlag        string
	partFlag             int
	pkgsFlag             string
	publisherCredDirFlag string
)

func init() {
	cmdTestRun.Flags.StringVar(&blessingsRootFlag, "blessings-root", "dev.v.io", "The blessings root.")
	cmdTestRun.Flags.StringVar(&adminCredDirFlag, "v23.credentials.admin", "", "Directory for vanadium credentials.")
	cmdTestRun.Flags.StringVar(&namespaceRootFlag, "v23.namespace.root", "/ns.dev.v.io:8101", "The namespace root.")
	cmdTestRun.Flags.StringVar(&publisherCredDirFlag, "v23.credentials.publisher", "", "Directory for vanadium credentials for publishing new binaries.")
	cmdTestRun.Flags.IntVar(&numWorkersFlag, "num-test-workers", runtime.NumCPU(), "Set the number of test workers to use; use 1 to serialize all tests.")
	cmdTestRun.Flags.Lookup("num-test-workers").DefValue = "<runtime.NumCPU()>"
	cmdTestRun.Flags.StringVar(&outputDirFlag, "output-dir", "", "Directory to output test results into.")
	cmdTestRun.Flags.IntVar(&partFlag, "part", -1, "Specify which part of the test to run.")
	cmdTestRun.Flags.StringVar(&pkgsFlag, "pkgs", "", "Comma-separated list of Go package expressions that identify a subset of tests to run; only relevant for Go-based tests")
	cmdTestRun.Flags.BoolVar(&cleanGoFlag, "clean-go", true, "Specify whether to remove Go object files and binaries before running the tests. Setting this flag to 'false' may lead to faster Go builds, but it may also result in some source code changes not being reflected in the tests (e.g., if the change was made in a different Go workspace).")
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
	Runner: cmdline.RunnerFunc(runTestProject),
	Name:   "project",
	Short:  "Run tests for a vanadium project",
	Long: `
Runs tests for a vanadium project that is by the remote URL specified as
the command-line argument. Projects hosted on googlesource.com, can be
specified using the basename of the URL (e.g. "vanadium.go.core" implies
"https://vanadium.googlesource.com/vanadium.go.core").
`,
	ArgsName: "<project>",
	ArgsLong: "<project> identifies the project for which to run tests.",
}

func runTestProject(env *cmdline.Env, args []string) error {
	if len(args) != 1 {
		return env.UsageErrorf("unexpected number of arguments")
	}
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	project := args[0]
	results, err := v23test.RunProjectTests(ctx, nil, []string{project}, optsFromFlags()...)
	if err != nil {
		return err
	}
	printSummary(ctx, results)
	for _, result := range results {
		if result.Status != test.Passed {
			return cmdline.ErrExitCode(test.FailedExitCode)
		}
	}
	return nil
}

// cmdTestRun represents the "v23 test run" command.
var cmdTestRun = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runTestRun),
	Name:     "run",
	Short:    "Run vanadium tests",
	Long:     "Run vanadium tests.",
	ArgsName: "<name...>",
	ArgsLong: "<name...> is a list names identifying the tests to run.",
}

func runTestRun(env *cmdline.Env, args []string) error {
	if len(args) == 0 {
		return env.UsageErrorf("unexpected number of arguments")
	}
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:   &colorFlag,
		DryRun:  &dryRunFlag,
		Verbose: &verboseFlag,
	})
	results, err := v23test.RunTests(ctx, nil, args, optsFromFlags()...)
	if err != nil {
		return err
	}
	printSummary(ctx, results)
	for _, result := range results {
		if result.Status != test.Passed {
			return cmdline.ErrExitCode(test.FailedExitCode)
		}
	}
	return nil
}

func optsFromFlags() (opts []v23test.Opt) {
	if partFlag >= 0 {
		opt := v23test.PartOpt(partFlag)
		opts = append(opts, opt)
	}
	pkgs := []string{}
	for _, pkg := range strings.Split(pkgsFlag, ",") {
		if len(pkg) > 0 {
			pkgs = append(pkgs, pkg)
		}
	}
	opts = append(opts, v23test.PkgsOpt(pkgs))
	opts = append(opts,
		v23test.BlessingsRootOpt(blessingsRootFlag),
		v23test.AdminCredDirOpt(adminCredDirFlag),
		v23test.NamespaceRootOpt(namespaceRootFlag),
		v23test.NumWorkersOpt(numWorkersFlag),
		v23test.OutputDirOpt(outputDirFlag),
		v23test.PublisherCredDirOpt(publisherCredDirFlag),
		v23test.CleanGoOpt(cleanGoFlag),
	)
	return
}

func printSummary(ctx *tool.Context, results map[string]*test.Result) {
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
	Runner: cmdline.RunnerFunc(runTestList),
	Name:   "list",
	Short:  "List vanadium tests",
	Long:   "List vanadium tests.",
}

func runTestList(env *cmdline.Env, _ []string) error {
	ctx := tool.NewContextFromEnv(env, tool.ContextOpts{
		Color:    &colorFlag,
		DryRun:   &dryRunFlag,
		Manifest: &manifestFlag,
		Verbose:  &verboseFlag,
	})
	testList, err := v23test.ListTests()
	if err != nil {
		fmt.Fprintf(ctx.Stderr(), "%v\n", err)
		return err
	}
	for _, test := range testList {
		fmt.Fprintf(ctx.Stdout(), "%v\n", test)
	}
	return nil
}
