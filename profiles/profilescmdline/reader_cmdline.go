// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package profilescmdline provides a command line driver (for v.io/x/lib/cmdline)
// for implementing jiri 'profile' subcommands. The intent is to support
// project specific instances of such profiles for managing software
// dependencies.
package profilescmdline

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/jiri/profiles/profilesreader"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/textutil"
)

// IsFlagSet returns true if the specified flag has been set on
// the command line.
func IsFlagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// NOTE: we use functions to initialize the commands so that we
// can reinitialize them in tests. cmd_test.go contains a 'Reset' function
// that is only available to tests for doing so.
// NOTE: we can't set cmdList.Runner in the initialization loop since runList
// needs to access cmdList.Flags.
var (
	// cmdList represents the "profile list" command.
	cmdList *cmdline.Command
	// cmdEnv represents the "profile env" command.
	cmdEnv *cmdline.Command = newCmdEnv()
)

func init() {
	cmdList = newCmdList()
	cmdList.Runner = jiri.RunnerFunc(runList)
}

func newCmdList() *cmdline.Command {
	return &cmdline.Command{
		Name:     "list",
		Short:    "List available or installed profiles",
		Long:     "List available or installed profiles.",
		ArgsName: "[<profiles>]",
		ArgsLong: "<profiles> is a list of profiles to list, defaulting to all profiles if none are specifically requested.",
	}
}

