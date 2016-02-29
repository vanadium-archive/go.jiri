// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package profilescmdline provides a command line driver
// (for v.io/x/lib/cmdline) for implementing jiri 'profile' subcommands.
// The intent is to support project specific instances of such profiles
// for managing software dependencies.
//
// There are two ways of using the cmdline support, one is to read profile
// information, via RegisterReaderCommands and
// RegisterReaderCommandsUsingParent; the other is to manage profile
// installations via the RegisterManagementCommands function. The management
// commands can manage profiles that are linked into the binary itself
// or invoke external commands that implement the profile management. These
// external 'installer' commands are accessed by specifing them as a prefix
// to the profile name. For example myproject::go will invoke the external
// command jiri-profile-myproject with "go" as the profile name. Thus the
// following invocations are equivalent:
// jiri profile install myproject::go
// jiri profile-myproject install go
//
// Regardless of which is used, the profile name, as seen by profile
// database readers will be myproject::go.
package profilescmdline

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/x/lib/cmdline"
)

// newCmdInstall represents the "profile install" command.
func newCmdInstall() *cmdline.Command {
	return &cmdline.Command{
		Runner:   jiri.RunnerFunc(runInstall),
		Name:     "install",
		Short:    "Install the given profiles",
		Long:     "Install the given profiles.",
		ArgsName: "<profiles>",
		ArgsLong: "<profiles> is a list of profiles to install.",
	}
}

// newCmdUninstall represents the "profile uninstall" command.
func newCmdUninstall() *cmdline.Command {
	return &cmdline.Command{
		Runner:   jiri.RunnerFunc(runUninstall),
		Name:     "uninstall",
		Short:    "Uninstall the given profiles",
		Long:     "Uninstall the given profiles.",
		ArgsName: "<profiles>",
		ArgsLong: "<profiles> is a list of profiles to uninstall.",
	}
}

// newCmdUpdate represents the "profile update" command.
func newCmdUpdate() *cmdline.Command {
	return &cmdline.Command{
		Runner:   jiri.RunnerFunc(runUpdate),
		Name:     "update",
		Short:    "Install the latest default version of the given profiles",
		Long:     "Install the latest default version of the given profiles.",
		ArgsName: "<profiles>",
		ArgsLong: "<profiles> is a list of profiles to update, if omitted all profiles are updated.",
	}
}

// newCmdCleanup represents the "profile cleanup" command.
func newCmdCleanup() *cmdline.Command {
	return &cmdline.Command{
		Runner:   jiri.RunnerFunc(runCleanup),
		Name:     "cleanup",
		Short:    "Cleanup the locally installed profiles",
		Long:     "Cleanup the locally installed profiles. This is generally required when recovering from earlier bugs or when preparing for a subsequent change to the profiles implementation.",
		ArgsName: "<profiles>",
		ArgsLong: "<profiles> is a list of profiles to cleanup, if omitted all profiles are cleaned.",
	}
}

