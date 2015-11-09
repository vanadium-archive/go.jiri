// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles

import (
	"archive/zip"
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/project"
  "v.io/jiri/runutil"
	"v.io/jiri/tool"
)

const (
	DefaultDirPerm  = os.FileMode(0755)
	DefaultFilePerm = os.FileMode(0644)
	targetDefValue  = "<runtime.GOARCH>-<runtime.GOOS>"
)

// RegisterTargetFlag registers the commonly used --target flag with
// the supplied FlagSet.
func RegisterTargetFlag(flags *flag.FlagSet, target *Target) {
	*target = DefaultTarget()
	flags.Var(target, "target", target.Usage())
	flags.Lookup("target").DefValue = targetDefValue
}

// RegisterTargetAndEnvFlags registers the commonly used --target and --env
// flags with the supplied FlagSet
func RegisterTargetAndEnvFlags(flags *flag.FlagSet, target *Target) {
	*target = DefaultTarget()
	flags.Var(target, "target", target.Usage())
	flags.Lookup("target").DefValue = targetDefValue
	flags.Var(&target.commandLineEnv, "env", target.commandLineEnv.Usage())
}

// RegisterManifestFlag registers the commonly used --profiles-manifest
// flag with the supplied FlagSet.
func RegisterManifestFlag(flags *flag.FlagSet, manifest *string, defaultManifest string) {
	root, _ := project.JiriRoot()
	flags.StringVar(manifest, "profiles-manifest", filepath.Join(root, defaultManifest), "specify the profiles XML manifest filename.")
	flags.Lookup("profiles-manifest").DefValue = filepath.Join("$JIRI_ROOT", defaultManifest)
}

// RegisterProfileFlags registers the commonly used --profiles-manifest, --profiles,
// --target and --merge-policies flags with the supplied FlagSet.
func RegisterProfileFlags(flags *flag.FlagSet, profilesMode *ProfilesMode, manifest, profiles *string, defaultManifest string, policies *MergePolicies, target *Target) {
	flags.Var(profilesMode, "skip-profiles", "if set, no profiles will be used")
	RegisterProfilesFlag(flags, profiles)
	RegisterMergePoliciesFlag(flags, policies)
	RegisterManifestFlag(flags, manifest, defaultManifest)
	RegisterTargetFlag(flags, target)
}

// RegisterProfilesFlag registers the --profiles flag
func RegisterProfilesFlag(flags *flag.FlagSet, profiles *string) {
	flags.StringVar(profiles, "profiles", "base,jiri", "a comma separated list of profiles to use")
}

// RegisterMergePoliciesFlag registers the --merge-policies flag
func RegisterMergePoliciesFlag(flags *flag.FlagSet, policies *MergePolicies) {
	flags.Var(policies, "merge-policies", "specify policies for merging environment variables")
}

type AppendJiriProfileMode bool

const (
	AppendJiriProfile      AppendJiriProfileMode = true
	DoNotAppendJiriProfile                       = false
)

// InitProfilesFromFlag splits a comma separated list of profile names into
// a slice and optionally appends the 'jiri' profile if it's not already
// present.
func InitProfilesFromFlag(flag string, appendJiriProfile AppendJiriProfileMode) []string {
	n := strings.Split(flag, ",")
	if appendJiriProfile == AppendJiriProfile && !strings.Contains(flag, "jiri") {
		n = append(n, "jiri")
	}
	return n
}

