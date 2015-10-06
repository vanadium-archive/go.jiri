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
	Name:  "xprofile",
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

In addition, a profile can transition from being up-to-date to
out-of-date by the virtue of a new version of the profile being
released.

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

// cmdList represents the "profile env" command.
var cmdEnv = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runEnv),
	Name:   "env",
	Short:  "Display profile environment variables",
	Long: `
List profile specific and target specific environment variables.

If no environment variable names are requested then all will be printed.
`,
	ArgsName: "[<environment variable names>]",
	ArgsLong: "[<environment variable names>] is an optional list of environment variables to display",
}

// cmdUpdate represents the "profile update" command.
var cmdUpdate = &cmdline.Command{
	Runner:   cmdline.RunnerFunc(runUpdate),
	Name:     "update",
	Short:    "Update the given profiles",
	Long:     "Update the given profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to update.",
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
	forceFlag        bool
	availableFlag    bool
	verboseFlag      bool
	allFlag          bool
)

func Main() {
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

	commonFlags := func(flags *flag.FlagSet) {
		flags.Var(&targetFlag, "target", targetFlag.Usage())
		flags.Lookup("target").DefValue = "<runtime.GOARCH>-<runtime.GOOS>"
		flags.Var(&targetFlag.Env, "env", targetFlag.Env.Usage())
		flags.StringVar(&targetFlag.Version, "version", "", "target version")
		flags.StringVar(&manifestFlag, "manifest", manifest, "specify the XML manifest to file read/write from.")
		flags.Lookup("manifest").DefValue = "$JIRI_ROOT/" + defaultManifestFilename
	}
	commonFlags(&cmdInstall.Flags)
	commonFlags(&cmdUpdate.Flags)
	commonFlags(&cmdUninstall.Flags)
	cmdList.Flags.StringVar(&manifestFlag, "manifest", manifest, "specify the XML manifest to file read/write from.")
	cmdList.Flags.Lookup("manifest").DefValue = "$JIRI_ROOT/" + defaultManifestFilename
	cmdList.Flags.BoolVar(&showManifestFlag, "show-manifest", false, "print out the manifest file")
	cmdList.Flags.BoolVar(&availableFlag, "available", false, "print the list of available profiles")
	cmdList.Flags.BoolVar(&verboseFlag, "v", false, "print more detailed information")
	cmdEnv.Flags.StringVar(&manifestFlag, "manifest", manifest, "specify the XML manifest to file read/write from.")
	cmdEnv.Flags.Lookup("manifest").DefValue = "$JIRI_ROOT/" + defaultManifestFilename
	cmdEnv.Flags.StringVar(&profileFlag, "profile", "", "the profile whose environment is to be displayed")
	cmdEnv.Flags.Var(&targetFlag, "target", targetFlag.Usage())
	cmdEnv.Flags.Lookup("target").DefValue = "<runtime.GOARCH>-<runtime.GOOS>"
	cmdEnv.Flags.StringVar(&targetFlag.Version, "version", "", "target version")

	for _, mgr := range profiles.Managers() {
		profiles.LookupManager(mgr).AddFlags(&cmdInstall.Flags, profiles.Install)
		profiles.LookupManager(mgr).AddFlags(&cmdUpdate.Flags, profiles.Update)
		profiles.LookupManager(mgr).AddFlags(&cmdUninstall.Flags, profiles.Uninstall)
	}
	cmdUpdate.Flags.BoolVar(&forceFlag, "force", false, "force an uninstall followed by install")
	cmdUninstall.Flags.BoolVar(&allFlag, "all", false, "uninstall all targets for the specified profile(s)")
}

