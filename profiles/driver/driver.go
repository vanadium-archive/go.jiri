package driver

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
	Runner: cmdline.RunnerFunc(runList),
	Name:   "list",
	Short:  "List supported and installed profiles",
	Long:   "List supported and installed profiles.",
}

// cmdList represents the "profile env" command.
var cmdEnv = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runEnv),
	Name:   "env",
	Short:  "Display profile environment variables",
	Long: `
List profile specific and target specific environment variables.
env --profile=<profile-name> --tag=<tag as appears in a target> [env var name]*

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
)

func init() {
	targetFlag = profiles.NativeTarget()

	var err error
	rootDir, err = project.JiriRoot()
	if err != nil {
		panic(err)
	}
	manifest := filepath.Join(rootDir, profiles.DefaultManifestFilename)

	commonFlags := func(flags *flag.FlagSet) {
		flags.Var(&targetFlag, "target", targetFlag.Usage())
		flags.Lookup("target").DefValue = "native=<runtime.GOARCH>-<runtime.GOOS>"
		flags.Var(&targetFlag.Env, "env", targetFlag.Env.Usage())
		flags.StringVar(&manifestFlag, "manifest", manifest, "specify the XML manifest to file read/write from.")
		flags.Lookup("manifest").DefValue = "$JIRI_ROOT/" + profiles.DefaultManifestFilename
	}
	commonFlags(&cmdInstall.Flags)
	commonFlags(&cmdUpdate.Flags)
	commonFlags(&cmdUninstall.Flags)
	cmdList.Flags.StringVar(&manifestFlag, "manifest", manifest, "specify the XML manifest to file read/write from.")
	cmdList.Flags.Lookup("manifest").DefValue = "$JIRI_ROOT/" + profiles.DefaultManifestFilename
	cmdList.Flags.BoolVar(&showManifestFlag, "show-manifest", false, "print out the manifest file")
	cmdEnv.Flags.StringVar(&manifestFlag, "manifest", manifest, "specify the XML manifest to file read/write from.")
	cmdEnv.Flags.Lookup("manifest").DefValue = "$JIRI_ROOT/" + profiles.DefaultManifestFilename
	cmdEnv.Flags.StringVar(&profileFlag, "profile", "", "the profile whose environment is to be displayed")
	cmdEnv.Flags.Var(&targetFlag, "target", targetFlag.Usage())
	cmdEnv.Flags.Lookup("target").DefValue = "native=<runtime.GOARCH>-<runtime.GOOS>"

	for _, mgr := range profiles.Managers() {
		profiles.LookupManager(mgr).AddFlags(&cmdInstall.Flags, profiles.Install)
		profiles.LookupManager(mgr).AddFlags(&cmdUpdate.Flags, profiles.Update)
		profiles.LookupManager(mgr).AddFlags(&cmdUninstall.Flags, profiles.Uninstall)
	}
	cmdUpdate.Flags.BoolVar(&forceFlag, "force", false, "force an uninstall followed by install")
}

func validateRequestedProfiles(names []string) error {
	for _, n := range names {
		if profiles.LookupManager(n) == nil {
			return fmt.Errorf("%q is not a supported profile, use the \"list\" command to see the ones that are available.", n)
		}
	}
	return nil
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
	fmt.Fprintf(ctx.Stdout(), "Manifest: %s\n", manifestFlag)
	fmt.Fprintf(ctx.Stdout(), "Available Profiles: %s\n", strings.Join(profiles.Managers(), ", "))
	if err := profiles.Read(ctx, manifestFlag); err != nil {
		fmt.Fprintf(ctx.Stderr(), "Failed to read manifest: %v", err)
		return err
	}
	fmt.Fprintf(ctx.Stdout(), "Installed Profiles: %s\n", strings.Join(profiles.Profiles(), ", "))
	for _, name := range profiles.Profiles() {
		profile := profiles.LookupProfile(name)
		fmt.Fprintf(ctx.Stdout(), "Profile: %s @ %s\n", profile.Name, profile.Root)
		for _, target := range profile.Targets {
			fmt.Fprintf(ctx.Stdout(), "\t%s\n", target)
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
		return fmt.Errorf("Profile %q is not installed", profileFlag)
	}
	target := profiles.FindTarget(profile.Targets, &targetFlag)
	if target == nil {
		return fmt.Errorf("Target %q is not installed for profile %q", profileFlag, profileFlag)
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
	if err := validateRequestedProfiles(args); err != nil {
		return err
	}
	if err := profiles.Read(ctx, manifestFlag); err != nil {
		fmt.Fprintf(ctx.Stderr(), "Failed to read manifest: %v", err)
		return err
	}
	return nil
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
			return mgr.Install(ctx, target)
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
	applyCommand(args, env, ctx, targetFlag,
		func(mgr profiles.Manager, ctx *tool.Context, target profiles.Target) error {
			err := mgr.Update(ctx, target)
			if forceFlag || (err == profiles.ErrNoIncrementalUpdate) {
				if err := runUninstall(env, args); err != nil {
					return err
				}
				if err := runInstall(env, args); err != nil {
					return err
				}
				err = nil
			}
			return err
		})
	return nil

}

func runUninstall(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env)
	if err := initCommand(ctx, args); err != nil {
		return err
	}
	applyCommand(args, env, ctx, targetFlag,
		func(mgr profiles.Manager, ctx *tool.Context, target profiles.Target) error {
			return mgr.Uninstall(ctx, target)
		})
	return profiles.Write(ctx, manifestFlag)
}
