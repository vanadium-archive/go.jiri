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
	"path/filepath"
	"strings"
	"text/template"

	"v.io/jiri"
	"v.io/jiri/profiles"
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
		ArgsLong: `<profiles> is a list of profiles to list, defaulting to all
profiles if none are specifically requested. List can also be used
to test for the presence of a specific target for the requested profiles.
If the target is not installed, it will exit with an error.`,
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
be printed, otherwise both name and value are printed, i.e. CFLAGS="foo" vs
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
	flags.StringVar(manifest, "profiles-db", filepath.Join(root, defaultDBPath), "the path, relative to JIRI_ROOT, that contains the profiles database.")
	flags.Lookup("profiles-db").DefValue = filepath.Join("$JIRI_ROOT", defaultDBPath)
}

// RegisterProfilesFlag registers the --profiles flag
func RegisterProfilesFlag(flags *flag.FlagSet, defaultProfiles string, profiles *string) {
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
func RegisterReaderFlags(flags *flag.FlagSet, fv *ReaderFlagValues, defaultProfiles, defaultDBPath string) {
	flags.Var(&fv.ProfilesMode, "skip-profiles", "if set, no profiles will be used")
	RegisterDBPathFlag(flags, &fv.DBFilename, defaultDBPath)
	RegisterProfilesFlag(flags, defaultProfiles, &fv.Profiles)
	fv.MergePolicies = profilesreader.JiriMergePolicies()
	RegisterMergePoliciesFlag(flags, &fv.MergePolicies)
	profiles.RegisterTargetAndEnvFlags(flags, &fv.Target)
}

// RegisterReaderCommandsUsingParent registers the 'reader' flags
// (see RegisterReaderFlags) with the parent command and creates the
// list and env subcommands. The values of the flags can be accessed via
// the supplied ReaderFlagValues struct.
// RegisterReaderCommandsUsingParent results in a command line of the form:
// <parent> <reader-flags> [list|env] <list/env specific commands>
func RegisterReaderCommandsUsingParent(parent *cmdline.Command, fv *ReaderFlagValues, defaultProfiles, defaultDBPath string) {
	envFlags.ReaderFlagValues = fv
	listFlags.ReaderFlagValues = fv
	RegisterReaderFlags(&parent.Flags, fv, defaultProfiles, defaultDBPath)
	RegisterReaderCommands(parent, defaultProfiles, defaultDBPath)
}

// RegisterReaderCommands registers the list and env subcommands. The
// subcommands will host the 'reader' flags (see RegisterReaderFlags)
// resulting in a command line of the form:
// <parent> [list|env] <reader-flags> <list/env specific specific commands>
func RegisterReaderCommands(parent *cmdline.Command, defaultProfiles, defaultDBPath string) {
	registerListCommand(parent, defaultProfiles, defaultDBPath)
	registerEnvCommand(parent, defaultProfiles, defaultDBPath)
}

func newReaderFlags() *ReaderFlagValues {
	return &ReaderFlagValues{MergePolicies: profilesreader.JiriMergePolicies()}
}

// registerListCommand the profiles list subcommand and returns it
// and a struct containing  the values of the command line flags.
func registerListCommand(parent *cmdline.Command, defaultProfiles, defaultDBPath string) {
	parent.Children = append(parent.Children, cmdList)
	if listFlags.ReaderFlagValues == nil {
		listFlags.ReaderFlagValues = newReaderFlags()
		RegisterReaderFlags(&cmdList.Flags, listFlags.ReaderFlagValues, defaultProfiles, defaultDBPath)
	}
	cmdList.Flags.BoolVar(&listFlags.Verbose, "v", false, "print more detailed information")
	cmdList.Flags.StringVar(&listFlags.info, "info", "", infoUsage())
}

// registerEnvCommand the profiles env subcommand and returns it and a
// struct containing the values of the command line flags.
func registerEnvCommand(parent *cmdline.Command, defaultProfiles, defaultDBPath string) {
	parent.Children = append(parent.Children, cmdEnv)
	if envFlags.ReaderFlagValues == nil {
		envFlags.ReaderFlagValues = newReaderFlags()
		RegisterReaderFlags(&cmdEnv.Flags, envFlags.ReaderFlagValues, defaultProfiles, defaultDBPath)
	}
	cmdEnv.Flags.BoolVar(&envFlags.Verbose, "v", false, "print more detailed information")
}

func matchingTargets(rd *profilesreader.Reader, profile *profiles.Profile) profiles.Targets {
	var targets profiles.Targets
	if IsFlagSet(cmdList.ParsedFlags, "target") {
		if t := rd.LookupProfileTarget(profile.Name(), listFlags.Target); t != nil {
			targets = profiles.Targets{t}
		}
	} else {
		targets = profile.Targets()
	}
	targets.Sort()
	return targets
}

func runList(jirix *jiri.X, args []string) error {
	if listFlags.Verbose {
		fmt.Fprintf(jirix.Stdout(), "Profiles Database Path: %s\n", listFlags.DBFilename)
	}
	rd, err := profilesreader.NewReader(jirix, listFlags.ProfilesMode, listFlags.DBFilename)
	if err != nil {
		return err
	}
	profileNames := []string{}
	for _, a := range args {
		if a != "" {
			profileNames = append(profileNames, a)
		}
	}
	if len(args) == 0 {
		if IsFlagSet(cmdList.ParsedFlags, "profiles") {
			profileNames = strings.Split(listFlags.Profiles, ",")
		} else {
			profileNames = rd.ProfileNames()
		}
	}

	if listFlags.Verbose {
		fmt.Fprintf(jirix.Stdout(), "Installed Profiles: ")
		fmt.Fprintf(jirix.Stdout(), "%s\n", strings.Join(rd.ProfileNames(), ", "))
		for _, name := range profileNames {
			profile := rd.LookupProfile(name)
			if profile == nil {
				continue
			}
			fmt.Fprintf(jirix.Stdout(), "Profile: %s @ %s\n", profile.Name(), profile.Root())
			for _, target := range matchingTargets(rd, profile) {
				fmt.Fprintf(jirix.Stdout(), "\t%s\n", target.DebugString())
			}
		}
		return nil
	}
	if listFlags.info == "" {
		matchingNames := []string{}
		for _, name := range profileNames {
			profile := rd.LookupProfile(name)
			if profile == nil {
				continue
			}
			if len(matchingTargets(rd, profile)) > 0 {
				matchingNames = append(matchingNames, name)
			}
		}
		if len(matchingNames) > 0 {
			fmt.Fprintln(jirix.Stdout(), strings.Join(matchingNames, ", "))
		} else {
			if IsFlagSet(cmdList.ParsedFlags, "target") {
				return fmt.Errorf("no matching targets for %s", listFlags.Target)
			}
		}
		return nil
	}
	// Handle --info
	found := false
	for _, name := range profileNames {
		profile := rd.LookupProfile(name)
		if profile == nil {
			continue
		}
		targets := matchingTargets(rd, profile)
		out := &bytes.Buffer{}
		printHeader := len(profileNames) > 1 || len(targets) > 1 || len(listFlags.info) == 0
		for _, target := range targets {
			if printHeader {
				out.WriteString(fmtHeader(name, target))
				out.WriteString(" ")
			}
			r, err := fmtInfo(jirix, listFlags.info, rd, profile, target)
			if err != nil {
				return err
			}
			out.WriteString(r)
			if printHeader {
				out.WriteString("\n")
			}
			found = true
		}
		fmt.Fprint(jirix.Stdout(), out.String())
	}
	if !found && IsFlagSet(cmdList.ParsedFlags, "target") {
		return fmt.Errorf("no matching targets for %s", listFlags.Target)
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
	DBPath        string
	Target        struct {
		InstallationDir string
		CommandLineEnv  []string
		Env             []string
		Command         string
	}
	Profile struct {
		Root      string
		Name      string
		Installer string
		DBPath    string
	}
}

func infoUsage() string {
	return `The following fields for use with -info are available:
	SchemaVersion - the version of the profiles implementation.
	DBPath - the path for the profiles database.
	Target.InstallationDir - the installation directory of the requested profile.
	Target.CommandLineEnv - the environment variables specified via the command line when installing this profile target.
	Target.Env - the environment variables computed by the profile installation process for this target.
	Target.Command - a command that can be used to create this profile.
	Note: if no --target is specified then the requested field will be displayed for all targets.

	Profile.Root - the root directory of the requested profile.
	Profile.Name - the qualified name of the profile.
	Profile.Installer - the name of the profile installer.
	Profile.DBPath - the path to the database file for this profile.
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
	w := textutil.NewUTF8WrapWriter(out, width)
	fmt.Fprint(w, o)
	w.Flush()
	return out.String()
}

func fmtInfo(jirix *jiri.X, infoFmt string, rd *profilesreader.Reader, profile *profiles.Profile, target *profiles.Target) (string, error) {
	// Populate an instance listInfo
	info := &listInfo{}
	name := profile.Name()
	installer, _ := profiles.SplitProfileName(name)
	info.SchemaVersion = rd.SchemaVersion()
	info.DBPath = rd.Path()
	if target != nil {
		info.Target.InstallationDir = jiri.NewRelPath(target.InstallationDir).Abs(jirix)
		info.Target.CommandLineEnv = target.CommandLineEnv().Vars
		info.Target.Env = target.Env.Vars
		clenv := ""
		if len(info.Target.CommandLineEnv) > 0 {
			clenv = fmt.Sprintf(" --env=\"%s\" ", strings.Join(info.Target.CommandLineEnv, ","))
		}
		if installer != "" {
			info.Target.Command = fmt.Sprintf("jiri profile install --target=%s %s%s", target, clenv, name)
		} else {
			// TODO(cnicolaou): remove this when the transition is complete.
			info.Target.Command = fmt.Sprintf("jiri v23-profile install --target=%s %s%s", target, clenv, name)
		}
	}
	if profile != nil {
		rp := jiri.NewRelPath(profile.Root())
		info.Profile.Root = rp.Abs(jirix)
		info.Profile.Name = name
		info.Profile.Installer = installer
		info.Profile.DBPath = info.DBPath
		if installer != "" {
			info.Profile.DBPath = filepath.Join(info.DBPath, installer)
		}
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
