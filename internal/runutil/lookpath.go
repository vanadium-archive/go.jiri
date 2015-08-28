// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// isExistingExecutable checks whether the given path points to an
// existing executable.
func isExistingExecutable(path string) (bool, error) {
	if fileInfo, err := os.Stat(path); err == nil {
		if mode := fileInfo.Mode(); !mode.IsDir() && (mode.Perm()&os.FileMode(0111)) != 0 {
			return true, nil
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("Stat(%v) failed: %v", path, err)
	}
	return false, nil
}

// LookPath inputs a command name and searches the PATH environment
// variable of the snapshot for an executable file that would be run
// had this command actually been invoked. The function returns an
// absolute path to the executable file. In other words, LookPath
// implements functionality similar to the UNIX "which" command
// relative to the snapshot environment.
func LookPath(file string, env map[string]string) (string, error) {
	if filepath.IsAbs(file) {
		ok, err := isExistingExecutable(file)
		if err != nil {
			return "", err
		} else if ok {
			return file, nil
		}
		return "", fmt.Errorf("failed to find %v", file)
	}
	envPath := env["PATH"]
	for _, dir := range strings.Split(envPath, string(os.PathListSeparator)) {
		path := filepath.Join(dir, file)
		ok, err := isExistingExecutable(path)
		if err != nil {
			return "", err
		} else if ok {
			if !filepath.IsAbs(path) {
				var err error
				path, err = filepath.Abs(path)
				if err != nil {
					return "", fmt.Errorf("Abs(%v) failed: %v", path, err)
				}
			}
			return path, nil
		}
	}
	return "", fmt.Errorf("failed to find %v in %v", file, envPath)
}
