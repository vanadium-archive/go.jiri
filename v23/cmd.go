package main

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"v.io/x/devtools/lib/collect"
	"v.io/x/devtools/lib/gitutil"
	"v.io/x/devtools/lib/util"
	"v.io/x/devtools/lib/version"
	"v.io/x/lib/cmdline"
)

var (
	platformFlag            string
	hostGoFlag              string
	targetGoFlag            string
	manifestFlag            string
	branchesFlag            bool
	noPristineFlag          bool
	checkDirtyFlag          bool
	showCurrentRepoNameFlag bool
	verboseFlag             bool
	dryRunFlag              bool
	noColorFlag             bool
	remoteFlag              bool
	gcFlag                  bool
	attemptsFlag            int
	pkgsFlag                string
	prefixFlag              string
	reportFlag              bool
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	cmdEnv.Flags.StringVar(&platformFlag, "platform", "", "Target platform.")
	cmdGo.Flags.StringVar(&hostGoFlag, "host_go", "go", "Go command for the host platform.")
	cmdGo.Flags.StringVar(&targetGoFlag, "target_go", "go", "Go command for the target platform.")
	cmdProject.Flags.StringVar(&manifestFlag, "manifest", "default", "Name of the project manifest.")
	cmdProjectList.Flags.BoolVar(&branchesFlag, "branches", false, "Show project branches.")
	cmdProjectList.Flags.BoolVar(&noPristineFlag, "nopristine", false, "If true, omit pristine projects, i.e. projects with a clean master branch and no other branches.")
	cmdProjectShellPrompt.Flags.BoolVar(&checkDirtyFlag, "check_dirty", true, "If false, don't check for uncommitted changes or untracked files. Setting this option to false is dangerous: dirty master branches will not appear in the output.")
	cmdProjectShellPrompt.Flags.BoolVar(&showCurrentRepoNameFlag, "show_current_repo_name", false, "Show the name of the current repo.")
	cmdRoot.Flags.BoolVar(&verboseFlag, "v", false, "Print verbose output.")
	cmdRoot.Flags.BoolVar(&dryRunFlag, "n", false, "Show what commands will run but do not execute them.")
	cmdRoot.Flags.BoolVar(&noColorFlag, "nocolor", false, "Do not use color to format output.")
	cmdSnapshot.Flags.BoolVar(&remoteFlag, "remote", false, "Manage remote snapshots.")
	cmdTestRun.Flags.StringVar(&pkgsFlag, "pkgs", "", "Comma-separated list of Go package expressions that identify a subset of tests to run; only relevant for Go-based tests")
	cmdTestRun.Flags.BoolVar(&reportFlag, "report", false, "Upload test report to Vanadium servers.")
	cmdUpdate.Flags.BoolVar(&gcFlag, "gc", false, "Garbage collect obsolete repositories.")
	cmdUpdate.Flags.StringVar(&manifestFlag, "manifest", "default", "Name of the project manifest.")
	cmdUpdate.Flags.IntVar(&attemptsFlag, "attempts", 1, "Number of attempts before failing.")
	cmdV23Generate.Flags.StringVar(&prefixFlag, "prefix", defaultV23TestPrefix, "Specifies the prefix to use for generated files. Up to two files may generated, the defaults are v23_test.go and v23_internal_test.go, or <prefix>_test.go and <prefix>_internal_test.go.")
	// The "v23 xgo" commands has the same flags as "v23 go".
	cmdXGo.Flags = cmdGo.Flags
}

// root returns a command that represents the root of the v23 tool.
func root() *cmdline.Command {
	return cmdRoot
}

// cmdRoot represents the root of the v23 tool.
var cmdRoot = &cmdline.Command{
	Name:  "v23",
	Short: "Tool for managing vanadium development",
	Long:  "The v23 tool helps manage vanadium development.",
	Children: []*cmdline.Command{
		cmdBuildCop,
		cmdContributors,
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
		cmdXGo,
	},
}

// cmdContributors represents the "v23 contributors" command.
var cmdContributors = &cmdline.Command{
	Run:   runContributors,
	Name:  "contributors",
	Short: "List vanadium project contributors",
	Long: `
Lists vanadium project contributors and the number of their
commits. Vanadium projects to consider can be specified as an
argument. If no projects are specified, all vanadium projects are
considered by default.
`,
	ArgsName: "<projects>",
	ArgsLong: "<projects> is a list of projects to consider.",
}

func runContributors(command *cmdline.Command, args []string) error {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	projects, err := util.LocalProjects(ctx)
	if err != nil {
		return err
	}
	repos := map[string]struct{}{}
	if len(args) != 0 {
		for _, arg := range args {
			repos[arg] = struct{}{}
		}
	} else {
		for name, _ := range projects {
			repos[name] = struct{}{}
		}
	}
	contributors := map[string]int{}
	for repo, _ := range repos {
		project, ok := projects[repo]
		if !ok {
			continue
		}
		if err := ctx.Run().Chdir(project.Path); err != nil {
			return err
		}
		switch project.Protocol {
		case "git":
			lines, err := listCommitters(ctx.Git())
			if err != nil {
				return err
			}
			for _, line := range lines {
				tokens := strings.SplitN(line, "\t", 2)
				n, err := strconv.Atoi(strings.TrimSpace(tokens[0]))
				if err != nil {
					return fmt.Errorf("Atoi(%v) failed: %v", tokens[0], err)
				}
				contributors[strings.TrimSpace(tokens[1])] += n
			}
		default:
		}
	}
	names := []string{}
	for name, _ := range contributors {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(command.Stdout(), "%4d %v\n", contributors[name], name)
	}
	return nil
}

func listCommitters(git *gitutil.Git) (_ []string, e error) {
	branch, err := git.CurrentBranchName()
	if err != nil {
		return nil, err
	}
	stashed, err := git.Stash()
	if err != nil {
		return nil, err
	}
	if stashed {
		defer collect.Error(func() error { return git.StashPop() }, &e)
	}
	if err := git.CheckoutBranch("master", !gitutil.Force); err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return git.CheckoutBranch(branch, !gitutil.Force) }, &e)
	return git.Committers()
}

// cmdVersion represents the "v23 version" command.
var cmdVersion = &cmdline.Command{
	Run:   runVersion,
	Name:  "version",
	Short: "Print version",
	Long:  "Print version of the v23 tool.",
}

func runVersion(command *cmdline.Command, _ []string) error {
	fmt.Fprintf(command.Stdout(), "v23 tool version %v\n", version.Version)
	return nil
}
