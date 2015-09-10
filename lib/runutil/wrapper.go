// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil

// TODO(jsimsa): Write wrappers for additional functions from the Go
// standard libraries "os" and "ioutil" that our tools use: Chmod(),
// Create(), OpenFile(), ...

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// helper executes the given Go standard library function,
// encapsulated as a closure, respecting the "dry run" option.
func (r *Run) helper(fn func() error, format string, args ...interface{}) error {
	if opts := r.opts; opts.DryRun {
		opts.Verbose = true
		return r.FunctionWithOpts(opts, func() error { return nil }, format, args...)
	}
	return r.Function(fn, format, args...)
}

// dryRun executes the given Go standard library function, encapsulated as a
// closure, but translating "dry run" into "verbose" for this particular
// command so that the command can execute and thus allow subsequent
// commands to complete. It is generally used for testing/making files/directories
// that affect subsequent behaviour.
func (r *Run) dryRun(fn func() error, format string, args ...interface{}) error {
	if opts := r.opts; opts.DryRun {
		// Disable the dry run option as this function has no effect and
		// doing so results in more informative "dry run" output.
		opts.DryRun = false
		opts.Verbose = true
		return r.FunctionWithOpts(opts, fn, format, args...)
	}
	return r.Function(fn, format, args...)
}

// Chdir is a wrapper around os.Chdir that handles options such as
// "verbose" or "dry run".
func (r *Run) Chdir(dir string) error {
	return r.dryRun(func() error {
		return os.Chdir(dir)
	}, fmt.Sprintf("cd %q", dir))
}

// Chmod is a wrapper around os.Chmod that handles options such as
// "verbose" or "dry run".
func (r *Run) Chmod(dir string, mode os.FileMode) error {
	return r.helper(func() error { return os.Chmod(dir, mode) }, fmt.Sprintf("chmod %v %q", mode, dir))
}

// MkdirAll is a wrapper around os.MkdirAll that handles options such
// as "verbose" or "dry run".
func (r *Run) MkdirAll(dir string, mode os.FileMode) error {
	return r.helper(func() error { return os.MkdirAll(dir, mode) }, fmt.Sprintf("mkdir -p %q", dir))
}

// Open is a wrapper around os.Open that handles options such as
// "verbose" or "dry run".
func (r *Run) Open(name string) (*os.File, error) {
	var file *os.File
	var err error
	r.helper(func() error {
		file, err = os.Open(name)
		return err
	}, fmt.Sprintf("open %q", name))
	return file, err
}

// ReadDir is a wrapper around ioutil.ReadDir that handles options
// such as "verbose" or "dry run".
func (r *Run) ReadDir(dirname string) ([]os.FileInfo, error) {
	var fileInfos []os.FileInfo
	var err error
	r.dryRun(func() error {
		fileInfos, err = ioutil.ReadDir(dirname)
		return err
	}, fmt.Sprintf("ls %q", dirname))
	return fileInfos, err
}

// ReadFile is a wrapper around ioutil.ReadFile that handles options
// such as "verbose" or "dry run".
func (r *Run) ReadFile(filename string) ([]byte, error) {
	var bytes []byte
	var err error
	r.dryRun(func() error {
		bytes, err = ioutil.ReadFile(filename)
		return err
	}, fmt.Sprintf("read %q", filename))
	return bytes, err
}

// RemoveAll is a wrapper around os.RemoveAll that handles options
// such as "verbose" or "dry run".
func (r *Run) RemoveAll(dir string) error {
	return r.helper(func() error { return os.RemoveAll(dir) }, fmt.Sprintf("rm -rf %q", dir))
}

// Remove is a wrapper around os.Remove that handles options
// such as "verbose" or "dry run".
func (r *Run) Remove(file string) error {
	return r.helper(func() error { return os.Remove(file) }, fmt.Sprintf("rm %q", file))
}

// Rename is a wrapper around os.Rename that handles options such as
// "verbose" or "dry run".
func (r *Run) Rename(src, dst string) error {
	return r.helper(func() error {
		if err := os.Rename(src, dst); err != nil {
			// Check if the rename operation failed
			// because the source and destination are
			// located on different mount points.
			linkErr, ok := err.(*os.LinkError)
			if !ok {
				return err
			}
			errno, ok := linkErr.Err.(syscall.Errno)
			if !ok || errno != syscall.EXDEV {
				return err
			}
			// Fall back to a non-atomic rename.
			cmd := exec.Command("mv", src, dst)
			return cmd.Run()
		}
		return nil
	}, fmt.Sprintf("mv %q %q", src, dst))
}

// Stat is a wrapper around os.Stat that handles options such as
// "verbose" or "dry run".
func (r *Run) Stat(name string) (os.FileInfo, error) {
	var fileInfo os.FileInfo
	var err error
	r.dryRun(func() error {
		fileInfo, err = os.Stat(name)
		return err
	}, fmt.Sprintf("stat %q", name))
	return fileInfo, err
}

// IsDir is a wrapper around os.Stat that handles options such as
// "verbose" or "dry run".
func (r *Run) IsDir(name string) (bool, error) {
	var fileInfo os.FileInfo
	var err error
	r.dryRun(func() error {
		fileInfo, err = os.Stat(name)
		return err
	}, fmt.Sprintf("isdir %q", name))
	if err != nil {
		return false, err
	}
	return fileInfo.IsDir(), err
}

// Symlink is a wrapper around os.Symlink that handles options such as
// "verbose" or "dry run".
func (r *Run) Symlink(src, dst string) error {
	return r.helper(func() error { return os.Symlink(src, dst) }, fmt.Sprintf("ln -s %q %q", src, dst))
}

// TempDir is a wrapper around ioutil.TempDir that handles options
// such as "verbose" or "dry run".
func (r *Run) TempDir(dir, prefix string) (string, error) {
	var err error
	if dir == "" {
		dir = os.Getenv("TMPDIR")
	}
	tmpDir := fmt.Sprintf("%v%c%vXXXXXX", dir, os.PathSeparator, prefix)
	tmpDir = filepath.Clean(tmpDir)
	r.helper(func() error {
		tmpDir, err = ioutil.TempDir(dir, prefix)
		return err
	}, fmt.Sprintf("mkdir -p %q", tmpDir))
	return tmpDir, err
}

// WriteFile is a wrapper around ioutil.WriteFile that handles options
// such as "verbose" or "dry run".
func (r *Run) WriteFile(filename string, data []byte, perm os.FileMode) error {
	return r.helper(func() error { return ioutil.WriteFile(filename, data, perm) }, fmt.Sprintf("write %q", filename))
}
