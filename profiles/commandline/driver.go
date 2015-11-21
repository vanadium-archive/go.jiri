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
	"text/template"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/textutil"
)

func init() {
	tool.InitializeRunFlags(&CommandLineDriver.Flags)
}

// CommandLineDriver implements the command line for the 'profile'
// subcommand.
var CommandLineDriver = &cmdline.Command{
	Name:  "profile",
	Short: "Manage profiles",
	Long:  helpMsg,
	Children: []*cmdline.Command{
		cmdInstall,
		cmdList,
		cmdEnv,
		cmdUninstall,
		cmdUpdate,
		cmdCleanup,
	},
}

// cmdInstall represents the "profile install" command.
var cmdInstall = &cmdline.Command{
	Runner:   jiri.RunnerFunc(runInstall),
	Name:     "install",
	Short:    "Install the given profiles",
	Long:     "Install the given profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to install.",
}

// cmdList represents the "profile list" command.
var cmdList = &cmdline.Command{
	Runner:   jiri.RunnerFunc(runList),
	Name:     "list",
	Short:    "List available or installed profiles",
	Long:     "List available or installed profiles.",
	ArgsName: "[<profiles>]",
	ArgsLong: "<profiles> is a list of profiles to list, defaulting to all profiles if none are specifically requested.",
}

// cmdEnv represents the "profile env" command.
var cmdEnv = &cmdline.Command{
	Runner: jiri.RunnerFunc(runEnv),
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
	Runner:   jiri.RunnerFunc(runUpdate),
	Name:     "update",
	Short:    "Install the latest default version of the given profiles",
	Long:     "Install the latest default version of the given profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to update, if omitted all profiles are updated.",
}

// cmdCleanup represents the "profile Cleanup" command.
var cmdCleanup = &cmdline.Command{
	Runner:   jiri.RunnerFunc(runCleanup),
	Name:     "cleanup",
	Short:    "Cleanup the locally installed profiles",
	Long:     "Cleanup the locally installed profiles. This is generally required when recovering from earlier bugs or when preparing for a subsequent change to the profiles implementation.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to cleanup, if omitted all profiles are cleaned.",
}

// cmdUninstall represents the "profile uninstall" command.
var cmdUninstall = &cmdline.Command{
	Runner:   jiri.RunnerFunc(runUninstall),
	Name:     "uninstall",
	Short:    "Uninstall the given profiles",
	Long:     "Uninstall the given profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to uninstall.",
}

var (
	rootPath             profiles.RelativePath
	targetFlag           profiles.Target
	manifestFlag         string
	showManifestFlag     bool
	profilesFlag         string
	rootDir              string
	availableFlag        bool
	verboseFlag          bool
	allFlag              bool
	infoFlag             string
	mergePoliciesFlag    profiles.MergePolicies
	specificVersionsFlag bool
	cleanupFlag          bool
	rmAllFlag            bool
	rewriteManifestFlag  bool
)

func Main(name string) {
	CommandLineDriver.Name = name
	cmdline.Main(CommandLineDriver)
}

