// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package commandline provides a command line driver (for v.io/x/lib/cmdline)
// for implementing jiri 'profile' subcommands. The intent is to support
// project specific instances of such profiles for managing software
// dependencies.
package commandline

import (
	"bytes"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"v.io/jiri/profiles"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/envvar"
)

func init() {
	tool.InitializeRunFlags(&CommandLineDriver.Flags)
}

// CommandLineDriver implements the command line for the 'profile'
// subcommand.
var CommandLineDriver = &cmdline.Command{
	Name:  "profile",
	Short: "Manage profiles",
	Long: `
Profiles provide a means of managing software dependencies that can
be built natively as well as being cross compiled. A profile generally
manages a suite of related software components that are required for
a particular application (e.g. for android development).

Each profile can be in one of three states: absent, up-to-date, or
out-of-date. The subcommands of the profile command realize the
following transitions:

  install:   absent => up-to-date
  update:    out-of-date => up-to-date
  uninstall: up-to-date or out-of-date => absent

A profile can simultaneously have multiple versions, one of which is
configured as the default. A profile installation is out of date if the
installed versions are older than the current default. Updating that
profile will install the default version which will then be used by default.
Newer versions than the default may be installed and used via appropriate
command line flags.

To enable cross-compilation, a profile can be installed for multiple
targets. If a profile supports multiple targets the above state
transitions are applied on a profile + target basis.
`,
	Children: []*cmdline.Command{
		cmdInstall,
		cmdList,
		cmdEnv,
		cmdUninstall,
		cmdUpdate,
		cmdRecreate,
	},
}

// cmdInstall represents the "profile install" command.
var cmdInstall = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runInstall),
	Name:     "install",
	Short:    "Install the given profiles",
	Long:     "Install the given profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to install.",
}

// cmdList represents the "profile list" command.
var cmdList = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runList),
	Name:     "list",
	Short:    "List available or installed profiles",
	Long:     "List available or installed profiles.",
	ArgsName: "[<profiles>]",
	ArgsLong: "<profiles> is a list of profiles to list, defaulting to all profiles if none are specifically requested.",
}

// cmdEnv represents the "profile env" command.
var cmdEnv = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runEnv),
	Name:   "env",
	Short:  "Display profile environment variables",
	Long: `
List profile specific and target specific environment variables. If the
requested environment variable name ends in = then only the value will
be printed, otherwise both name and value are printed, i.e. GOPATH="foo" vs
just "foo".

If no environment variable names are requested then all will be printed
in <name>=<val> format.
`,
	ArgsName: "[<environment variable names>]",
	ArgsLong: "[<environment variable names>] is an optional list of environment variables to display",
}

// cmdUpdate represents the "profile update" command.
var cmdUpdate = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runUpdate),
	Name:     "update",
	Short:    "Install the latest default version of the given profiles",
	Long:     "Install the latest default version of the given profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to update, if omitted all profiles are updated.",
}

// cmdCleanup represents the "profile cleanup" command.
var cmdCleanup = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runCleanup),
	Name:     "cleanup",
	Short:    "Uninstall versions of the given profiles that are older than the default",
	Long:     "Uninstall versions of the given profiles that are older than the default.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to cleanup, if omitted all profiles are cleaned.",
}

// cmdRecreate represents the "profile recreate" command.
var cmdRecreate = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runRecreate),
	Name:     "recreate",
	Short:    "Display a list of commands that will recreate the currently installed profiles",
	Long:     "Display a list of commands that will recreate the currently installed profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to be recreated, if omitted commands to recreate all profiles are displayed.",
}

// cmdUninstall represents the "profile uninstall" command.
var cmdUninstall = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runUninstall),
	Name:     "uninstall",
	Short:    "Uninstall the given profiles",
	Long:     "Uninstall the given profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to uninstall.",
}

var (
	targetFlag       profiles.Target
	manifestFlag     string
	showManifestFlag bool
	profileFlag      string
	rootDir          string
	availableFlag    bool
	verboseFlag      bool
	allFlag          bool
)

func Main(name string) {
	CommandLineDriver.Name = name
	cmdline.Main(CommandLineDriver)
}

