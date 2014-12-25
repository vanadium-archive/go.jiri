package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"v.io/lib/cmdline"
	"v.io/tools/lib/collect"
	"v.io/tools/lib/envutil"
	"v.io/tools/lib/util"
)

// cmdGo represents the "v23 go" command.
var cmdGo = &cmdline.Command{
	Run:   runGo,
	Name:  "go",
	Short: "Execute the go tool using the vanadium environment",
	Long: `
Wrapper around the 'go' tool that can be used for compilation of
vanadium Go sources. It takes care of vanadium-specific setup, such as
setting up the Go specific environment variables or making sure that
VDL generated files are regenerated before compilation.

In particular, the tool invokes the following command before invoking
any go tool commands that compile vanadium Go code:

vdl generate -lang=go all
`,
	ArgsName: "<arg ...>",
	ArgsLong: "<arg ...> is a list of arguments for the go tool.",
}

func runGo(command *cmdline.Command, args []string) error {
	if len(args) == 0 {
		return command.UsageErrorf("not enough arguments")
	}
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	return runGoForPlatform(ctx, util.HostPlatform(), command, args)
}

// cmdXGo represents the "v23 xgo" command.
var cmdXGo = &cmdline.Command{
	Run:   runXGo,
	Name:  "xgo",
	Short: "Execute the go tool using the vanadium environment and cross-compilation",
	Long: `
Wrapper around the 'go' tool that can be used for cross-compilation of
vanadium Go sources. It takes care of vanadium-specific setup, such as
setting up the Go specific environment variables or making sure that
VDL generated files are regenerated before compilation.

In particular, the tool invokes the following command before invoking
any go tool commands that compile vanadium Go code:

vdl generate -lang=go all

`,
	ArgsName: "<platform> <arg ...>",
	ArgsLong: `
<platform> is the cross-compilation target and has the general format
<arch><sub>-<os> or <arch><sub>-<os>-<env> where:
- <arch> is the platform architecture (e.g. 386, amd64 or arm)
- <sub> is the platform sub-architecture (e.g. v6 or v7 for arm)
- <os> is the platform operating system (e.g. linux or darwin)
- <env> is the platform environment (e.g. gnu or android)

<arg ...> is a list of arguments for the go tool."
`,
}

func runXGo(command *cmdline.Command, args []string) error {
	if len(args) < 2 {
		return command.UsageErrorf("not enough arguments")
	}
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	platform, err := util.ParsePlatform(args[0])
	if err != nil {
		return err
	}
	return runGoForPlatform(ctx, platform, command, args[1:])
}

func runGoForPlatform(ctx *util.Context, platform util.Platform, command *cmdline.Command, args []string) error {
	// Generate vdl files, if necessary.
	switch args[0] {
	case "build", "generate", "install", "run", "test":
		// Check that all non-master branches have merged the
		// master branch to make sure the vdl tool is not run
		// against out-of-date code base.
		if err := reportOutdatedBranches(ctx); err != nil {
			return err
		}

		if err := generateVDL(ctx, args); err != nil {
			return err
		}
	}

	// Run the go tool for the given platform.
	targetEnv, err := util.VanadiumEnvironment(platform)
	if err != nil {
		return err
	}
	bin, err := targetEnv.LookPath(targetGoFlag)
	if err != nil {
		return err
	}
	opts := ctx.Run().Opts()
	opts.Env = targetEnv.Map()
	return translateExitCode(ctx.Run().CommandWithOpts(opts, bin, args...))
}

// generateVDL generates VDL for the transitive Go package
// dependencies.
//
// Note that the vdl tool takes VDL packages as input, but we're
// supplying Go packages.  We're assuming the package paths for the
// VDL packages we want to generate have the same path names as the Go
// package paths.  Some of the Go package paths may not correspond to
// a valid VDL package, so we provide the -ignore_unknown flag to
// silently ignore these paths.
//
// It's fine if the VDL packages have dependencies not reflected in
// the Go packages; the vdl tool will compute the transitive closure
// of VDL package dependencies, as usual.
//
// TODO(toddw): Change the vdl tool to return vdl packages given the
// full Go dependencies, after vdl config files are implemented.
func generateVDL(ctx *util.Context, cmdArgs []string) error {
	hostEnv, err := util.VanadiumEnvironment(util.HostPlatform())
	if err != nil {
		return err
	}

	// Compute which VDL-based Go packages might need to be regenerated.
	goPkgs, goFiles := extractGoPackagesOrFiles(cmdArgs[0], cmdArgs[1:])
	goDeps, err := computeGoDeps(ctx, hostEnv, append(goPkgs, goFiles...))
	if err != nil {
		return err
	}

	// Regenerate the VDL-based Go packages.
	vdlArgs := []string{"-ignore_unknown", "generate", "-lang=go"}
	vdlArgs = append(vdlArgs, goDeps...)
	vdlBin, err := hostEnv.LookPath("vdl")
	if err != nil {
		return err
	}
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	opts.Env = hostEnv.Map()
	if err := ctx.Run().CommandWithOpts(opts, vdlBin, vdlArgs...); err != nil {
		return fmt.Errorf("failed to generate vdl: %v\n%s", err, out.String())
	}
	return nil
}