func Init(defaultManifestFilename string) {
	targetFlag = profiles.DefaultTarget()
	mergePoliciesFlag = profiles.JiriMergePolicies()

	var err error
	rootDir, err = project.JiriRoot()
	if err != nil {
		panic(err)
	}

	rootPath = profiles.NewRelativePath("JIRI_ROOT", rootDir).Join("profiles")

	// Every sub-command accepts: --manifest
	for _, fs := range []*flag.FlagSet{
		&cmdInstall.Flags,
		&cmdUpdate.Flags,
		&cmdUninstall.Flags,
		&cmdCleanup.Flags,
		&cmdEnv.Flags,
		&cmdList.Flags} {
		profiles.RegisterManifestFlag(fs, &manifestFlag, defaultManifestFilename)
	}

	// install accepts: --target and, --env.
	profiles.RegisterTargetAndEnvFlags(&cmdInstall.Flags, &targetFlag)

	// uninstall, env and list accept: --target,
	for _, fs := range []*flag.FlagSet{
		&cmdUninstall.Flags,
		&cmdEnv.Flags,
		&cmdList.Flags} {
		profiles.RegisterTargetFlag(fs, &targetFlag)
	}

	// env accepts --profiles and --merge-policies
	profiles.RegisterProfilesFlag(&cmdEnv.Flags, &profilesFlag)
	profiles.RegisterMergePoliciesFlag(&cmdEnv.Flags, &mergePoliciesFlag)

	// uninstall, list, env and cleanup accept: --v
	for _, fs := range []*flag.FlagSet{
		&cmdUpdate.Flags,
		&cmdList.Flags,
		&cmdCleanup.Flags,
		&cmdEnv.Flags} {
		fs.BoolVar(&verboseFlag, "v", false, "print more detailed information")
	}

	// uninstall accept --all-targets but with different defaults.
	cmdUninstall.Flags.BoolVar(&allFlag, "all-targets", false, "apply to all targets for the specified profile(s)")

	// list accepts --show-profiles-manifest, --available, --dir, --default, --versions
	cmdList.Flags.BoolVar(&showManifestFlag, "show-profiles-manifest", false, "print out the manifest file")
	cmdList.Flags.BoolVar(&availableFlag, "available", false, "print the list of available profiles")
	cmdList.Flags.StringVar(&infoFlag, "info", "", infoUsage())

	for _, mgr := range profiles.Managers() {
		profiles.LookupManager(mgr).AddFlags(&cmdInstall.Flags, profiles.Install)
		profiles.LookupManager(mgr).AddFlags(&cmdUninstall.Flags, profiles.Uninstall)
	}

	// cleanup accepts the following flags:
	cmdCleanup.Flags.BoolVar(&cleanupFlag, "gc", false, "uninstall profile targets that are older than the current default")
	cmdCleanup.Flags.BoolVar(&specificVersionsFlag, "ensure-specific-versions-are-set", false, "ensure that profile targets have a specific version set")
	cmdCleanup.Flags.BoolVar(&rmAllFlag, "rm-all", false, "remove profiles manifest and all profile generated output files.")
	cmdCleanup.Flags.BoolVar(&rewriteManifestFlag, "rewrite-profiles-manifest", false, "rewrite the profiles manifest file to use the latest schema version")
}

func runList(jirix *jiri.X, args []string) error {
	if showManifestFlag {
		data, err := jirix.Run().ReadFile(manifestFlag)
		if err != nil {
			return err
		}
		fmt.Fprintln(jirix.Stdout(), string(data))
		return nil
	}
	if verboseFlag {
		fmt.Fprintf(jirix.Stdout(), "Manifest: %s\n", manifestFlag)
	}
	if availableFlag {
		if verboseFlag {
			fmt.Fprintf(jirix.Stdout(), "Available Profiles:\n")
			for _, name := range profiles.Managers() {
				mgr := profiles.LookupManager(name)
				vi := mgr.VersionInfo()
				fmt.Fprintf(jirix.Stdout(), "%s: versions: %s\n", name, vi)
			}
		} else {
			fmt.Fprintf(jirix.Stdout(), "%s\n", strings.Join(profiles.Managers(), ", "))
		}
	}
	if err := profiles.Read(jirix, manifestFlag); err != nil {
		fmt.Fprintf(jirix.Stderr(), "Failed to read manifest: %v", err)
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
		fmt.Fprintf(jirix.Stdout(), "Installed Profiles: ")
		fmt.Fprintf(jirix.Stdout(), "%s\n", strings.Join(profiles.Profiles(), ", "))
		for _, name := range availableNames {
			profile := profiles.LookupProfile(name)
			fmt.Fprintf(jirix.Stdout(), "Profile: %s @ %s\n", profile.Name, profile.Root)
			for _, target := range profile.Targets() {
				fmt.Fprintf(jirix.Stdout(), "\t%s\n", target.DebugString())
			}
		}
	} else {
		for _, name := range availableNames {
			profile := profiles.LookupProfile(name)
			mgr := profiles.LookupManager(name)
			out := &bytes.Buffer{}
			var targets profiles.OrderedTargets
			if targetFlag.IsSet() {
				targets = append(targets, profiles.LookupProfileTarget(name, targetFlag))
			} else {
				targets = profile.Targets()
			}
			printHeader := len(availableNames) > 1 || len(targets) > 1 || len(infoFlag) == 0
			for _, target := range targets {
				if printHeader {
					out.WriteString(fmtHeader(name, target))
					out.WriteString(" ")
				}
				r, err := fmtInfo(jirix, mgr, profile, target)
				if err != nil {
					return err
				}
				out.WriteString(r)
				if printHeader {
					out.WriteString("\n")
				}
			}
			fmt.Fprint(jirix.Stdout(), out.String())
		}
	}
	return nil
}