func Init(defaultManifestFilename string) {
	targetFlag = profiles.DefaultTarget()

	var err error
	rootDir, err = project.JiriRoot()
	if err != nil {
		panic(err)
	}
	manifest := filepath.Join(rootDir, defaultManifestFilename)

	// Every sub-command accepts: --manifest
	for _, fs := range []*flag.FlagSet{
		&cmdInstall.Flags,
		&cmdUpdate.Flags,
		&cmdCleanup.Flags,
		&cmdUninstall.Flags,
		&cmdEnv.Flags,
		&cmdList.Flags,
		&cmdRecreate.Flags} {
		profiles.RegisterManifestFlag(fs, &manifestFlag, manifest)
	}

	// install accepts: --target and, --env.
	profiles.RegisterTargetAndEnvFlags(&cmdInstall.Flags, &targetFlag)

	// uninstall and env accept: --target,
	for _, fs := range []*flag.FlagSet{
		&cmdUninstall.Flags,
		&cmdEnv.Flags} {
		profiles.RegisterTargetFlag(fs, &targetFlag)
	}

	// uninstall accept --all-targets but with different defaults.
	cmdUninstall.Flags.BoolVar(&allFlag, "all-targets", false, "apply to all targets for the specified profile(s)")

	// update accepts --v
	cmdUpdate.Flags.BoolVar(&verboseFlag, "v", false, "print more detailed information")

	// list accepts --show-manifest, --availabe, --v
	cmdList.Flags.BoolVar(&showManifestFlag, "show-manifest", false, "print out the manifest file")
	cmdList.Flags.BoolVar(&availableFlag, "available", false, "print the list of available profiles")
	cmdList.Flags.BoolVar(&verboseFlag, "v", false, "print more detailed information")

	// env accepts --profile
	cmdEnv.Flags.StringVar(&profileFlag, "profile", "", "the profile whose environment is to be displayed")

	for _, mgr := range profiles.Managers() {
		profiles.LookupManager(mgr).AddFlags(&cmdInstall.Flags, profiles.Install)
		profiles.LookupManager(mgr).AddFlags(&cmdUninstall.Flags, profiles.Uninstall)
	}
}

func runList(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env)
	if showManifestFlag {
		data, err := ctx.Run().ReadFile(manifestFlag)
		if err != nil {
			return err
		}
		fmt.Fprintln(ctx.Stdout(), string(data))
		return nil
	}
	if verboseFlag {
		fmt.Fprintf(ctx.Stdout(), "Manifest: %s\n", manifestFlag)
	}
	if availableFlag {
		if verboseFlag {
			fmt.Fprintf(ctx.Stdout(), "Available Profiles:\n")
			for _, name := range profiles.Managers() {
				mgr := profiles.LookupManager(name)
				vi := mgr.VersionInfo()
				fmt.Fprintf(ctx.Stdout(), "%s: versions: %s - %s\n", name, vi.Default(), strings.Join(vi.Supported(), " "))
			}
		} else {
			fmt.Fprintf(ctx.Stdout(), "%s\n", strings.Join(profiles.Managers(), ", "))
		}
	}
	if err := profiles.Read(ctx, manifestFlag); err != nil {
		fmt.Fprintf(ctx.Stderr(), "Failed to read manifest: %v", err)
		return err
	}
	profileNames := args
	if len(args) == 0 {
		profileNames = profiles.Profiles()
	}
	availableNames := []string{}
	for _, name := range profileNames {
		if profiles.LookupProfile(name) != nil {
			availableNames = append(availableNames, name)
		}
	}
	if verboseFlag {
		fmt.Fprintf(ctx.Stdout(), "Installed Profiles: ")
		fmt.Fprintf(ctx.Stdout(), "%s\n", strings.Join(profiles.Profiles(), ", "))
		for _, name := range availableNames {
			profile := profiles.LookupProfile(name)
			fmt.Fprintf(ctx.Stdout(), "Profile: %s @ %s\n", profile.Name, profile.Root)
			for _, target := range profile.Targets() {
				fmt.Fprintf(ctx.Stdout(), "\t%s\n", target.DebugString())
			}
		}
	} else {
		for _, name := range availableNames {
			profile := profiles.LookupProfile(name)
			for _, target := range profile.Targets() {
				fmt.Fprintf(ctx.Stdout(), "%s %s\n", name, target)
			}
		}
	}
	return nil
}

