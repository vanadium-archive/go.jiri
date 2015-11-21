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
)

const (
	rootEnv = "JIRI_ROOT"
)

// X holds the execution environment for the jiri tool and related tools.  This
// includes the jiri filesystem root directory.
//
// TODO(toddw): The Root is not currently used; we still use project.JiriRoot
// everywhere.  Transition those uses gradually over to use this instead.
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
	root, err := findJiriRoot(ctx)
	if err != nil {
		return nil, err
	}
	return &X{
		Context: ctx,
		Root:    root,
		Usage:   env.UsageErrorf,
	}, nil
}

func findJiriRoot(ctx *tool.Context) (string, error) {
	ctx.TimerPush("find JIRI_ROOT")
	defer ctx.TimerPop()
	if root := os.Getenv(rootEnv); root != "" {
		// Always use JIRI_ROOT if it's set.
		result, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", fmt.Errorf("%v EvalSymlinks(%v) failed: %v", rootEnv, root, err)
		}
		if !filepath.IsAbs(result) {
			return "", fmt.Errorf("%v isn't an absolute path: %v", rootEnv, result)
		}
		return filepath.Clean(result), nil
	}
	// TODO(toddw): Try to find the root by walking up the filesystem.
	return "", fmt.Errorf("%v is not set", rootEnv)
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
