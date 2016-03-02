// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"v.io/jiri"
	"v.io/x/lib/gosh"
)

type importTestCase struct {
	Args           []string
	Filename       string
	Exist, Want    string
	Stdout, Stderr string
}

func TestImport(t *testing.T) {
	tests := []importTestCase{
		{
			Stderr: `wrong number of arguments`,
		},
		{
			Args:   []string{"a"},
			Stderr: `wrong number of arguments`,
		},
		{
			Args:   []string{"a", "b", "c"},
			Stderr: `wrong number of arguments`,
		},
		// Remote imports, default append behavior
		{
			Args: []string{"-name=name", "-remote-branch=remotebranch", "-root=root", "foo", "https://github.com/new.git"},
			Want: `<manifest>
  <imports>
    <import manifest="foo" name="name" remote="https://github.com/new.git" remotebranch="remotebranch" root="root"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"foo", "https://github.com/new.git"},
			Want: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args:     []string{"-out=file", "foo", "https://github.com/new.git"},
			Filename: `file`,
			Want: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"-out=-", "foo", "https://github.com/new.git"},
			Stdout: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"foo", "https://github.com/new.git"},
			Exist: `<manifest>
  <imports>
    <import manifest="bar" remote="https://github.com/orig.git"/>
  </imports>
</manifest>
`,
			Want: `<manifest>
  <imports>
    <import manifest="bar" remote="https://github.com/orig.git"/>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		// Remote imports, explicit overwrite behavior
		{
			Args: []string{"-overwrite", "foo", "https://github.com/new.git"},
			Want: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args:     []string{"-overwrite", "-out=file", "foo", "https://github.com/new.git"},
			Filename: `file`,
			Want: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"-overwrite", "-out=-", "foo", "https://github.com/new.git"},
			Stdout: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"-overwrite", "foo", "https://github.com/new.git"},
			Exist: `<manifest>
  <imports>
    <import manifest="bar" remote="https://github.com/orig.git"/>
  </imports>
</manifest>
`,
			Want: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
	}
	sh := gosh.NewShell(t)
	defer sh.Cleanup()
	jiriTool := gosh.BuildGoPkg(sh, sh.MakeTempDir(), "v.io/jiri/cmd/jiri")
	for _, test := range tests {
		if err := testImport(t, jiriTool, test); err != nil {
			t.Errorf("%v: %v", test.Args, err)
		}
	}
}

func testImport(t *testing.T, jiriTool string, test importTestCase) error {
	sh := gosh.NewShell(t)
	defer sh.Cleanup()
	tmpDir := sh.MakeTempDir()
	jiriRoot := filepath.Join(tmpDir, "root")
	if err := os.Mkdir(jiriRoot, 0755); err != nil {
		return err
	}
	sh.Pushd(jiriRoot)
	filename := test.Filename
	if filename == "" {
		filename = ".jiri_manifest"
	}
	// Set up manfile for the local file import tests.  It should exist in both
	// the tmpDir (for ../manfile tests) and jiriRoot.
	for _, dir := range []string{tmpDir, jiriRoot} {
		if err := ioutil.WriteFile(filepath.Join(dir, "manfile"), nil, 0644); err != nil {
			return err
		}
	}
	// Set up an existing file if it was specified.
	if test.Exist != "" {
		if err := ioutil.WriteFile(filename, []byte(test.Exist), 0644); err != nil {
			return err
		}
	}
	// Run import and check the error.
	sh.Vars[jiri.RootEnv] = jiriRoot
	cmd := sh.Cmd(jiriTool, append([]string{"import"}, test.Args...)...)
	if test.Stderr != "" {
		cmd.ExitErrorIsOk = true
	}
	stdout, stderr := cmd.StdoutStderr()
	if got, want := stdout, test.Stdout; !strings.Contains(got, want) || (got != "" && want == "") {
		return fmt.Errorf("stdout got %q, want substr %q", got, want)
	}
	if got, want := stderr, test.Stderr; !strings.Contains(got, want) || (got != "" && want == "") {
		return fmt.Errorf("stderr got %q, want substr %q", got, want)
	}
	// Make sure the right file is generated.
	if test.Want != "" {
		data, err := ioutil.ReadFile(filename)
		if err != nil {
			return err
		}
		if got, want := string(data), test.Want; got != want {
			return fmt.Errorf("GOT\n%s\nWANT\n%s", got, want)
		}
	}
	return nil
}
