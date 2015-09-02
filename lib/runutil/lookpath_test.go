// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
)

func pathsMatch(t *testing.T, path1, path2 string) bool {
	eval1, err := filepath.EvalSymlinks(path1)
	if err != nil {
		t.Fatalf("EvalSymlinks(%v) failed: %v", path1, err)
	}
	eval2, err := filepath.EvalSymlinks(path2)
	if err != nil {
		t.Fatalf("EvalSymlinks(%v) failed: %v", path2, err)
	}
	return eval1 == eval2
}

// TestLookPathCommandOK checks that LookPath() succeeds when given an
// existing command.
func TestLookPathCommandOK(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	cmd := "jiri-unlikely-binary-name"
	absPath := filepath.Join(tmpDir, cmd)

	tmpFile, err := os.OpenFile(absPath, os.O_CREATE, os.FileMode(0755))
	if err != nil {
		t.Fatalf("OpenFile(%v) failed: %v", absPath, err)
	}
	defer tmpFile.Close()
	command := filepath.Base(absPath)
	env := map[string]string{"PATH": tmpDir}
	got, err := LookPath(command, env)
	if err != nil {
		t.Fatalf("LookPath(%v) failed: %v", command, err)
	}
	if want := absPath; !pathsMatch(t, got, want) {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

// TestLookPathCommandFail checks that LookPath() fails when given a
// non-existing command.
func TestLookPathCommandFail(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	absPath := filepath.Join(tmpDir, "jiri-unlikely-binary-name")
	env := map[string]string{"PATH": tmpDir}
	if _, err := LookPath(filepath.Base(absPath), env); err == nil {
		t.Fatalf("LookPath(%v) did not fail when it should", filepath.Base(absPath))
	}
}

// TestLookPathAbsoluteOk checks that LookPath() succeeds when given
// an existing absolute path.
func TestLookPathAbsoluteOK(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	absPath := filepath.Join(tmpDir, "jiri-unlikely-binary-name")
	tmpFile, err := os.OpenFile(absPath, os.O_CREATE, os.FileMode(0755))
	if err != nil {
		t.Fatalf("OpenFile(%v) failed: %v", absPath, err)
	}
	defer tmpFile.Close()
	env := map[string]string{"PATH": tmpDir}
	got, err := LookPath(absPath, env)
	if err != nil {
		t.Fatalf("LookPath(%v) failed: %v", absPath, err)
	}
	if want := absPath; !pathsMatch(t, got, want) {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

// TestLookPathAbsoluteFail checks that LookPath() fails when given a
// non-existing absolute path.
func TestLookPathAbsoluteFail(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	absPath := filepath.Join(tmpDir, "jiri-unlikely-binary-name")
	env := map[string]string{"PATH": tmpDir}
	if _, err := LookPath(absPath, env); err == nil {
		t.Fatalf("LookPath(%v) did not fail when it should", absPath)
	}
}

// TestLookPathAbsoluteExecFail checks that LookPath() fails when
// given an existing absolute path to a non-executable file.
func TestLookPathAbsoluteExecFail(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	absPath := filepath.Join(tmpDir, "jiri-unlikely-binary-name")
	tmpFile, err := os.OpenFile(absPath, os.O_CREATE, os.FileMode(0644))
	if err != nil {
		t.Fatalf("OpenFile(%v) failed: %v", absPath, err)
	}
	defer tmpFile.Close()
	env := map[string]string{"PATH": tmpDir}
	if _, err := LookPath(absPath, env); err == nil {
		t.Fatalf("LookPath(%v) did not fail when it should", absPath)
	}
}

// TestLookPathRelativeOK checks that LookPath() succeeds when given
// an existing relative path.
func TestLookPathRelativeOK(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	cmd := "jiri-unlikely-binary-name"
	absPath := filepath.Join(tmpDir, cmd)
	tmpFile, err := os.OpenFile(absPath, os.O_CREATE, os.FileMode(0755))
	if err != nil {
		t.Fatalf("OpenFile(%v) failed: %v", absPath, err)
	}
	defer tmpFile.Close()
	relPath := "." + string(os.PathSeparator) + filepath.Base(absPath)
	env := map[string]string{"PATH": tmpDir}
	got, err := LookPath(relPath, env)
	if err != nil {
		t.Fatalf("LookPath(%v) failed: %v", relPath, err)
	}
	if want := absPath; !pathsMatch(t, got, want) {
		t.Fatalf("unexpected output: got %v, want %v", got, want)
	}
}

// TestLookPathRelativeFail checks that LookPath() fails when given a
// non-existing relative path.
func TestLookPathRelativeFail(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	absPath := filepath.Join(tmpDir, "jiri-unlikely-binary-name")
	relPath := "." + string(os.PathSeparator) + filepath.Base(absPath)
	env := map[string]string{"PATH": tmpDir}
	if _, err := LookPath(relPath, env); err == nil {
		t.Fatalf("LookPath(%v) did not fail when it should", relPath)
	}
}

// TestLookPathRelativeExecFail checks that LookPath() fails when
// given an existing relative path to a non-executable file.
func TestLookPathRelativeExecFail(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir(%v) failed: %v", tmpDir, err)
	}
	defer os.Chdir(cwd)
	absPath := filepath.Join(tmpDir, "jiri-unlikely-binary-name")
	tmpFile, err := os.OpenFile(absPath, os.O_CREATE, os.FileMode(0644))
	if err != nil {
		t.Fatalf("OpenFile(%v) failed: %v", absPath, err)
	}
	defer tmpFile.Close()
	relPath := "." + string(os.PathSeparator) + filepath.Base(absPath)
	env := map[string]string{"PATH": tmpDir}
	if _, err := LookPath(relPath, env); err == nil {
		t.Fatalf("LookPath(%v) did not fail when it should", relPath)
	}
}