func fmtHeader(name string, target *profiles.Target) string {
	if target == nil {
		return name
	}
	return name + " " + target.String()
}

type listInfo struct {
	SchemaVersion profiles.Version
	Target        struct {
		InstallationDir string
		CommandLineEnv  []string
		Env             []string
		Command         string
	}
	Profile struct {
		Description    string
		Root           string
		DefaultVersion string
		LatestVersion  string
		Versions       []string
	}
}

func infoUsage() string {
	return `The following fields for use with --profile-info are available:
	SchemaVersion - the version of the profiles implementation.
	Target.InstallationDir - the installation directory of the requested profile.
	Target.CommandLineEnv - the environment variables specified via the command line when installing this profile target.
	Target.Env - the environment variables computed by the profile installation process for this target.
	Target.Command - a command that can be used to create this profile.
	Note: if no --target is specified then the requested field will be displayed for all targets.
	Profile.Description - description of the requested profile.
	Profile.Root - the root directory of the requested profile.
	Profile.Versions - the set of supported versions for this profile.
	Profile.DefaultVersion - the default version of the requested profile.
	Profile.LatestVersion - the latest version available for the requested profile.
	Note: if no profiles are specified then the requested field will be displayed for all profiles.`
}

func fmtOutput(jirix *jiri.X, o string) string {
	_, width, err := textutil.TerminalSize()
	if err != nil {
		width = 80
	}
	if len(o) < width {
		return o
	}
	out := &bytes.Buffer{}
	w := textutil.NewUTF8LineWriter(out, width)
	fmt.Fprint(w, o)
	w.Flush()
	return out.String()
}

func handleRelativePath(root profiles.RelativePath, s string) string {
	// Handle the transition from absolute to relative paths.
	if filepath.IsAbs(s) {
		return s
	}
	return root.RootJoin(s).Expand()
}

func fmtInfo(jirix *jiri.X, mgr profiles.Manager, profile *profiles.Profile, target *profiles.Target) (string, error) {
	if len(infoFlag) > 0 {
		// Populate an instance listInfo
		info := &listInfo{}
		name := ""
		if mgr != nil {
			// Format the description on its own, without any preceeding
			// text so that the line breaks work correctly.
			info.Profile.Description = "\n" + fmtOutput(jirix, mgr.Info()) + "\n"
			vi := mgr.VersionInfo()
			if supported := vi.Supported(); len(supported) > 0 {
				info.Profile.Versions = supported
				info.Profile.LatestVersion = supported[0]
			}
			info.Profile.DefaultVersion = vi.Default()
			name = mgr.Name()
		}
		info.SchemaVersion = profiles.SchemaVersion()
		if target != nil {
			info.Target.InstallationDir = handleRelativePath(rootPath, target.InstallationDir)
			info.Target.CommandLineEnv = target.CommandLineEnv().Vars
			info.Target.Env = target.Env.Vars
			clenv := ""
			if len(info.Target.CommandLineEnv) > 0 {
				clenv = fmt.Sprintf(" --env=\"%s\" ", strings.Join(info.Target.CommandLineEnv, ","))
			}
			info.Target.Command = fmt.Sprintf("jiri v23-profile install --target=%s %s%s", target, clenv, name)
		}
		if profile != nil {
			info.Profile.Root = handleRelativePath(rootPath, profile.Root)
		}

		// Use a template to print out any field in our instance of listInfo.
		tmpl, err := template.New("list").Parse("{{ ." + infoFlag + "}}")
		if err != nil {
			return "", err
		}
		out := &bytes.Buffer{}
		if err = tmpl.Execute(out, info); err != nil {
			return "", fmt.Errorf("please specify a supported field:\n%s", infoUsage())
		}
		return out.String(), nil
	}
	return "", nil
}

func runEnv(jirix *jiri.X, args []string) error {
	if len(profilesFlag) == 0 {
		return fmt.Errorf("no profiles were specified using --profiles")
	}
	ch, err := profiles.NewConfigHelper(jirix, profiles.UseProfiles, manifestFlag)
	if err != nil {
		return err
	}
	profileNames := strings.Split(profilesFlag, ",")
	if err := ch.ValidateRequestedProfilesAndTarget(profileNames, targetFlag); err != nil {
		return err
	}
	ch.MergeEnvFromProfiles(mergePoliciesFlag, targetFlag, profileNames...)
	out := fmtVars(ch.ToMap(), args)
	if len(out) > 0 {
		fmt.Fprintln(jirix.Stdout(), out)
	}
	return nil
}

