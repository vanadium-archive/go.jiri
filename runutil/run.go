// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package runutil provides functions for running commands and
// functions and logging their outcome.
package runutil

import (
	"fmt"
	"io"
	"strings"
	"time"
)

var (
	commandTimedOutErr = fmt.Errorf("command timed out")
)

type Run struct {
	*executor
}

// NewRun is the Run factory.
func NewRun(env map[string]string, stdin io.Reader, stdout, stderr io.Writer, color, dryRun, verbose bool) *Run {
	return &Run{
		newExecutor(env, stdin, stdout, stderr, color, dryRun, verbose),
	}
}

// Command runs the given command and logs its outcome using the
// default options.
func (r *Run) Command(path string, args ...string) error {
	return r.CommandWithOpts(r.opts, path, args...)
}

// CommandWithOpts runs the given command and logs its outcome using
// the given options.
func (r *Run) CommandWithOpts(opts Opts, path string, args ...string) error {
	return r.command(0, opts, path, args...)
}

// TimedCommand runs the given command with a timeout and logs its
// outcome using the default options.
func (r *Run) TimedCommand(timeout time.Duration, path string, args ...string) error {
	return r.TimedCommandWithOpts(timeout, r.opts, path, args...)
}

// TimedCommandWithOpts runs the given command with a timeout and logs
// its outcome using the given options.
func (r *Run) TimedCommandWithOpts(timeout time.Duration, opts Opts, path string, args ...string) error {
	return r.command(timeout, opts, path, args...)
}

func (r *Run) command(timeout time.Duration, opts Opts, path string, args ...string) error {
	_, err := r.execute(true, timeout, opts, path, args...)
	return err
}

// Function runs the given function and logs its outcome using the
// default verbosity.
func (r *Run) Function(fn func() error, format string, args ...interface{}) error {
	return r.FunctionWithOpts(r.opts, fn, format, args...)
}

// FunctionWithOpts runs the given function and logs its outcome using
// the given options.
func (r *Run) FunctionWithOpts(opts Opts, fn func() error, format string, args ...interface{}) error {
	r.increaseIndent()
	defer r.decreaseIndent()
	if opts.Verbose {
		r.printf(r.opts.Stdout, format, args...)
	}
	err := fn()
	if err != nil {
		if opts.Verbose {
			r.printf(r.opts.Stdout, "FAILED: %v", err)
		}
	} else {
		if opts.Verbose {
			r.printf(r.opts.Stdout, "OK")
		}
	}
	return err
}

// Output logs the given list of lines using the default verbosity.
func (r *Run) Output(output []string) {
	r.OutputWithOpts(r.opts, output)
}

// OutputWithOpts logs the given list of lines using the given
// options.
func (r *Run) OutputWithOpts(opts Opts, output []string) {
	if opts.Verbose {
		for _, line := range output {
			r.logLine(line)
		}
	}
}

func (r *Run) logLine(line string) {
	if !strings.HasPrefix(line, prefix) {
		r.increaseIndent()
		defer r.decreaseIndent()
	}
	r.printf(r.opts.Stdout, "%v", line)
}