func newCmdEnv() *cmdline.Command {
	// cmdEnv represents the "profile env" command.
	return &cmdline.Command{
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
}

// ReaderFlagValues contains the values of the command line flags accepted
// required to configure and use the profiles/Reader package.
type ReaderFlagValues struct {
	// The value of --skip-profiles
	ProfilesMode profilesreader.ProfilesMode
	// The value of --profiles-db
	DBFilename string
	// The value of --profiles
	Profiles string
	// The value of --target and --env
	Target profiles.Target
	// The value of --merge-policies
	MergePolicies profilesreader.MergePolicies
	// The value of -v
	Verbose bool
}

// listFlagValues contains the flag values expected by the list subcommand
type listFlagValues struct {
	*ReaderFlagValues
	// The value of --show-profiles-db
	showProfilesDB bool
	// The value of --available
	available bool
	// The value of --info
	info string
}

// envFlagValues contains the flag values expected by the env subcommand
type envFlagValues struct {
	*ReaderFlagValues
}

// All flag values are stored in listFlags and envFlags.
var (
	listFlags listFlagValues
	envFlags  envFlagValues
)

// RegisterDBPathFlag registers the --profiles-db flag with the supplied FlagSet.
func RegisterDBPathFlag(flags *flag.FlagSet, manifest *string, defaultDBPath string) {
	root := jiri.FindRoot()
	flags.StringVar(manifest, "profiles-db", filepath.Join(root, defaultDBPath), "specify the profiles database directory or file.")
	flags.Lookup("profiles-db").DefValue = filepath.Join("$JIRI_ROOT", defaultDBPath)
}

// RegisterProfilesFlag registers the --profiles flag
func RegisterProfilesFlag(flags *flag.FlagSet, profiles *string) {
	// TODO(cnicolaou): delete this when the new profiles are in use.
	root := jiri.FindRoot()
	fi, err := os.Stat(filepath.Join(root, jiri.ProfilesDBDir))
	defaultProfiles := "base,jiri"
	if err == nil && fi.IsDir() {
		// TODO(cnicolaou): we need a better way of setting the default profiles,
		// ideally via the profiles db, or some other config. Provide a command
		// line tool for setting the default profiles.
		defaultProfiles = "v23:base,jiri"
	}
	flags.StringVar(profiles, "profiles", defaultProfiles, "a comma separated list of profiles to use")
}

// RegisterMergePoliciesFlag registers the --merge-policies flag
func RegisterMergePoliciesFlag(flags *flag.FlagSet, policies *profilesreader.MergePolicies) {
	flags.Var(policies, "merge-policies", "specify policies for merging environment variables")
}

// RegisterReaderFlags registers the 'reader' flags (see below)
// with the parent command. The values of the flags can be accessed via
// the supplied ReaderFlagValues struct.
// The reader flags are:
//  --skip-profiles
//  --profiles-db
//  --profiles
//  --merge-policies
//  --target and --env
func RegisterReaderFlags(flags *flag.FlagSet, fv *ReaderFlagValues, defaultDBLocation string) {
	flags.Var(&fv.ProfilesMode, "skip-profiles", "if set, no profiles will be used")
	RegisterDBPathFlag(flags, &fv.DBFilename, defaultDBLocation)
	RegisterProfilesFlag(flags, &fv.Profiles)
	fv.MergePolicies = profilesreader.JiriMergePolicies()
	RegisterMergePoliciesFlag(flags, &fv.MergePolicies)
	profiles.RegisterTargetAndEnvFlags(flags, &fv.Target)
}

func initializeReaderFlags(flags *flag.FlagSet, fv *ReaderFlagValues, defaultDBLocation string) {
	envFlags.ReaderFlagValues = fv
	listFlags.ReaderFlagValues = fv
	RegisterReaderFlags(flags, fv, defaultDBLocation)
}

// RegisterReaderCommandsUsingParent registers the 'reader' flags
// (see RegisterReaderFlags) with the parent command and creates the
// list and env subcommands. The values of the flags can be accessed via
// the supplied ReaderFlagValues struct.
// RegisterReaderCommandsUsingParent results in a command line of the form:
// <parent> <reader-flags> [list|env] <list/env specific commands>
func RegisterReaderCommandsUsingParent(parent *cmdline.Command, fv *ReaderFlagValues, defaultDBLocation string) {
	initializeReaderFlags(&parent.Flags, fv, defaultDBLocation)
	RegisterReaderCommands(parent, defaultDBLocation)
}

// RegisterReaderCommands registers the list and env subcommands. The
// subcommands will host the 'reader' flags (see RegisterReaderFlags)
// resulting in a command line of the form:
// <parent> [list|env] <reader-flags> <list/env specific specific commands>
func RegisterReaderCommands(parent *cmdline.Command, defaultDBLocation string) {
	registerListCommand(parent, defaultDBLocation)
	registerEnvCommand(parent, defaultDBLocation)
}

func newReaderFlags() *ReaderFlagValues {
	return &ReaderFlagValues{MergePolicies: profilesreader.JiriMergePolicies()}
}

// registerListCommand the profiles list subcommand and returns it
// and a struct containing  the values of the command line flags.
func registerListCommand(parent *cmdline.Command, defaultDBLocation string) {
	parent.Children = append(parent.Children, cmdList)
	if listFlags.ReaderFlagValues == nil {
		listFlags.ReaderFlagValues = newReaderFlags()
		RegisterReaderFlags(&cmdList.Flags, listFlags.ReaderFlagValues, defaultDBLocation)
	}
	cmdList.Flags.BoolVar(&listFlags.Verbose, "v", false, "print more detailed information")
	cmdList.Flags.BoolVar(&listFlags.showProfilesDB, "show-profiles-db", false, "print out the profiles database file")
	cmdList.Flags.BoolVar(&listFlags.available, "available", false, "print the list of available profiles")
	cmdList.Flags.StringVar(&listFlags.info, "info", "", infoUsage())
}

// registerEnvCommand the profiles env subcommand and returns it and a
// struct containing the values of the command line flags.
func registerEnvCommand(parent *cmdline.Command, defaultDBLocation string) {
	parent.Children = append(parent.Children, cmdEnv)
	if envFlags.ReaderFlagValues == nil {
		envFlags.ReaderFlagValues = newReaderFlags()
		RegisterReaderFlags(&cmdEnv.Flags, envFlags.ReaderFlagValues, defaultDBLocation)
	}
	cmdEnv.Flags.BoolVar(&envFlags.Verbose, "v", false, "print more detailed information")
}

func runList(jirix *jiri.X, args []string) error {
	if listFlags.showProfilesDB {
		data, err := jirix.NewSeq().ReadFile(listFlags.DBFilename)
		if err != nil {
			return err
		}
		fmt.Fprintln(jirix.Stdout(), string(data))
		return nil
	}
	if listFlags.Verbose {
		fmt.Fprintf(jirix.Stdout(), "Profiles Database Filename: %s\n", listFlags.DBFilename)
	}
	if listFlags.available {
		managers := profilesmanager.Managers()
		// TODO(cnicolaou): this will need to run the external
		// profile installer subcommands to obtain the list of
		// profiles that can be installed by them. For now it
		// just assumes they are all linked into this binary.
		if listFlags.Verbose {
			fmt.Fprintf(jirix.Stdout(), "Available Profiles:\n")
			for _, name := range managers {
				mgr := profilesmanager.LookupManager(name)
				vi := mgr.VersionInfo()
				fmt.Fprintf(jirix.Stdout(), "%s: versions: %s\n", name, vi)
			}
		} else {
			if len(managers) > 0 {
				fmt.Fprintf(jirix.Stdout(), "%s\n", strings.Join(managers, ", "))
			}
		}
	}
	rd, err := profilesreader.NewReader(jirix, listFlags.ProfilesMode, listFlags.DBFilename)
	if err != nil {
		return err
	}
	profileNames := args
	if len(args) == 0 {
		if IsFlagSet(&cmdList.Flags, "profiles") {
			profileNames = strings.Split(listFlags.Profiles, ",")
		} else {
			profileNames = rd.ProfileNames()
		}
	}
	availableNames := []string{}
	for _, name := range profileNames {
		if rd.LookupProfile(name) != nil {
			availableNames = append(availableNames, name)
		}
	}
	if listFlags.Verbose {
		fmt.Fprintf(jirix.Stdout(), "Installed Profiles: ")
		fmt.Fprintf(jirix.Stdout(), "%s\n", strings.Join(rd.ProfileNames(), ", "))
		for _, name := range availableNames {
			profile := rd.LookupProfile(name)
			fmt.Fprintf(jirix.Stdout(), "Profile: %s @ %s\n", profile.Name(), profile.Root())
			for _, target := range profile.Targets() {
				fmt.Fprintf(jirix.Stdout(), "\t%s\n", target.DebugString())
			}
		}
	} else {
		for _, name := range availableNames {
			profile := rd.LookupProfile(name)
			mgr := profilesmanager.LookupManager(name)
			out := &bytes.Buffer{}
			var targets profiles.Targets
			if listFlags.Target.IsSet() {
				targets = append(targets, rd.LookupProfileTarget(name, listFlags.Target))
			} else {
				targets = profile.Targets()
			}
			printHeader := len(availableNames) > 1 || len(targets) > 1 || len(listFlags.info) == 0
			for _, target := range targets {
				if printHeader {
					out.WriteString(fmtHeader(name, target))
					out.WriteString(" ")
				}
				r, err := fmtInfo(jirix, listFlags.info, rd, mgr, profile, target)
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

func fmtInfo(jirix *jiri.X, infoFmt string, rd *profilesreader.Reader, mgr profiles.Manager, profile *profiles.Profile, target *profiles.Target) (string, error) {
	if len(infoFmt) > 0 {
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
		info.SchemaVersion = rd.SchemaVersion()
		if target != nil {
			info.Target.InstallationDir = jiri.NewRelPath(target.InstallationDir).Abs(jirix)
			info.Target.CommandLineEnv = target.CommandLineEnv().Vars
			info.Target.Env = target.Env.Vars
			clenv := ""
			if len(info.Target.CommandLineEnv) > 0 {
				clenv = fmt.Sprintf(" --env=\"%s\" ", strings.Join(info.Target.CommandLineEnv, ","))
			}
			info.Target.Command = fmt.Sprintf("jiri v23-profile install --target=%s %s%s", target, clenv, name)
		}
		if profile != nil {
			rp := jiri.NewRelPath(profile.Root())
			info.Profile.Root = rp.Abs(jirix)
		}

		// Use a template to print out any field in our instance of listInfo.
		tmpl, err := template.New("list").Parse("{{ ." + infoFmt + "}}")
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
	if len(envFlags.Profiles) == 0 {
		return fmt.Errorf("no profiles were specified using --profiles")
	}
	rd, err := profilesreader.NewReader(jirix, envFlags.ProfilesMode, envFlags.DBFilename)
	if err != nil {
		return err
	}
	profileNames := strings.Split(envFlags.Profiles, ",")
	if err := rd.ValidateRequestedProfilesAndTarget(profileNames, envFlags.Target); err != nil {
		return err
	}
	rd.MergeEnvFromProfiles(envFlags.MergePolicies, envFlags.Target, profileNames...)
	out := fmtVars(rd.ToMap(), args)
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