// reportOutdatedProjects checks if the currently checked out branches
// are up-to-date with respect to the local master branch. For each
// branch that is not, a notification is printed.
func reportOutdatedBranches(ctx *util.Context) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	projects, err := util.LocalProjects(ctx)
	for _, project := range projects {
		if err := ctx.Run().Chdir(project.Path); err != nil {
			return err
		}
		switch project.Protocol {
		case "git":
			branches, _, err := ctx.Git().GetBranches("--merged")
			if err != nil {
				return err
			}
			found := false
			for _, branch := range branches {
				if branch == "master" {
					found = true
					break
				}
			}
			merging, err := ctx.Git().MergeInProgress()
			if err != nil {
				return err
			}
			if !found && !merging {
				fmt.Fprintf(ctx.Stderr(), "NOTE: project=%q path=%q\n", path.Base(project.Name), project.Path)
				fmt.Fprintf(ctx.Stderr(), "This project is on a non-master branch that is out of date.\n")
				fmt.Fprintf(ctx.Stderr(), "Please update this branch using %q.\n", "git merge master")
				fmt.Fprintf(ctx.Stderr(), "Until then the %q tool might not function properly.\n", "v23")
			}
		}
	}
	return nil
}

// extractGoPackagesOrFiles is given the cmd and args for the go tool, filters
// out flags, and returns the PACKAGES or GOFILES that were specified in args.
// Note that all commands that accept PACKAGES also accept GOFILES.
//
//   go build    [build flags]              [-o out]      [PACKAGES]
//   go generate                            [-run regexp] [PACKAGES]
//   go install  [build flags]                            [PACKAGES]
//   go run      [build flags]              [-exec prog]  [GOFILES]  [run args]
//   go test     [build flags] [test flags] [-exec prog]  [PACKAGES] [testbin flags]
//
// Sadly there's no way to do this syntactically.  It's easy for single token
// -flag and -flag=x, but non-boolean flags may be two tokens "-flag x".
//
// We keep track of all non-boolean flags F, and skip every token that starts
// with - or --, and also skip the next token if the flag is in F and isn't of
// the form -flag=x.  If we forget to update F, we'll still handle the -flag and
// -flag=x cases correctly, but we'll get "-flag x" wrong.
func extractGoPackagesOrFiles(cmd string, args []string) ([]string, []string) {
	var nonBool map[string]bool
	switch cmd {
	case "build":
		nonBool = nonBoolGoBuild
	case "generate":
		nonBool = nonBoolGoGenerate
	case "install":
		nonBool = nonBoolGoInstall
	case "run":
		nonBool = nonBoolGoRun
	case "test":
		nonBool = nonBoolGoTest
	}

	// Move start to the start of PACKAGES or GOFILES, by skipping flags.
	start := 0
	for start < len(args) {
		// Handle special-case terminator --
		if args[start] == "--" {
			start++
			break
		}
		match := goFlagRE.FindStringSubmatch(args[start])
		if match == nil {
			break
		}
		// Skip this flag, and maybe skip the next token for the "-flag x" case.
		//   match[1] is the flag name
		//   match[2] is the optional "=" for the -flag=x case
		start++
		if nonBool[match[1]] && match[2] == "" {
			start++
		}
	}

	// Move end to the end of PACKAGES or GOFILES.
	var end int
	switch cmd {
	case "test":
		// Any arg starting with - is a testbin flag.
		// https://golang.org/cmd/go/#hdr-Test_packages
		for end = start; end < len(args); end++ {
			if strings.HasPrefix(args[end], "-") {
				break
			}
		}
	case "run":
		// Go run takes gofiles, which are defined as a file ending in ".go".
		// https://golang.org/cmd/go/#hdr-Compile_and_run_Go_program
		for end = start; end < len(args); end++ {
			if !strings.HasSuffix(args[end], ".go") {
				break
			}
		}
	default:
		end = len(args)
	}

	// Decide whether these are packages or files.
	switch {
	case start == end:
		return nil, nil
	case (start < len(args) && strings.HasSuffix(args[start], ".go")):
		return nil, args[start:end]
	default:
		return args[start:end], nil
	}
}