// newCmdAvailable represents the "profile available" command.
func newCmdAvailable() *cmdline.Command {
	return &cmdline.Command{
		Runner: jiri.RunnerFunc(runAvailable),
		Name:   "available",
		Short:  "List the available profiles",
		Long:   "List the available profiles.",
	}
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

func runAvailable(jirix *jiri.X, args []string) error {
	return availableImpl(jirix, &availableFlags, args)
}

type commonFlagValues struct {
	// The value of --profiles-db
	dbPath string
	// The value of --profiles-dir
	root string
}

func initCommon(flags *flag.FlagSet, c *commonFlagValues, installer, defaultDBPath, defaultProfilesPath string) {
	RegisterDBPathFlag(flags, &c.dbPath, defaultDBPath)
	flags.StringVar(&c.root, "profiles-dir", defaultProfilesPath, "the directory, relative to JIRI_ROOT, that profiles are installed in")
}

func (cv *commonFlagValues) args() []string {
	a := append([]string{}, "--profiles-db="+cv.dbPath)
	a = append(a, "--profiles-dir="+cv.root)
	return a
}

type installFlagValues struct {
	commonFlagValues
	// The value of --target and --env
	target profiles.Target
	// The value of --force
	force bool
}

func initInstallCommand(flags *flag.FlagSet, installer, defaultDBPath, defaultProfilesPath string) {
	initCommon(flags, &installFlags.commonFlagValues, installer, defaultDBPath, defaultProfilesPath)
	profiles.RegisterTargetAndEnvFlags(flags, &installFlags.target)
	flags.BoolVar(&installFlags.force, "force", false, "force install the profile even if it is already installed")
	for _, name := range profilesmanager.Managers() {
		profilesmanager.LookupManager(name).AddFlags(flags, profiles.Install)
	}
}

func (iv *installFlagValues) args() []string {
	a := iv.commonFlagValues.args()
	if t := iv.target.String(); t != "" {
		a = append(a, "--target="+t)
	}
	if e := iv.target.CommandLineEnv().String(); e != "" {
		a = append(a, "--target="+e)
	}
	return append(a, fmt.Sprintf("--%s=%v", "force", iv.force))
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

func initUninstallCommand(flags *flag.FlagSet, installer, defaultDBPath, defaultProfilesPath string) {
	initCommon(flags, &uninstallFlags.commonFlagValues, installer, defaultDBPath, defaultProfilesPath)
	profiles.RegisterTargetFlag(flags, &uninstallFlags.target)
	flags.BoolVar(&uninstallFlags.allTargets, "all-targets", false, "apply to all targets for the specified profile(s)")
	flags.BoolVar(&uninstallFlags.verbose, "v", false, "print more detailed information")
	for _, name := range profilesmanager.Managers() {
		profilesmanager.LookupManager(name).AddFlags(flags, profiles.Uninstall)
	}
}

func (uv *uninstallFlagValues) args() []string {
	a := uv.commonFlagValues.args()
	if uv.target.String() != "" {
		a = append(a, "--target="+uv.target.String())
	}
	a = append(a, fmt.Sprintf("--%s=%v", "all-targets", uv.allTargets))
	return append(a, fmt.Sprintf("--%s=%v", "v", uv.verbose))
}

type cleanupFlagValues struct {
	commonFlagValues
	// The value of --gc
	gc bool
	// The value of --rewrite-profiles-db
	rewriteDB bool
	// The value of --rm-all
	rmAll bool
	// The value of --v
	verbose bool
}

func initCleanupCommand(flags *flag.FlagSet, installer, defaultDBPath, defaultProfilesPath string) {
	initCommon(flags, &cleanupFlags.commonFlagValues, installer, defaultDBPath, defaultProfilesPath)
	flags.BoolVar(&cleanupFlags.gc, "gc", false, "uninstall profile targets that are older than the current default")
	flags.BoolVar(&cleanupFlags.rmAll, "rm-all", false, "remove profiles database and all profile generated output files.")
	flags.BoolVar(&cleanupFlags.rewriteDB, "rewrite-profiles-db", false, "rewrite the profiles database to use the latest schema version")
	flags.BoolVar(&cleanupFlags.verbose, "v", false, "print more detailed information")
}

func (cv *cleanupFlagValues) args() []string {
	return append(cv.commonFlagValues.args(),
		fmt.Sprintf("--%s=%v", "gc", cv.gc),
		fmt.Sprintf("--%s=%v", "rewrite-profiles-db", cv.rewriteDB),
		fmt.Sprintf("--%s=%v", "v", cv.verbose),
		fmt.Sprintf("--%s=%v", "rm-all", cv.rmAll))
}

type updateFlagValues struct {
	commonFlagValues
	// The value of --v
	verbose bool
}

func initUpdateCommand(flags *flag.FlagSet, installer, defaultDBPath, defaultProfilesPath string) {
	initCommon(flags, &updateFlags.commonFlagValues, installer, defaultDBPath, defaultProfilesPath)
	flags.BoolVar(&updateFlags.verbose, "v", false, "print more detailed information")
}

func (uv *updateFlagValues) args() []string {
	return append(uv.commonFlagValues.args(), fmt.Sprintf("--%s=%v", "v", uv.verbose))
}

type availableFlagValues struct {
	// The value of --v
	verbose bool
	// The value of --describe
	describe bool
}

func initAvailableCommand(flags *flag.FlagSet, installer, defaultDBPath, defaultProfilesPath string) {
	flags.BoolVar(&availableFlags.verbose, "v", false, "print more detailed information")
	flags.BoolVar(&availableFlags.describe, "describe", false, "print the profile description")
}

func (av *availableFlagValues) args() []string {
	return []string{
		fmt.Sprintf("--%s=%v", "v", av.verbose),
		fmt.Sprintf("--%s=%v", "describe", av.describe),
	}
}

var (
	installFlags     installFlagValues
	uninstallFlags   uninstallFlagValues
	cleanupFlags     cleanupFlagValues
	updateFlags      updateFlagValues
	availableFlags   availableFlagValues
	profileInstaller string
	runSubcommands   bool
)

// RegisterManagementCommands registers the management subcommands:
// uninstall, install, update and cleanup.
func RegisterManagementCommands(parent *cmdline.Command, useSubcommands bool, installer, defaultDBPath, defaultProfilesPath string) {
	cmdInstall := newCmdInstall()
	cmdUninstall := newCmdUninstall()
	cmdUpdate := newCmdUpdate()
	cmdCleanup := newCmdCleanup()
	cmdAvailable := newCmdAvailable()
	initInstallCommand(&cmdInstall.Flags, installer, defaultDBPath, defaultProfilesPath)
	initUninstallCommand(&cmdUninstall.Flags, installer, defaultDBPath, defaultProfilesPath)
	initUpdateCommand(&cmdUpdate.Flags, installer, defaultDBPath, defaultProfilesPath)
	initCleanupCommand(&cmdCleanup.Flags, installer, defaultDBPath, defaultProfilesPath)
	initAvailableCommand(&cmdAvailable.Flags, installer, defaultDBPath, defaultProfilesPath)
	parent.Children = append(parent.Children, cmdInstall, cmdUninstall, cmdUpdate, cmdCleanup, cmdAvailable)
	profileInstaller = installer
	runSubcommands = useSubcommands
}

func findProfileSubcommands(jirix *jiri.X) []string {
	if !runSubcommands {
		return []string{}
	}
	fi, err := os.Stat(filepath.Join(jirix.Root, jiri.ProfilesDBDir))
	if err == nil && fi.IsDir() {
		env := cmdline.EnvFromOS()
		env.Vars["PATH"] = jirix.Env()["PATH"]
		return env.LookPathPrefix("jiri-profile", map[string]bool{})
	}
	return []string{}
}

func allAvailableManagers(jirix *jiri.X) ([]string, error) {
	names := profilesmanager.Managers()
	if profileInstaller != "" {
		return names, nil
	}
	subcommands := findProfileSubcommands(jirix)
	s := jirix.NewSeq()
	for _, sc := range subcommands {
		var out bytes.Buffer
		args := []string{"available"}
		if err := s.Capture(&out, nil).Last(sc, args...); err != nil {
			fmt.Fprintf(jirix.Stderr(), "failed to run %s %s: %v", sc, strings.Join(args, " "), err)
			return nil, err
		}
		mgrs := out.String()
		for _, m := range strings.Split(mgrs, ",") {
			names = append(names, strings.TrimSpace(m))
		}
	}
	return names, nil
}

// availableProfileManagers creates a profileManager for all available
// profiles, whether in this process or in a sub command.
func availableProfileManagers(jirix *jiri.X, dbpath string, args []string) ([]profileManager, *profiles.DB, error) {
	db := profiles.NewDB()
	if err := db.Read(jirix, dbpath); err != nil {
		fmt.Fprintf(jirix.Stderr(), "Failed to read profiles database %q: %v\n", dbpath, err)
		return nil, nil, err
	}
	mgrs := []profileManager{}
	names := args
	if len(names) == 0 {
		var err error
		names, err = allAvailableManagers(jirix)
		if err != nil {
			return nil, nil, err
		}
	}
	for _, name := range names {
		mgrs = append(mgrs, newProfileManager(name, db))
	}
	return mgrs, db, nil
}

// installedProfileManagers creates a profileManager for all installed
// profiles, whether in this process or in a sub command.
func installedProfileManagers(jirix *jiri.X, dbpath string, args []string) ([]profileManager, *profiles.DB, error) {
	db := profiles.NewDB()
	if err := db.Read(jirix, dbpath); err != nil {
		fmt.Fprintf(jirix.Stderr(), "Failed to read profiles database %q: %v\n", dbpath, err)
		return nil, nil, err
	}
	mgrs := []profileManager{}
	names := args
	if len(names) == 0 {
		names = db.Names()
	}
	for _, name := range names {
		mgrs = append(mgrs, newProfileManager(name, db))
	}
	return mgrs, db, nil
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

func writeDB(jirix *jiri.X, db *profiles.DB, installer, path string) error {
	// If path is a directory and installer is empty, then do nothing,
	// otherwise write out the file.
	isdir := false
	fi, err := os.Stat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		isdir = fi.IsDir()
	}
	if isdir {
		if installer != "" {
			// New setup with installers writing their own file in a directory
			return db.Write(jirix, installer, path)
		} else {
			// New setup, no installer, so don't write out the file.
			return nil
		}
	}
	if installer == "" {
		// Old setup with no installers and writing to a file.
		return db.Write(jirix, installer, path)
	}
	// New setup, but the directory doesn't exist yet.
	if err := os.MkdirAll(path, os.FileMode(0755)); err != nil {
		return err
	}
	return db.Write(jirix, installer, path)
}

func updateImpl(jirix *jiri.X, cl *updateFlagValues, args []string) error {
	mgrs, db, err := availableProfileManagers(jirix, cl.dbPath, args)
	if err != nil {
		return err
	}
	root := jiri.NewRelPath(cl.root).Join(profileInstaller)
	for _, mgr := range mgrs {
		if err := mgr.update(jirix, cl, root); err != nil {
			return err
		}
	}
	return writeDB(jirix, db, profileInstaller, cl.dbPath)
}

func cleanupImpl(jirix *jiri.X, cl *cleanupFlagValues, args []string) error {
	count := 0
	if cl.gc {
		count++
	}
	if cl.rewriteDB {
		count++
	}
	if cl.rmAll {
		count++
	}
	if count != 1 {
		fmt.Errorf("exactly one option must be specified")
	}
	mgrs, db, err := installedProfileManagers(jirix, cl.dbPath, args)
	if err != nil {
		return err
	}
	root := jiri.NewRelPath(cl.root).Join(profileInstaller)
	for _, mgr := range mgrs {
		if err := mgr.cleanup(jirix, cl, root); err != nil {
			return err
		}
	}
	if !cl.rmAll {
		return writeDB(jirix, db, profileInstaller, cl.dbPath)
	}
	return nil
}

func installImpl(jirix *jiri.X, cl *installFlagValues, args []string) error {
	mgrs, db, err := availableProfileManagers(jirix, cl.dbPath, args)
	if err != nil {
		return err
	}
	cl.target.UseCommandLineEnv()
	newMgrs := []profileManager{}
	for _, mgr := range mgrs {
		name := mgr.mgrName()
		if !cl.force {
			installer, profile := profiles.SplitProfileName(name)
			if p := db.LookupProfileTarget(installer, profile, cl.target); p != nil {
				fmt.Fprintf(jirix.Stdout(), "%v %v is already installed as %v\n", name, cl.target, p)
				continue
			}
		}
		newMgrs = append(newMgrs, mgr)
	}
	root := jiri.NewRelPath(cl.root).Join(profileInstaller)
	for _, mgr := range newMgrs {
		if err := mgr.install(jirix, cl, root); err != nil {
			return err
		}
	}
	return writeDB(jirix, db, profileInstaller, cl.dbPath)
}

func uninstallImpl(jirix *jiri.X, cl *uninstallFlagValues, args []string) error {
	mgrs, db, err := availableProfileManagers(jirix, cl.dbPath, args)
	if err != nil {
		return err
	}
	if cl.allTargets && cl.target.IsSet() {
		fmt.Fprintf(jirix.Stdout(), "ignore target (%v) when used in conjunction with --all-targets\n", cl.target)
	}
	root := jiri.NewRelPath(cl.root).Join(profileInstaller)
	for _, mgr := range mgrs {
		if err := mgr.uninstall(jirix, cl, root); err != nil {
			return err
		}
	}
	return writeDB(jirix, db, profileInstaller, cl.dbPath)
}

func availableImpl(jirix *jiri.X, cl *availableFlagValues, _ []string) error {
	if profileInstaller == "" {
		subcommands := findProfileSubcommands(jirix)
		if cl.verbose {
			fmt.Fprintf(jirix.Stdout(), "Available Subcommands: %s\n", strings.Join(subcommands, ", "))
		}
		s := jirix.NewSeq()
		args := []string{"available"}
		args = append(args, cl.args()...)
		out := bytes.Buffer{}
		for _, sc := range subcommands {
			if err := s.Capture(&out, nil).Last(sc, args...); err != nil {
				return err
			}
		}
		if s := strings.TrimSpace(out.String()); s != "" {
			fmt.Fprintln(jirix.Stdout(), s)
		}
	}
	mgrs := profilesmanager.Managers()
	if len(mgrs) == 0 {
		return nil
	}
	if cl.verbose {
		scname := ""
		if profileInstaller != "" {
			scname = profileInstaller + ": "
		}
		fmt.Fprintf(jirix.Stdout(), "%sAvailable Profiles:\n", scname)
		for _, name := range mgrs {
			mgr := profilesmanager.LookupManager(name)
			vi := mgr.VersionInfo()
			fmt.Fprintf(jirix.Stdout(), "%s: versions: %s\n", name, vi)
		}
	} else {
		if cl.describe {
			for _, name := range mgrs {
				mgr := profilesmanager.LookupManager(name)
				fmt.Fprintf(jirix.Stdout(), "%s: %s\n", name, strings.Replace(strings.TrimSpace(mgr.Info()), "\n", " ", -1))
			}
		} else {
			fmt.Fprintf(jirix.Stdout(), "%s\n", strings.Join(mgrs, ", "))
		}
	}
	return nil
}
