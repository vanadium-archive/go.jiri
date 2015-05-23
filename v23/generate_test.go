// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"v.io/x/lib/cmdline"
)

// TestV23TestGenerate tests that "v23 test generate" works as expected.  For
// each "golden" source directory under ./testdata/generate/* we copy the
// contents into a tmpdir, then run "v23 test generate" against that tmpdir, and
// finally compare the generated files against the golden source directory.
func TestV23TestGenerate(t *testing.T) {
	// Create a tmpdir where all generated files will go.
	tmpdir, err := ioutil.TempDir("", "v23_test_gen_")
	if err != nil {
		t.Fatalf("TempDir failed: %v", err)
	}
	defer os.RemoveAll(tmpdir)
	// Set GOPATH so that the tmpdir appears first.
	oldGoPath := os.Getenv("GOPATH")
	newGoPath := strings.Join([]string{tmpdir, oldGoPath}, ":")
	if err := os.Setenv("GOPATH", newGoPath); err != nil {
		t.Fatalf(`Setenv("GOPATH", %q) failed: %v`, newGoPath, err)
	}
	defer os.Setenv("GOPATH", oldGoPath)
	// Read each test directory under ./testdata/generate.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	gendir := filepath.Join(tmpdir, "src", "v.io", "x", "devtools", "v23", "testdata", "generate")
	srcdir := filepath.Join(cwd, "testdata", "generate")
	infos, err := ioutil.ReadDir(srcdir)
	if err != nil {
		t.Fatal(err)
	}
	// Test generation of each test directory.
	var testDirs []string
	for _, info := range infos {
		if !info.IsDir() {
			continue
		}
		testGenerate(t, gendir, srcdir, info.Name())
		testDirs = append(testDirs, info.Name())
	}
	// Make sure that we really ran the tests against all our test directories.
	if got, want := testDirs, []string{"empty", "external_only", "has_main", "internal_only", "internal_transitive_external", "modules_and_v23", "modules_only", "one", "prefix_other", "transitive", "v23_only"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Ran tests against %v, want %v", got, want)
	}
}

func testGenerate(t *testing.T, gendir, srcdir, name string) {
	// Copy source files for "name" from srcdir to gendir.
	const pre = "prefix_"
	prefix := "v23"
	if strings.HasPrefix(name, pre) {
		prefix = name[len(pre):]
	}
	if err := copyAll(filepath.Join(gendir, name), filepath.Join(srcdir, name), prefix); err != nil {
		t.Fatal(err)
	}
	// Generate test files into gendir.
	env := cmdline.EnvFromOS()
	if err := cmdline.ParseAndRun(cmdTestGenerate, env, []string{"-prefix=" + prefix, "v.io/x/devtools/v23/testdata/generate/" + name}); err != nil {
		t.Fatal(err)
	}
	// Validate generated files.
	extFile := filepath.Join(name, prefix+externalSuffix)
	intFile := filepath.Join(name, prefix+internalSuffix)
	for _, f := range []string{extFile, intFile} {
		srcData, srcErr := ioutil.ReadFile(filepath.Join(srcdir, f))
		if srcErr != nil && !os.IsNotExist(srcErr) {
			t.Errorf("%s: Read src file failed: %v", f, srcErr)
		}
		genData, genErr := ioutil.ReadFile(filepath.Join(gendir, f))
		if genErr != nil && !os.IsNotExist(genErr) {
			t.Errorf("%s: Read gen file failed: %v", f, genErr)
		}
		if got, want := srcErr == nil, genErr == nil; got != want {
			t.Errorf("%s: Got src exist %v, want %v", f, got, want)
		}
		if got, want := srcData, genData; !bytes.Equal(got, want) {
			t.Errorf("%s: Got data %s, want %s", f, got, want)
		}
	}
}

// copyAll copies all files and directories from srcdir into dstdir, skipping
// generated files with the given prefix.
func copyAll(dstdir, srcdir, prefix string) error {
	if err := os.MkdirAll(dstdir, os.ModePerm); err != nil {
		return err
	}
	infos, err := ioutil.ReadDir(srcdir)
	if err != nil {
		return err
	}
	for _, info := range infos {
		name := info.Name()
		if name == prefix+externalSuffix || name == prefix+internalSuffix {
			// Skip generated files, based on the prefix.
			continue
		}
		dst, src := filepath.Join(dstdir, name), filepath.Join(srcdir, name)
		if info.IsDir() {
			// Copy directories recursively.
			return copyAll(dst, src, prefix)
		}
		// Copy files from src to dst.
		srcFile, err := os.Open(src)
		if err != nil {
			return err
		}
		dstFile, err := os.Create(dst)
		if err != nil {
			return err
		}
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return err
		}
	}
	return nil
}

// TestV23TestGenerateTestdata runs "go test" against all packages under
// testdata/generate.  These are normally skipped since they're under a testdata
// directory.
func TestV23TestGenerateTestdata(t *testing.T) {
	var out bytes.Buffer
	env := &cmdline.Env{Stdout: &out, Stderr: &out}
	if err := runGo(env, []string{"test", "./testdata/generate/...", "-v", "-v23.tests"}); err != nil {
		t.Log(out.String())
		t.Errorf("tests under testdata/generate failed: %v", err)
	}
}