func runRecreate(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env)
	if err := profiles.Read(ctx, manifestFlag); err != nil {
		fmt.Fprintf(ctx.Stderr(), "Failed to read manifest: %v", err)
		return err
	}
	profileNames := args
	if len(args) == 0 {
		profileNames = profiles.Profiles()
	}
	prefix := "jiri v23-profile install"
	for _, name := range profileNames {
		profile := profiles.LookupProfile(name)
		if profile == nil {
			return fmt.Errorf("Profile %v is not installed", name)
		}
		for _, target := range profile.Targets() {
			fmt.Fprintf(ctx.Stdout(), "%s --target=%s", prefix, target)
			cmdEnv := target.CommandLineEnv()
			if len(cmdEnv.Vars) > 0 {
				fmt.Fprintf(ctx.Stdout(), " --env=\"%s\"", strings.Join(cmdEnv.Vars, ","))
			}
			fmt.Fprintf(ctx.Stdout(), " %s\n", name)
		}
	}
	return nil
}

type mapper func(target *profiles.Target) string

var pseudoVariables = map[string]mapper{
	"V23_TARGET_INSTALLATION_DIR": func(t *profiles.Target) string { return t.InstallationDir },
	"V23_TARGET_VERSION":          func(t *profiles.Target) string { return t.Version() },
}

func expr(k, v string, trimmed bool) string {
	if trimmed {
		return v
	}
	return fmt.Sprintf("%s=%q ", k, v)
}

func runEnv(env *cmdline.Env, args []string) error {
	if len(profileFlag) == 0 {
		return fmt.Errorf("no profile was specified using --profile")
	}
	ctx := tool.NewContextFromEnv(env)
	if err := profiles.Read(ctx, manifestFlag); err != nil {
		return fmt.Errorf("Failed to read manifest: %v", err)
	}
	profile := profiles.LookupProfile(profileFlag)
	if profile == nil {
		return fmt.Errorf("profile %q is not installed", profileFlag)
	}
	target := profiles.FindTarget(profile.Targets(), &targetFlag)
	if target == nil {
		return fmt.Errorf("target %q is not installed for profile %q", targetFlag, profileFlag)
	}
	vars := envvar.SliceToMap(target.Env.Vars)
	buf := bytes.Buffer{}
	if len(args) == 0 {
		for k, v := range vars {
			buf.WriteString(fmt.Sprintf("%s=%q ", k, v))
		}
		for k, fn := range pseudoVariables {
			buf.WriteString(fmt.Sprintf("%s=%q ", k, fn(target)))
		}
	} else {
		for _, arg := range args {
			name := strings.TrimSuffix(arg, "=")
			trimmed := name != arg
			for k, fn := range pseudoVariables {
				if k == name {
					buf.WriteString(expr(k, fn(target), trimmed))
				}
			}
			for k, v := range vars {
				if k == name {
					buf.WriteString(expr(k, v, trimmed))
				}
			}
		}
	}
	fmt.Fprintf(ctx.Stdout(), strings.TrimSuffix(buf.String(), " ")+"\n")
	return nil
}

func initCommand(ctx *tool.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no profiles specified")
	}
	for _, n := range args {
		if mgr := profiles.LookupManager(n); mgr == nil {
			return fmt.Errorf("profile %v is not available, use \"list --available\" to see the list of available profiles", n)
		}
	}
	if err := profiles.Read(ctx, manifestFlag); err != nil {
		fmt.Fprintf(ctx.Stderr(), "Failed to read manifest: %v", err)
		return err
	}
	return nil
}