var (
	goFlagRE     = regexp.MustCompile(`^--?([^=]+)(=?)`)
	nonBoolBuild = []string{
		"p", "ccflags", "compiler", "gccgoflags", "gcflags", "installsuffix", "ldflags", "tags",
	}
	nonBoolTest = []string{
		"bench", "benchtime", "blockprofile", "blockprofilerate", "covermode", "coverpkg", "coverprofile", "cpu", "cpuprofile", "memprofile", "memprofilerate", "outputdir", "parallel", "run", "timeout",
	}
	nonBoolGoBuild    = makeStringSet(append(nonBoolBuild, "o"))
	nonBoolGoGenerate = makeStringSet([]string{"run"})
	nonBoolGoInstall  = makeStringSet(nonBoolBuild)
	nonBoolGoRun      = makeStringSet(append(nonBoolBuild, "exec"))
	nonBoolGoTest     = makeStringSet(append(append(nonBoolBuild, nonBoolTest...), "exec"))
)

func makeStringSet(values []string) map[string]bool {
	ret := make(map[string]bool)
	for _, v := range values {
		ret[v] = true
	}
	return ret
}

// computeGoDeps computes the transitive Go package dependencies for the given
// set of pkgs.  The strategy is to run "go list <pkgs>" with a special format
// string that dumps the specified pkgs and all deps as space / newline
// separated tokens.  The pkgs may be in any format recognized by "go list"; dir
// paths, import paths, or go files.
func computeGoDeps(ctx *util.Context, env *envutil.Snapshot, pkgs []string) ([]string, error) {
	goListArgs := []string{`list`, `-f`, `{{.ImportPath}} {{join .Deps " "}}`}
	goListArgs = append(goListArgs, pkgs...)
	var stdout, stderr bytes.Buffer
	// TODO(jsimsa): Avoid buffering all of the output in memory
	// either by extending the runutil API to support piping of
	// output, or by writing the output to a temporary file
	// instead of an in-memory buffer.
	opts := ctx.Run().Opts()
	opts.Stdout = &stdout
	opts.Stderr = &stderr
	opts.Env = env.Map()
	if err := ctx.Run().CommandWithOpts(opts, hostGoFlag, goListArgs...); err != nil {
		return nil, fmt.Errorf("failed to compute go deps: %v\n%s", err, stderr.String())
	}
	scanner := bufio.NewScanner(&stdout)
	scanner.Split(bufio.ScanWords)
	depsMap := make(map[string]bool)
	for scanner.Scan() {
		depsMap[scanner.Text()] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Scan() failed: %v", err)
	}
	var deps []string
	for dep, _ := range depsMap {
		// Filter out bad packages:
		//   command-line-arguments is the dummy import path for "go run".
		switch dep {
		case "command-line-arguments":
			continue
		}
		deps = append(deps, dep)
	}
	return deps, nil
}

// cmdGoExt represents the "v23 goext" command.
var cmdGoExt = &cmdline.Command{
	Name:     "goext",
	Short:    "Vanadium extensions of the go tool",
	Long:     "Vanadium extension of the go tool.",
	Children: []*cmdline.Command{cmdGoExtDistClean},
}

// cmdGoExtDistClean represents the "v23 goext distclean" command.
var cmdGoExtDistClean = &cmdline.Command{
	Run:   runGoExtDistClean,
	Name:  "distclean",
	Short: "Restore the vanadium Go workspaces to their pristine state",
	Long: `
Unlike the 'go clean' command, which only removes object files for
packages in the source tree, the 'goext disclean' command removes all
object files from vanadium Go workspaces. This functionality is needed
to avoid accidental use of stale object files that correspond to
packages that no longer exist in the source tree.
`,
}

func runGoExtDistClean(command *cmdline.Command, _ []string) error {
	ctx := util.NewContextFromCommand(command, !noColorFlag, dryRunFlag, verboseFlag)
	env, err := util.VanadiumEnvironment(util.HostPlatform())
	if err != nil {
		return err
	}
	failed := false
	for _, workspace := range env.GetTokens("GOPATH", ":") {
		for _, name := range []string{"bin", "pkg"} {
			dir := filepath.Join(workspace, name)
			if err := ctx.Run().RemoveAll(dir); err != nil {
				failed = true
			}
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}