// AtomicAction performs an action 'atomically' by keeping track of successfully
// completed actions in the supplied completion log and re-running them if they
// are not successfully logged therein after deleting the entire contents of the
// dir parameter. Consequently it does not make sense to apply AtomicAction to
// the same directory in sequence.
func AtomicAction(ctx *tool.Context, installFn func() error, dir, message string) error {
	atomicFn := func() error {
		completionLogPath := filepath.Join(dir, ".complete")
		if dir != "" && ctx.Run().DirectoryExists(dir) {
			// If the dir exists but the completionLogPath doesn't, then it means
			// the previous action didn't finish.
			// Remove the dir so we can perform the action again.
			if !ctx.Run().FileExists(completionLogPath) {
				ctx.Run().RemoveAll(dir)
			} else {
				fmt.Fprintf(ctx.Stdout(), "AtomicAction: %s already completed in %s\n", message, dir)
				return nil
			}
		}
		if err := installFn(); err != nil {
			if dir != "" {
				ctx.Run().RemoveAll(dir)
			}
			return err
		}
		if err := ctx.Run().WriteFile(completionLogPath, []byte("completed"), DefaultFilePerm); err != nil {
			return err
		}
		return nil
	}
	return ctx.Run().Function(atomicFn, message)
}

// RunCommand runs the specified command with the specified args and environment
// whilst logging the output to ctx.Stdout() on error or if tracing is requested
// via the -v flag.
func RunCommand(ctx *tool.Context, env map[string]string, bin string, args ...string) error {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	opts.Env = env
	err := ctx.Run().CommandWithOpts(opts, bin, args...)
	if err != nil || tool.VerboseFlag {
		fmt.Fprintf(ctx.Stdout(), "%s", out.String())
	}
	return err
}

func brewList(ctx *tool.Context) (map[string]bool, error) {
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	err := ctx.Run().CommandWithOpts(opts, "brew", "list")
	if err != nil || tool.VerboseFlag {
		fmt.Fprintf(ctx.Stdout(), "%s", out.String())
	}
	scanner := bufio.NewScanner(&out)
	pkgs := map[string]bool{}
	for scanner.Scan() {
		pkgs[scanner.Text()] = true
	}
	return pkgs, err
}

// InstallPackages identifies the packages that need to be installed
// and installs them using the OS-specific package manager.
func InstallPackages(ctx *tool.Context, pkgs []string) error {
	installDepsFn := func() error {
		switch runtime.GOOS {
		case "linux":
			if runutil.IsFNLHost() {
				fmt.Fprintf(ctx.Stdout(), "skipping installation of %v on FNL host", pkgs)
				fmt.Fprintf(ctx.Stdout(), "success\n")
				break
			}
			installPkgs := []string{}
			for _, pkg := range pkgs {
				if err := RunCommand(ctx, nil, "dpkg", "-L", pkg); err != nil {
					installPkgs = append(installPkgs, pkg)
				}
			}
			if len(installPkgs) > 0 {
				args := append([]string{"apt-get", "install", "-y"}, installPkgs...)
				fmt.Fprintf(ctx.Stdout(), "Running: sudo %s: ", strings.Join(args, " "))
				if err := RunCommand(ctx, nil, "sudo", args...); err != nil {
					fmt.Fprintf(ctx.Stdout(), "%v\n", err)
					return err
				}
				fmt.Fprintf(ctx.Stdout(), "success\n")
			}
		case "darwin":
			installPkgs := []string{}
			installedPkgs, err := brewList(ctx)
			if err != nil {
				return err
			}
			for _, pkg := range pkgs {
				if !installedPkgs[pkg] {
					installPkgs = append(installPkgs, pkg)
				}
			}
			if len(installPkgs) > 0 {
				args := append([]string{"install"}, installPkgs...)
				fmt.Fprintf(ctx.Stdout(), "Running: brew %s: ", strings.Join(args, " "))
				if err := RunCommand(ctx, nil, "brew", args...); err != nil {
					fmt.Fprintf(ctx.Stdout(), "%v\n", err)
					return err
				}
				fmt.Fprintf(ctx.Stdout(), "success\n")
			}
		}
		return nil
	}
	return ctx.Run().Function(installDepsFn, "Install dependencies")
}

