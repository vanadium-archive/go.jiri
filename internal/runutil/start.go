// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil

import (
	"io"
	"os/exec"
)

type Start struct {
	*executor
}

// NewStart is the Start factory.
func NewStart(env map[string]string, stdin io.Reader, stdout, stderr io.Writer, color, dryRun, verbose bool) *Start {
	return &Start{
		newExecutor(env, stdin, stdout, stderr, color, dryRun, verbose),
	}
}

// Command starts the given command in background and logs its outcome using the
// default options.
func (s *Start) Command(path string, args ...string) (*exec.Cmd, error) {
	return s.CommandWithOpts(s.opts, path, args...)
}

// CommandWithOpts starts the given command in background and logs its outcome using
// the given options.
func (s *Start) CommandWithOpts(opts Opts, path string, args ...string) (*exec.Cmd, error) {
	return s.command(opts, path, args...)
}

func (s *Start) command(opts Opts, path string, args ...string) (*exec.Cmd, error) {
	return s.execute(false, 0, opts, path, args...)
}
