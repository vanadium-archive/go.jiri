// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package jiri provides utilities used by the jiri tool and related tools.
package jiri

// TODO(toddw): Rename this package to v.io/jiri, and rename the tool itself to
// v.io/jiri/cmd/jiri

import (
	"fmt"
	"os"
	"path/filepath"

	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/envvar"
	"v.io/x/lib/timing"
)

const (
	RootEnv          = "JIRI_ROOT"
	RootMetaDir      = ".jiri_root"
	ProjectMetaDir   = ".jiri"
	ProjectMetaFile  = "metadata.v2"
	ProfilesDBDir    = RootMetaDir + string(filepath.Separator) + "profile_db"
	ProfilesRootDir  = RootMetaDir + string(filepath.Separator) + "profiles"
	JiriManifestFile = ".jiri_manifest"

	// PreservePathEnv is the name of the environment variable that, when set to a
	// non-empty value, causes jiri tools to use the existing PATH variable,
	// rather than mutating it.
	PreservePathEnv = "JIRI_PRESERVE_PATH"
)

// X holds the execution environment for the jiri tool and related tools.  This
// includes the jiri filesystem root directory.
//
// TODO(toddw): Other jiri state should be transitioned to this struct,
// including the manifest and related operations.
type X struct {
	*tool.Context
	Root  string
	Usage func(format string, args ...interface{}) error
}

// NewX returns a new execution environment, given a cmdline env.
// It also prepends $JIRI_ROOT/.jiri_root/bin to the PATH.
func NewX(env *cmdline.Env) (*X, error) {
	ctx := tool.NewContextFromEnv(env)
	root, err := findJiriRoot(ctx.Timer())
	if err != nil {
		return nil, err
	}
	x := &X{
		Context: ctx,
		Root:    root,
		Usage:   env.UsageErrorf,
	}
	if ctx.Env()[PreservePathEnv] == "" {
		// Prepend $JIRI_ROOT/.jiri_root/bin to the PATH, so execing a binary will
		// invoke the one in that directory, if it exists.  This is crucial for jiri
		// subcommands, where we want to invoke the binary that jiri installed, not
		// whatever is in the user's PATH.
		//
		// Note that we must modify the actual os env variable with os.SetEnv and
		// also the ctx.env, so that execing a binary through the os/exec package
		// and with ctx.Run both have the correct behavior.
		newPath := envvar.PrependUniqueToken(ctx.Env()["PATH"], string(os.PathListSeparator), x.BinDir())
		ctx.Env()["PATH"] = newPath
		if err := os.Setenv("PATH", newPath); err != nil {
			return nil, err
		}
	}
	return x, nil
}

func findJiriRoot(timer *timing.Timer) (string, error) {
	if timer != nil {
		timer.Push("find JIRI_ROOT")
		defer timer.Pop()
	}
	if root := os.Getenv(RootEnv); root != "" {
		// Always use JIRI_ROOT if it's set.
		result, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", fmt.Errorf("%v EvalSymlinks(%v) failed: %v", RootEnv, root, err)
		}
		if !filepath.IsAbs(result) {
			return "", fmt.Errorf("%v isn't an absolute path: %v", RootEnv, result)
		}
		return filepath.Clean(result), nil
	}
	// TODO(toddw): Try to find the root by walking up the filesystem.
	return "", fmt.Errorf("%v is not set", RootEnv)
}

// FindRoot returns the root directory of the jiri environment.  All state
// managed by jiri resides under this root.
//
// If the RootEnv environment variable is non-empty, we always attempt to use
// it.  It must point to an absolute path, after symlinks are evaluated.
// TODO(toddw): Walk up the filesystem too.
//
// Returns an empty string if the root directory cannot be determined, or if any
// errors are encountered.
//
// FindRoot should be rarely used; typically you should use NewX to create a new
// execution environment, and handle errors.  An example of a valid usage is to
// initialize default flag values in an init func before main.
func FindRoot() string {
	root, _ := findJiriRoot(nil)
	return root
}

// Clone returns a clone of the environment.
func (x *X) Clone(opts tool.ContextOpts) *X {
	return &X{
		Context: x.Context.Clone(opts),
		Root:    x.Root,
		Usage:   x.Usage,
	}
}

// UsageErrorf prints the error message represented by the printf-style format
// and args, followed by the usage output.  The implementation typically calls
// cmdline.Env.UsageErrorf.
func (x *X) UsageErrorf(format string, args ...interface{}) error {
	if x.Usage != nil {
		return x.Usage(format, args...)
	}
	return fmt.Errorf(format, args...)
}

// RootMetaDir returns the path to the root metadata directory.
func (x *X) RootMetaDir() string {
	return filepath.Join(x.Root, RootMetaDir)
}

// JiriManifestFile returns the path to the .jiri_manifest file.
func (x *X) JiriManifestFile() string {
	return filepath.Join(x.Root, JiriManifestFile)
}

// BinDir returns the path to the bin directory.
func (x *X) BinDir() string {
	return filepath.Join(x.RootMetaDir(), "bin")
}

// ScriptsDir returns the path to the scripts directory.
func (x *X) ScriptsDir() string {
	return filepath.Join(x.RootMetaDir(), "scripts")
}

// UpdateHistoryDir returns the path to the update history directory.
func (x *X) UpdateHistoryDir() string {
	return filepath.Join(x.RootMetaDir(), "update_history")
}

// ProfilesDBDir returns the path to the profiles data base directory.
func (x *X) ProfilesDBDir() string {
	return filepath.Join(x.RootMetaDir(), "profile_db")
}

// ProfilesRootDir returns the path to the root of the profiles installation.
func (x *X) ProfilesRootDir() string {
	return filepath.Join(x.RootMetaDir(), "profiles")
}

// UpdateHistoryLatestLink returns the path to a symlink that points to the
// latest update in the update history directory.
func (x *X) UpdateHistoryLatestLink() string {
	return filepath.Join(x.UpdateHistoryDir(), "latest")
}

// UpdateHistorySecondLatestLink returns the path to a symlink that points to
// the second latest update in the update history directory.
func (x *X) UpdateHistorySecondLatestLink() string {
	return filepath.Join(x.UpdateHistoryDir(), "second-latest")
}

// RunnerFunc is an adapter that turns regular functions into cmdline.Runner.
// This is similar to cmdline.RunnerFunc, but the first function argument is
// jiri.X, rather than cmdline.Env.
func RunnerFunc(run func(*X, []string) error) cmdline.Runner {
	return runner(run)
}

type runner func(*X, []string) error

func (r runner) Run(env *cmdline.Env, args []string) error {
	x, err := NewX(env)
	if err != nil {
		return err
	}
	return r(x, args)
}