// EnsureProfileTargetIsInstalled ensures that the requested profile and target
// is installed, installing it if only if necessary.
func EnsureProfileTargetIsInstalled(ctx *tool.Context, profile string, target Target, root string) error {
	if t := LookupProfileTarget(profile, target); t != nil {
		if ctx.Run().Opts().Verbose {
			fmt.Fprintf(ctx.Stdout(), "%v %v is already installed as %v\n", profile, target, t)
		}
		return nil
	}
	mgr := LookupManager(profile)
	if mgr == nil {
		return fmt.Errorf("profile %v is not supported", profile)
	}
	version, err := mgr.VersionInfo().Select(target.Version())
	if err != nil {
		return err
	}
	target.SetVersion(version)
	mgr.SetRoot(root)
	if ctx.Run().Opts().Verbose || ctx.Run().Opts().DryRun {
		fmt.Fprintf(ctx.Stdout(), "install %s %s\n", profile, target.DebugString())
	}
	if err := mgr.Install(ctx, target); err != nil {
		return err
	}
	return nil
}

// EnsureProfileTargetIsUninstalled ensures that the requested profile and target
// are no longer installed.
func EnsureProfileTargetIsUninstalled(ctx *tool.Context, profile string, target Target, root string) error {
	if LookupProfileTarget(profile, target) != nil {
		if ctx.Run().Opts().Verbose {
			fmt.Fprintf(ctx.Stdout(), "%v is not installed: %v", profile, target)
		}
		return nil
	}
	mgr := LookupManager(profile)
	if mgr == nil {
		return fmt.Errorf("profile %v is not supported", profile)
	}
	version, err := mgr.VersionInfo().Select(target.Version())
	if err != nil {
		return err
	}
	target.SetVersion(version)
	mgr.SetRoot(root)
	if ctx.Run().Opts().Verbose || ctx.Run().Opts().DryRun {
		fmt.Fprintf(ctx.Stdout(), "uninstall %s %s\n", profile, target.DebugString())
	}
	if err := mgr.Uninstall(ctx, target); err != nil {
		return err
	}
	return nil
}

// Fetch downloads the specified url and saves it to dst.
// TODO(nlacasse, cnicoloau): Move this to a package for profile-implementors
// so it does not pollute the profile package namespace.
func Fetch(ctx *tool.Context, dst, url string) error {
	ctx.Run().Output([]string{"fetching " + url})
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("got non-200 status code while getting %v: %v", url, resp.StatusCode)
	}
	file, err := ctx.Run().Create(dst)
	if err != nil {
		return err
	}
	_, err = ctx.Run().Copy(file, resp.Body)
	return err
}

// GitCloneRepo clones a repo at a specific revision in outDir.
// TODO(nlacasse, cnicoloau): Move this to a package for profile-implementors
// so it does not pollute the profile package namespace.
func GitCloneRepo(ctx *tool.Context, remote, revision, outDir string, outDirPerm os.FileMode) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)

	if err := ctx.Run().MkdirAll(outDir, outDirPerm); err != nil {
		return err
	}
	if err := ctx.Git().Clone(remote, outDir); err != nil {
		return err
	}
	if err := ctx.Run().Chdir(outDir); err != nil {
		return err
	}
	return ctx.Git().Reset(revision)
}

// Unzip unzips the file in srcFile and puts resulting files in directory dstDir.
// TODO(nlacasse, cnicoloau): Move this to a package for profile-implementors
// so it does not pollute the profile package namespace.
func Unzip(ctx *tool.Context, srcFile, dstDir string) error {
	r, err := zip.OpenReader(srcFile)
	if err != nil {
		return err
	}
	defer r.Close()

	unzipFn := func(zFile *zip.File) error {
		rc, err := zFile.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		fileDst := filepath.Join(dstDir, zFile.Name)
		if zFile.FileInfo().IsDir() {
			return ctx.Run().MkdirAll(fileDst, zFile.Mode())
		}
		file, err := ctx.Run().OpenFile(fileDst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zFile.Mode())
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = ctx.Run().Copy(file, rc)
		return err
	}

	ctx.Run().Output([]string{"unzipping " + srcFile})
	for _, zFile := range r.File {
		ctx.Run().Output([]string{"extracting " + zFile.Name})
		if err := unzipFn(zFile); err != nil {
			return err
		}
	}
	return nil
}