func runUpdate(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env)
	if len(args) == 0 {
		args = profiles.Managers()
	}
	if err := initCommand(ctx, args); err != nil {
		return err
	}
	for _, n := range args {
		mgr := profiles.LookupManager(n)
		profile := profiles.LookupProfile(n)
		if profile == nil {
			continue
		}
		vi := mgr.VersionInfo()
		for _, target := range profile.Targets() {
			if vi.IsNewerThanDefault(target.Version()) {
				if verboseFlag {
					fmt.Fprintf(ctx.Stdout(), "Updating %s %s from %q to %s\n", n, target, target.Version(), vi)
				}
				target.SetVersion(vi.Default())
				err := mgr.Install(ctx, *target)
				logResult(ctx, "Update", mgr, *target, err)
				if err != nil {
					return err
				}
			} else {
				if verboseFlag {
					fmt.Fprintf(ctx.Stdout(), "%s %s at %q is up to date(%s)\n", n, target, target.Version(), vi)
				}
			}
		}
	}
	return profiles.Write(ctx, manifestFlag)
}

func runCleanup(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env)
	if len(args) == 0 {
		args = profiles.Managers()
	}
	if err := initCommand(ctx, args); err != nil {
		return err
	}
	for _, n := range args {
		mgr := profiles.LookupManager(n)
		vi := mgr.VersionInfo()
		profile := profiles.LookupProfile(n)
		for _, target := range profile.Targets() {
			if vi.IsOlderThanDefault(target.Version()) {
				err := mgr.Uninstall(ctx, *target)
				logResult(ctx, "Cleanup", mgr, *target, err)
				if err != nil {
					return err
				}
			}
		}
	}
	return profiles.Write(ctx, manifestFlag)
}

func logResult(ctx *tool.Context, action string, mgr profiles.Manager, target profiles.Target, err error) {
	fmt.Fprintf(ctx.Stdout(), "%s: %s %s: ", action, mgr.Name(), target)
	if err == nil {
		fmt.Fprintf(ctx.Stdout(), "success\n")
	} else {
		fmt.Fprintf(ctx.Stdout(), "%v\n", err)
	}
}

func applyCommand(names []string, env *cmdline.Env, ctx *tool.Context, target profiles.Target, fn func(profiles.Manager, *tool.Context, profiles.Target) error) error {
	for _, n := range names {
		mgr := profiles.LookupManager(n)
		version, err := mgr.VersionInfo().Select(target.Version())
		if err != nil {
			return err
		}
		target.SetVersion(version)
		mgr.SetRoot(rootDir)
		if err := fn(mgr, ctx, target); err != nil {
			return err
		}
	}
	return nil
}

func runInstall(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env)
	if err := initCommand(ctx, args); err != nil {
		return err
	}
	names := []string{}
	if len(args) == 0 {
		for _, name := range profiles.Managers() {
			names = append(names, name)
		}
	}
	for _, name := range args {
		if p := profiles.LookupProfileTarget(name, targetFlag); p != nil {
			fmt.Fprintf(ctx.Stdout(), "%v %v is already installed as %v\n", name, targetFlag, p)
			continue
		}
		names = append(names, name)
	}
	if err := applyCommand(names, env, ctx, targetFlag,
		func(mgr profiles.Manager, ctx *tool.Context, target profiles.Target) error {
			err := mgr.Install(ctx, target)
			logResult(ctx, "Install:", mgr, target, err)
			return err
		}); err != nil {
		return err
	}
	return profiles.Write(ctx, manifestFlag)
}

func runUninstall(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env)
	if err := initCommand(ctx, args); err != nil {
		return err
	}
	if allFlag && targetFlag.IsSet() {
		return fmt.Errorf("don't specify a target in conjunction with --all")
	}
	if allFlag {
		for _, name := range args {
			profile := profiles.LookupProfile(name)
			mgr := profiles.LookupManager(name)
			if profile == nil || mgr == nil {
				continue
			}
			mgr.SetRoot(rootDir)
			for _, target := range profile.Targets() {
				if err := mgr.Uninstall(ctx, *target); err != nil {
					logResult(ctx, "Uninstall", mgr, *target, err)
					return err
				}
				logResult(ctx, "Uninstall", mgr, *target, nil)
			}
		}
	} else {
		applyCommand(args, env, ctx, targetFlag,
			func(mgr profiles.Manager, ctx *tool.Context, target profiles.Target) error {
				err := mgr.Uninstall(ctx, target)
				logResult(ctx, "Uninstall", mgr, target, err)
				return err
			})
	}
	return profiles.Write(ctx, manifestFlag)
}