func expr(k, v string, trimmed bool) string {
	if trimmed {
		return v
	}
	return fmt.Sprintf("%s=%q ", k, v)
}

func fmtVars(vars map[string]string, args []string) string {
	buf := bytes.Buffer{}
	if len(args) == 0 {
		for k, v := range vars {
			buf.WriteString(fmt.Sprintf("%s=%q ", k, v))
		}
	} else {
		for _, arg := range args {
			name := strings.TrimSuffix(arg, "=")
			trimmed := name != arg
			for k, v := range vars {
				if k == name {
					buf.WriteString(expr(k, v, trimmed))
				}
			}
		}
	}
	return strings.TrimSuffix(buf.String(), " ")
}

func initCommand(jirix *jiri.X, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no profiles specified")
	}
	for _, n := range args {
		if mgr := profiles.LookupManager(n); mgr == nil {
			return fmt.Errorf("profile %v is not available, use \"list --available\" to see the list of available profiles", n)
		}
	}
	if err := profiles.Read(jirix, manifestFlag); err != nil {
		fmt.Fprintf(jirix.Stderr(), "Failed to read manifest: %v", err)
		return err
	}
	return nil
}

func runUpdate(jirix *jiri.X, args []string) error {
	if len(args) == 0 {
		args = profiles.Managers()
	}
	if err := initCommand(jirix, args); err != nil {
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
			if vi.IsTargetOlderThanDefault(target.Version()) {
				if verboseFlag {
					fmt.Fprintf(jirix.Stdout(), "Updating %s %s from %q to %s\n", n, target, target.Version(), vi)
				}
				target.SetVersion(vi.Default())
				err := mgr.Install(jirix, rootPath, *target)
				logResult(jirix, "Update", mgr, *target, err)
				if err != nil {
					return err
				}
			} else {
				if verboseFlag {
					fmt.Fprintf(jirix.Stdout(), "%s %s at %q is up to date(%s)\n", n, target, target.Version(), vi)
				}
			}

		}
	}
	return profiles.Write(jirix, manifestFlag)
}