func applyCommand(names []string, env *cmdline.Env, ctx *tool.Context, target profiles.Target, fn func(profiles.Manager, *tool.Context, profiles.Target) error) error {
	for _, n := range names {
		mgr := profiles.LookupManager(n)
		mgr.SetRoot(rootDir)
		if err := fn(mgr, ctx, target); err != nil {
			return err
		}
	}
	return nil
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
			fmt.Fprintf(ctx.Stdout(), "Available Profiles: ")
		}
		fmt.Fprintf(ctx.Stdout(), "%s\n", strings.Join(profiles.Managers(), ", "))
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
			for _, target := range profile.Targets {
				fmt.Fprintf(ctx.Stdout(), "\t%s\n", target)
			}
		}
	} else {
		for _, name := range availableNames {
			profile := profiles.LookupProfile(name)
			for _, target := range profile.Targets {
				fmt.Fprintf(ctx.Stdout(), "%s %s=%s-%s\n", name, target.Tag, target.Arch, target.OS)
			}
		}
	}
	return nil
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
	target := profiles.FindTarget(profile.Targets, &targetFlag)
	if target == nil {
		return fmt.Errorf("target %q is not installed for profile %q", targetFlag, profileFlag)
	}
	vars := envvar.SliceToMap(target.Env.Vars)
	buf := bytes.Buffer{}
	if len(args) == 0 {
		for k, v := range vars {
			buf.WriteString(fmt.Sprintf("%s=%q ", k, v))
		}
	} else {
		for _, name := range args {
			for k, v := range vars {
				if k == name {
					buf.WriteString(fmt.Sprintf("%s=%q ", k, v))
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
	if err := profiles.ValidateRequestedProfiles(args); err != nil {
		return err
	}
	if err := profiles.Read(ctx, manifestFlag); err != nil {
		fmt.Fprintf(ctx.Stderr(), "Failed to read manifest: %v", err)
		return err
	}
	return nil
}

func logAction(ctx *tool.Context, action string, mgr profiles.Manager, target profiles.Target) {
	fmt.Fprintf(ctx.Stdout(), "%s: %s %s=%s-%s@%s...", action, mgr.Name(), target.Tag, target.Arch, target.OS, target.Version)
}

func logResult(ctx *tool.Context, err error) {
	if err == nil {
		fmt.Fprintf(ctx.Stdout(), " done\n")
	} else {
		fmt.Fprintf(ctx.Stdout(), " %v\n", err)
	}
}

func runInstall(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env)
	if err := initCommand(ctx, args); err != nil {
		return err
	}
	names := []string{}
	for _, name := range args {
		if profiles.HasTarget(name, targetFlag) {
			fmt.Fprintf(ctx.Stdout(), "%v %v is already installed\n", name, targetFlag)
			continue
		}
		names = append(names, name)
	}
	if err := applyCommand(names, env, ctx, targetFlag,
		func(mgr profiles.Manager, ctx *tool.Context, target profiles.Target) error {
			logAction(ctx, "Installing", mgr, target)
			err := mgr.Install(ctx, target)
			logResult(ctx, err)
			return err
		}); err != nil {
		return err
	}
	return profiles.Write(ctx, manifestFlag)
}

func runUpdate(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env)
	if err := initCommand(ctx, args); err != nil {
		return err
	}
	if err := applyCommand(args, env, ctx, targetFlag,
		func(mgr profiles.Manager, ctx *tool.Context, target profiles.Target) error {
			logAction(ctx, "Updating", mgr, target)
			err := mgr.Update(ctx, target)
			logResult(ctx, err)
			if (forceFlag && err == nil) || err == profiles.ErrNoIncrementalUpdate {
				logAction(ctx, "Uninstalling", mgr, target)
				if err := mgr.Uninstall(ctx, target); err != nil {
					logResult(ctx, err)
					return err
				}
				logAction(ctx, "Installing", mgr, target)
				if err := mgr.Install(ctx, target); err != nil {
					logResult(ctx, err)
					return err
				}
				logResult(ctx, err)
				return nil
			}
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
			if mgr == nil {
				continue
			}
			mgr.SetRoot(rootDir)
			for _, target := range profile.Targets {
				logAction(ctx, "Uninstalling", mgr, *target)
				if err := mgr.Uninstall(ctx, *target); err != nil {
					logResult(ctx, err)
					return err
				}
				logResult(ctx, nil)

			}
		}
	} else {
		applyCommand(args, env, ctx, targetFlag,
			func(mgr profiles.Manager, ctx *tool.Context, target profiles.Target) error {
				logAction(ctx, "Uninstalling", mgr, target)
				err := mgr.Uninstall(ctx, target)
				logResult(ctx, err)
				return err
			})
	}
	return profiles.Write(ctx, manifestFlag)
}
