// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package commandline provides a command line driver (for v.io/x/lib/cmdline)
// for implementing jiri 'profile' subcommands. The intent is to support
// project specific instances of such profiles for managing software
// dependencies.
package commandline

import (
	"flag"
	"fmt"
	"strings"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/manager"
	"v.io/x/lib/cmdline"
)

// cmdInstall represents the "profile install" command.
var cmdInstall = &cmdline.Command{
	Runner:   jiri.RunnerFunc(runInstall),
	Name:     "install",
	Short:    "Install the given profiles",
	Long:     "Install the given profiles.",
	ArgsName: "<profiles>",
	ArgsLong: "<profiles> is a list of profiles to install.",
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

// cmdCleanup represents the "profile cleanup" command.
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

func runUpdate(jirix *jiri.X, args []string) error {
	return updateImpl(jirix, &updateFlags, args)
}

func runCleanup(jirix *jiri.X, args []string) error {
	return cleanupImpl(jirix, &cleanupFlags, args)
}

func runInstall(jirix *jiri.X, args []string) error {
	return installImpl(jirix, &installFlags, args)
}

func runUninstall(jirix *jiri.X, args []string) error {
	return uninstallImpl(jirix, &uninstallFlags, args)
}

type commonFlagValues struct {
	// The value of --profiles-db
	dbFilename string
	// The value of --profile-root
	root string
}

type installFlagValues struct {
	commonFlagValues
	// The value of --target and --env
	target profiles.Target
	// The value of --force
	force bool
}

type uninstallFlagValues struct {
	commonFlagValues
	// The value of --target
	target profiles.Target
	// The value of --all-targets
	allTargets bool
	// The value of --v
	verbose bool
	// TODO(cnicolaou): add a flag to remove the profile only from the DB.
}

type updateFlagValues struct {
	commonFlagValues
	// The value of --v
	verbose bool
}

type cleanupFlagValues struct {
	commonFlagValues
	// The value of --gc
	gc bool
	// The value of --ensure-specific-versions-are-set
	ensureSpecificVersions bool
	// The value of --rewrite-profiles-manifest
	rewriteManifest bool
	// The value of --rm-all
	rmAll bool
	// The value of --v
	verbose bool
}

var (
	installFlags   installFlagValues
	uninstallFlags uninstallFlagValues
	cleanupFlags   cleanupFlagValues
	updateFlags    updateFlagValues
)

// RegisterManagementCommands registers the management subcommands:
// uninstall, install, update and cleanup.
//
func RegisterManagementCommands(parent *cmdline.Command, defaultDBFilename string) {
	initInstallCommand(&cmdInstall.Flags, defaultDBFilename)
	initUninstallCommand(&cmdUninstall.Flags, defaultDBFilename)
	initUpdateCommand(&cmdUpdate.Flags, defaultDBFilename)
	initCleanupCommand(&cmdCleanup.Flags, defaultDBFilename)
	parent.Children = append(parent.Children, cmdInstall, cmdUninstall, cmdUpdate, cmdCleanup)
}

func initCommon(flags *flag.FlagSet, c *commonFlagValues, defaultDBFilename string) {
	RegisterDBFilenameFlag(flags, &c.dbFilename, defaultDBFilename)
	flags.StringVar(&c.root, "profile-dir", "profiles", "the directory, relative to JIRI_ROOT, that profiles are installed in")
}

func initInstallCommand(flags *flag.FlagSet, defaultDBFilename string) {
	initCommon(flags, &installFlags.commonFlagValues, defaultDBFilename)
	profiles.RegisterTargetAndEnvFlags(flags, &installFlags.target)
	flags.BoolVar(&installFlags.force, "force", false, "force install the profile even if it is already installed")
}

func initUninstallCommand(flags *flag.FlagSet, defaultDBFilename string) {
	initCommon(flags, &uninstallFlags.commonFlagValues, defaultDBFilename)
	profiles.RegisterTargetFlag(flags, &uninstallFlags.target)
	flags.BoolVar(&uninstallFlags.allTargets, "all-targets", false, "apply to all targets for the specified profile(s)")
	flags.BoolVar(&uninstallFlags.verbose, "v", false, "print more detailed information")
}

func initUpdateCommand(flags *flag.FlagSet, defaultDBFilename string) {
	initCommon(flags, &updateFlags.commonFlagValues, defaultDBFilename)
	flags.BoolVar(&updateFlags.verbose, "v", false, "print more detailed information")
}

func initCleanupCommand(flags *flag.FlagSet, defaultDBFilename string) {
	initCommon(flags, &cleanupFlags.commonFlagValues, defaultDBFilename)
	flags.BoolVar(&cleanupFlags.gc, "gc", false, "uninstall profile targets that are older than the current default")
	flags.BoolVar(&cleanupFlags.ensureSpecificVersions, "ensure-specific-versions-are-set", false, "ensure that profile targets have a specific version set")
	flags.BoolVar(&cleanupFlags.rmAll, "rm-all", false, "remove profiles manifest and all profile generated output files.")
	flags.BoolVar(&cleanupFlags.rewriteManifest, "rewrite-profiles-manifest", false, "rewrite the profiles manifest file to use the latest schema version")
	flags.BoolVar(&cleanupFlags.verbose, "v", false, "print more detailed information")
}

// init a command that takes a list of profile managers as its arguments.
func initProfileManagersCommand(jirix *jiri.X, dbfile string, args []string) ([]string, *profiles.DB, error) {
	if len(args) == 0 {
		args = manager.Managers()
	} else {
		for _, n := range args {
			if mgr := manager.LookupManager(n); mgr == nil {
				avail := manager.Managers()
				return nil, nil, fmt.Errorf("profile %v is not one of the available ones: %s", n, strings.Join(avail, ", "))
			}
		}
	}
	db := profiles.NewDB()
	if err := db.Read(jirix, dbfile); err != nil {
		fmt.Fprintf(jirix.Stderr(), "Failed to read profiles database %q: %v", dbfile, err)
		return nil, nil, err
	}
	return args, db, nil
}

// init a command that takes a list of profiles (already installed) as
// its arguments.
func initProfilesCommand(jirix *jiri.X, dbfile string, args []string) ([]string, *profiles.DB, error) {
	db := profiles.NewDB()
	if err := db.Read(jirix, dbfile); err != nil {
		fmt.Fprintf(jirix.Stderr(), "Failed to read profiles database %q: %v", dbfile, err)
		return nil, nil, err
	}
	if len(args) == 0 {
		args = db.Names()
	} else {
		for _, n := range args {
			if p := db.LookupProfile(n); p == nil {
				avail := db.Names()
				return nil, nil, fmt.Errorf("profile %v is not one of the installed ones: %s", n, strings.Join(avail, ", "))
			}
		}
	}
	return args, db, nil
}

func updateImpl(jirix *jiri.X, cl *updateFlagValues, args []string) error {
	verbose := cl.verbose
	args, db, err := initProfileManagersCommand(jirix, cl.dbFilename, args)
	if err != nil {
		return err
	}
	for _, n := range args {
		mgr := manager.LookupManager(n)
		profile := db.LookupProfile(n)
		if profile == nil {
			continue
		}
		vi := mgr.VersionInfo()
		for _, target := range profile.Targets() {
			if vi.IsTargetOlderThanDefault(target.Version()) {
				if verbose {
					fmt.Fprintf(jirix.Stdout(), "Updating %s %s from %q to %s\n", n, target, target.Version(), vi)
				}
				target.SetVersion(vi.Default())
				err := mgr.Install(jirix, db, jiri.NewRelPath(cl.root), *target)
				logResult(jirix, "Update", mgr, *target, err)
				if err != nil {
					return err
				}
			} else {
				if verbose {
					fmt.Fprintf(jirix.Stdout(), "%s %s at %q is up to date(%s)\n", n, target, target.Version(), vi)
				}
			}
		}
	}
	return db.Write(jirix, cl.dbFilename)
}

func cleanupGC(jirix *jiri.X, db *profiles.DB, root jiri.RelPath, verbose bool, args []string) error {
	for _, name := range args {
		mgr := manager.LookupManager(name)
		if mgr == nil {
			fmt.Fprintf(jirix.Stderr(), "%s is not linked into this binary", name)
			continue
		}
		vi := mgr.VersionInfo()
		profile := db.LookupProfile(name)
		for _, target := range profile.Targets() {
			if vi.IsTargetOlderThanDefault(target.Version()) {
				err := mgr.Uninstall(jirix, db, root, *target)
				logResult(jirix, "gc", mgr, *target, err)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func cleanupEnsureVersionsAreSet(jirix *jiri.X, db *profiles.DB, root jiri.RelPath, verbose bool, args []string) error {
	for _, name := range args {
		mgr := manager.LookupManager(name)
		if mgr == nil {
			fmt.Fprintf(jirix.Stderr(), "%s is not linked into this binary", name)
			continue
		}
		profile := db.LookupProfile(name)
		for _, target := range profile.Targets() {
			if len(target.Version()) == 0 {
				prior := *target
				version, err := mgr.VersionInfo().Select(target.Version())
				if err != nil {
					return err
				}
				target.SetVersion(version)
				db.RemoveProfileTarget(name, prior)
				if err := db.AddProfileTarget(name, *target); err != nil {
					return err
				}
				if verbose {
					fmt.Fprintf(jirix.Stdout(), "%s %s had no version, now set to: %s\n", name, prior, target)
				}
			}
		}
	}
	return nil
}

func cleanupRmAll(jirix *jiri.X, db *profiles.DB, root jiri.RelPath, verbose bool, args []string) error {
	s := jirix.NewSeq()
	if exists, err := s.FileExists(db.Filename()); err != nil || exists {
		if err := s.Remove(db.Filename()).Done(); err != nil {
			return err
		}
	}
	d := root.Abs(jirix)
	if exists, err := s.DirectoryExists(d); err != nil || exists {
		if err := s.Run("chmod", "-R", "u+w", d).
			RemoveAll(d).Done(); err != nil {
			return err
		}
	}
	return nil
}

func cleanupImpl(jirix *jiri.X, cl *cleanupFlagValues, args []string) error {
	args, db, err := initProfilesCommand(jirix, cl.dbFilename, args)
	if err != nil {
		return err
	}
	verbose := cl.verbose
	root := jiri.NewRelPath(cl.root)
	dirty := false
	if cl.ensureSpecificVersions {
		if verbose {
			fmt.Fprintf(jirix.Stdout(), "Ensuring that all targets have a specific version set for %s\n", args)
		}
		if err := cleanupEnsureVersionsAreSet(jirix, db, root, verbose, args); err != nil {
			return fmt.Errorf("ensure-specific-versions-are-set: %v", err)
		}
		dirty = true
	}
	if cl.gc {
		if verbose {
			fmt.Fprintf(jirix.Stdout(), "Removing targets older than the default version for %s\n", args)
		}
		if err := cleanupGC(jirix, db, root, verbose, args); err != nil {
			return fmt.Errorf("gc: %v", err)
		}
		dirty = true
	}
	if cl.rmAll {
		if verbose {
			fmt.Fprintf(jirix.Stdout(), "Removing profile manifest and all profile output files\n")
		}
		if err := cleanupRmAll(jirix, db, root, verbose, args); err != nil {
			return err
		}
		// Don't write out the profiles manifest file again.
		return nil
	}
	if cl.rewriteManifest {
		dirty = true
	}
	if !dirty {
		return fmt.Errorf("at least one option must be specified")
	}
	return db.Write(jirix, cl.dbFilename)
}

func logResult(jirix *jiri.X, action string, mgr profiles.Manager, target profiles.Target, err error) {
	fmt.Fprintf(jirix.Stdout(), "%s: %s %s: ", action, mgr.Name(), target)
	if err == nil {
		fmt.Fprintf(jirix.Stdout(), "success\n")
	} else {
		fmt.Fprintf(jirix.Stdout(), "%v\n", err)
	}
}

func targetAtDefaultVersion(mgr profiles.Manager, target profiles.Target) (profiles.Target, error) {
	def := target
	version, err := mgr.VersionInfo().Select(target.Version())
	if err != nil {
		return profiles.Target{}, err
	}
	def.SetVersion(version)
	return def, nil
}

func installImpl(jirix *jiri.X, cl *installFlagValues, args []string) error {
	args, db, err := initProfileManagersCommand(jirix, cl.dbFilename, args)
	if err != nil {
		return err
	}
	root := jiri.NewRelPath(cl.root)
	cl.target.UseCommandLineEnv()
	names := []string{}
	if cl.force {
		names = args
	} else {
		for _, name := range args {
			if p := db.LookupProfileTarget(name, cl.target); p != nil {
				fmt.Fprintf(jirix.Stdout(), "%v %v is already installed as %v\n", name, cl.target, p)
				continue
			}
			names = append(names, name)
		}
	}
	for _, name := range names {
		mgr := manager.LookupManager(name)
		def, err := targetAtDefaultVersion(mgr, cl.target)
		if err != nil {
			return err
		}
		err = mgr.Install(jirix, db, root, def)
		logResult(jirix, "Install:", mgr, def, err)
		if err != nil {
			return err
		}
	}
	return db.Write(jirix, cl.dbFilename)
}

func uninstallImpl(jirix *jiri.X, cl *uninstallFlagValues, args []string) error {
	args, db, err := initProfileManagersCommand(jirix, cl.dbFilename, args)
	if err != nil {
		return err
	}
	root := jiri.NewRelPath(cl.root)
	if cl.allTargets && cl.target.IsSet() {
		fmt.Fprintf(jirix.Stdout(), "ignore target (%v) when used in conjunction with --all-targets\n", cl.target)
	}
	for _, name := range args {
		profile := db.LookupProfile(name)
		if profile == nil {
			continue
		}
		mgr := manager.LookupManager(name)
		var targets []*profiles.Target
		if cl.allTargets {
			targets = profile.Targets()
		} else {
			def, err := targetAtDefaultVersion(mgr, cl.target)
			if err != nil {
				return err
			}
			targets = []*profiles.Target{&def}
		}
		for _, target := range targets {
			if err := mgr.Uninstall(jirix, db, root, *target); err != nil {
				logResult(jirix, "Uninstall", mgr, *target, err)
				return err
			}
			logResult(jirix, "Uninstall", mgr, *target, nil)

		}
	}
	return db.Write(jirix, cl.dbFilename)
}