func runGC(jirix *jiri.X, args []string) error {
	for _, n := range args {
		mgr := profiles.LookupManager(n)
		vi := mgr.VersionInfo()
		profile := profiles.LookupProfile(n)
		for _, target := range profile.Targets() {
			if vi.IsTargetOlderThanDefault(target.Version()) {
				err := mgr.Uninstall(jirix, rootPath, *target)
				logResult(jirix, "gc", mgr, *target, err)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func runEnsureVersionsAreSet(jirix *jiri.X, args []string) error {
	for _, name := range args {
		profile := profiles.LookupProfile(name)
		mgr := profiles.LookupManager(name)
		if mgr == nil {
			fmt.Fprintf(jirix.Stderr(), "%s is not linked into this binary", name)
			continue
		}
		for _, target := range profile.Targets() {
			if len(target.Version()) == 0 {
				prior := *target
				version, err := mgr.VersionInfo().Select(target.Version())
				if err != nil {
					return err
				}
				target.SetVersion(version)
				profiles.RemoveProfileTarget(name, prior)
				if err := profiles.AddProfileTarget(name, *target); err != nil {
					return err
				}
				if verboseFlag {
					fmt.Fprintf(jirix.Stdout(), "%s %s had no version, now set to: %s\n", name, prior, target)
				}
			}
		}
	}
	return nil
}

func runRmAll(jirix *jiri.X) error {
	s := jirix.NewSeq()
	if exists, err := s.FileExists(manifestFlag); err != nil || exists {
		if err := s.Remove(manifestFlag).Done(); err != nil {
			return err
		}
	}
	rp := rootPath.Expand()
	if exists, err := s.DirectoryExists(rp); err != nil || exists {
		if err := s.Run("chmod", "-R", "u+w", rp).
			RemoveAll(rp).Done(); err != nil {
			return err
		}
	}
	return nil
}

func runCleanup(jirix *jiri.X, args []string) error {
	if err := profiles.Read(jirix, manifestFlag); err != nil {
		fmt.Fprintf(jirix.Stderr(), "Failed to read manifest: %v", err)
		return err
	}
	if len(args) == 0 {
		args = profiles.Profiles()
	}
	dirty := false
	if specificVersionsFlag {
		if verboseFlag {
			fmt.Fprintf(jirix.Stdout(), "Ensuring that all targets have a specific version set for %s\n", args)
		}
		if err := runEnsureVersionsAreSet(jirix, args); err != nil {
			return fmt.Errorf("ensure-specific-versions-are-set: %v", err)
		}
		dirty = true
	}
	if cleanupFlag {
		if verboseFlag {
			fmt.Fprintf(jirix.Stdout(), "Removing targets older than the default version for %s\n", args)
		}
		if err := runGC(jirix, args); err != nil {
			return fmt.Errorf("gc: %v", err)
		}
		dirty = true
	}
	if rmAllFlag {
		if verboseFlag {
			fmt.Fprintf(jirix.Stdout(), "Removing profile manifest and all profile output files\n")
		}
		if err := runRmAll(jirix); err != nil {
			return err
		}
		// Don't write out the profiles manifest file again.
		return nil
	}
	if rewriteManifestFlag {
		dirty = true
	}
	if !dirty {
		return fmt.Errorf("at least one option must be specified")
	}
	return profiles.Write(jirix, manifestFlag)
}

func logResult(jirix *jiri.X, action string, mgr profiles.Manager, target profiles.Target, err error) {
	fmt.Fprintf(jirix.Stdout(), "%s: %s %s: ", action, mgr.Name(), target)
	if err == nil {
		fmt.Fprintf(jirix.Stdout(), "success\n")
	} else {
		fmt.Fprintf(jirix.Stdout(), "%v\n", err)
	}
}

func applyCommand(names []string, jirix *jiri.X, target profiles.Target, fn func(profiles.Manager, *jiri.X, profiles.Target) error) error {
	for _, n := range names {
		mgr := profiles.LookupManager(n)
		version, err := mgr.VersionInfo().Select(target.Version())
		if err != nil {
			return err
		}
		target.SetVersion(version)
		if err := fn(mgr, jirix, target); err != nil {
			return err
		}
	}
	return nil
}

func runInstall(jirix *jiri.X, args []string) error {
	if err := initCommand(jirix, args); err != nil {
		return err
	}
	names := []string{}
	if len(args) == 0 {
		for _, name := range profiles.Managers() {
			names = append(names, name)
		}
	}
	targetFlag.UseCommandLineEnv()
	for _, name := range args {
		if p := profiles.LookupProfileTarget(name, targetFlag); p != nil {
			fmt.Fprintf(jirix.Stdout(), "%v %v is already installed as %v\n", name, targetFlag, p)
			continue
		}
		names = append(names, name)
	}
	if err := applyCommand(names, jirix, targetFlag,
		func(mgr profiles.Manager, jirix *jiri.X, target profiles.Target) error {
			err := mgr.Install(jirix, rootPath, target)
			logResult(jirix, "Install:", mgr, target, err)
			return err
		}); err != nil {
		return err
	}
	return profiles.Write(jirix, manifestFlag)
}

func runUninstall(jirix *jiri.X, args []string) error {
	if err := initCommand(jirix, args); err != nil {
		return err
	}
	if allFlag && targetFlag.IsSet() {
		fmt.Fprintf(jirix.Stdout(), "ignore target (%v) when used in conjunction with --all-targets\n", targetFlag)
	}
	if allFlag {
		for _, name := range args {
			profile := profiles.LookupProfile(name)
			mgr := profiles.LookupManager(name)
			if profile == nil || mgr == nil {
				continue
			}
			for _, target := range profile.Targets() {
				if err := mgr.Uninstall(jirix, rootPath, *target); err != nil {
					logResult(jirix, "Uninstall", mgr, *target, err)
					return err
				}
				logResult(jirix, "Uninstall", mgr, *target, nil)
			}
		}
	} else {
		applyCommand(args, jirix, targetFlag,
			func(mgr profiles.Manager, jirix *jiri.X, target profiles.Target) error {
				err := mgr.Uninstall(jirix, rootPath, target)
				logResult(jirix, "Uninstall", mgr, target, err)
				return err
			})
	}
	return profiles.Write(jirix, manifestFlag)
}
