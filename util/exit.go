// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"os/exec"
	"syscall"

	"v.io/x/lib/cmdline"
)

const (
	// NoSnapshotExitCode is returned when the vbinary tool fails to
	// find a snapshot for the given date.
	NoSnapshotExitCode = 3
)

// TranslateExitCode translates errors from the "os/exec" package that
// contain exit codes into cmdline.ErrExitCode errors.
func TranslateExitCode(err error) error {
	if exit, ok := err.(*exec.ExitError); ok {
		if wait, ok := exit.Sys().(syscall.WaitStatus); ok {
			if status := wait.ExitStatus(); wait.Exited() && status != 0 {
				return cmdline.ErrExitCode(status)
			}
		}
	}
	return err
}
