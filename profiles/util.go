// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"v.io/jiri/collect"
	"v.io/jiri/tool"
	"v.io/x/lib/envvar"
)

const (
	DefaultDirPerm  = os.FileMode(0755)
	DefaultFilePerm = os.FileMode(0644)
)

// GoFlags lists all of the Go environment variables and will be sorted in the
// init function for this package.
var GoFlags = []string{
	"CGO_ENABLED",
	"CGO_CFLAGS",
	"CGO_CXXFLAGS",
	"CGO_LDFLAGS",
	"GOARCH",
	"GOBIN",
	"GOOS",
	"GOPATH",
	"GOROOT",
}

func init() {
	sort.Strings(GoFlags)
}

// UnsetGoEnv unsets Go environment variables in the given environment.
func UnsetGoEnv(env *envvar.Vars) {
	for _, k := range GoFlags {
		env.Set(k, "")
	}
}

// GoEnvironmentFromOS() returns the values of all Go environment variables
// as set via the OS; unset variables are omitted.
func GoEnvironmentFromOS() []string {
	os := envvar.SliceToMap(os.Environ())
	vars := make([]string, 0, len(GoFlags))
	for _, k := range GoFlags {
		v, present := os[k]
		if !present {
			continue
		}
		vars = append(vars, envvar.JoinKeyValue(k, v))
	}
	return vars
}

// DirectoryExists tests if a directory exists with appropriate logging.
func DirectoryExists(ctx *tool.Context, dir string) bool {
	isdir, err := ctx.Run().IsDir(dir)
	if err != nil {
		return false
	}
	return isdir
}

// FileExists tests if a file exists with appropriate logging.
func FileExists(ctx *tool.Context, file string) bool {
	_, err := ctx.Run().Stat(file)
	return err == nil
}

// AtomicAction performs an action 'atomically' by keeping track of successfully
// completed actions in the supplied completion log and re-running them if they
// are not successfully logged therein after deleting the entire contents of the
// dir parameter. Consequently it does not make sense to apply AtomicAction to
// the same directory in sequence.
func AtomicAction(ctx *tool.Context, installFn func() error, dir, message string) error {
	atomicFn := func() error {
		completionLogPath := filepath.Join(dir, ".complete")
		if dir != "" && DirectoryExists(ctx, dir) {
			// If the dir exists but the completionLogPath doesn't, then it means
			// the previous action didn't finish.
			// Remove the dir so we can perform the action again.
			if !FileExists(ctx, completionLogPath) {
				ctx.Run().RemoveAll(dir)
			} else {
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
func RunCommand(ctx *tool.Context, bin string, args []string, env map[string]string) error {
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
			installPkgs := []string{}
			for _, pkg := range pkgs {
				if err := RunCommand(ctx, "dpkg", []string{"-s", pkg}, nil); err != nil {
					installPkgs = append(installPkgs, pkg)
				}
			}
			if len(installPkgs) > 0 {
				args := append([]string{"apt-get", "install", "-y"}, installPkgs...)
				if err := RunCommand(ctx, "sudo", args, nil); err != nil {
					return err
				}
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
				if err := RunCommand(ctx, "brew", args, nil); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return ctx.Run().Function(installDepsFn, "Install dependencies")
}

// GitCloneRepo clones a repo at a specific revision in outDir.
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
	if err := ctx.Git().Reset(revision); err != nil {
		return err
	}
	return nil
}

// TargetSpecificDirname returns a directory name that is specific
// to that target taking the Tag, Arch, OS and environment variables
// if relevant into account (e.g GOARM={5,6,7}). If ignoreTag is set
// then the target Tag will never be used a shorthand for the entire target.
// It is intended to be used
func TargetSpecificDirname(target Target, ignoreTag bool) string {
	if !ignoreTag && len(target.Tag) > 0 {
		return target.Tag
	}
	env := envvar.SliceToMap(target.Env.Vars)
	dir := target.Arch + "_" + target.OS
	if target.Arch == "arm" {
		if armv, present := env["GOARM"]; present {
			dir += "_armv" + armv
		}
	}
	return dir
}
