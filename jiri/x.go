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
	"v.io/x/lib/timing"
)

const (
	RootEnv         = "JIRI_ROOT"
	RootMetaDir     = ".jiri_root"
	ProjectMetaDir  = ".jiri"
	ProjectMetaFile = "metadata.v2"
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
func NewX(env *cmdline.Env) (*X, error) {
	ctx := tool.NewContextFromEnv(env)
	root, err := findJiriRoot(ctx.Timer())
	if err != nil {
		return nil, err
	}
	return &X{
		Context: ctx,
		Root:    root,
		Usage:   env.UsageErrorf,
	}, nil
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
	return nil
}

// LocalManifestFile returns the path to the local manifest file.
func (x *X) LocalManifestFile() string {
	return filepath.Join(x.Root, ".local_manifest")
}

// LocalSnapshotDir returns the path to the local snapshot directory.
func (x *X) LocalSnapshotDir() string {
	return filepath.Join(x.Root, ".snapshot")
}

// RemoteSnapshotDir returns the path to the remote snapshot directory.
func (x *X) RemoteSnapshotDir() string {
	return filepath.Join(x.ManifestDir(), "snapshot")
}

// ManifestDir returns the path to the manifest directory.
func (x *X) ManifestDir() string {
	return filepath.Join(x.Root, ".manifest", "v2")
}

// ManifestFile returns the path to the manifest file with the given name.
func (x *X) ManifestFile(name string) string {
	return filepath.Join(x.ManifestDir(), name)
}

// ResolveManifestPath resolves the given manifest name to an absolute path in
// the local filesystem.
func (x *X) ResolveManifestPath(name string) (string, error) {
	if name != "" {
		if filepath.IsAbs(name) {
			return name, nil
		}
		return x.ManifestFile(name), nil
	}
	path := x.LocalManifestFile()
	switch _, err := os.Stat(path); {
	case err == nil:
		return path, nil
	case os.IsNotExist(err):
		return x.ManifestFile("default"), nil
	default:
		return "", fmt.Errorf("Stat(%v) failed: %v", path, err)
	}
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
