// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"v.io/jiri/jiri"
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
			Stderr: `must specify non-empty`,
		},
		{
			Args:   []string{"https://github.com/new.git"},
			Stderr: `must specify non-empty`,
		},
		// Default mode = append
		{
			Args: []string{"-name=name", "-path=path", "-remotebranch=remotebranch", "-revision=revision", "-root=root", "https://github.com/new.git", "foo"},
			Want: `<manifest>
  <imports>
    <import manifest="foo" root="root" name="name" path="path" remote="https://github.com/new.git" remotebranch="remotebranch" revision="revision"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"https://github.com/new.git", "foo"},
			Want: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args:     []string{"-out=file", "https://github.com/new.git", "foo"},
			Filename: `file`,
			Want: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"-out=-", "https://github.com/new.git", "foo"},
			Stdout: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"https://github.com/new.git", "foo"},
			Exist: `<manifest>
  <imports>
    <import manifest="bar" remote="https://github.com/exist.git"/>
  </imports>
</manifest>
`,
			Want: `<manifest>
  <imports>
    <import manifest="bar" remote="https://github.com/exist.git"/>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		// Explicit mode = append
		{
			Args: []string{"-mode=append", "https://github.com/new.git", "foo"},
			Want: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args:     []string{"-mode=append", "-out=file", "https://github.com/new.git", "foo"},
			Filename: `file`,
			Want: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"-mode=append", "-out=-", "https://github.com/new.git", "foo"},
			Stdout: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"-mode=append", "https://github.com/new.git", "foo"},
			Exist: `<manifest>
  <imports>
    <import manifest="bar" remote="https://github.com/exist.git"/>
  </imports>
</manifest>
`,
			Want: `<manifest>
  <imports>
    <import manifest="bar" remote="https://github.com/exist.git"/>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		// Explicit mode = overwrite
		{
			Args: []string{"-mode=overwrite", "https://github.com/new.git", "foo"},
			Want: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args:     []string{"-mode=overwrite", "-out=file", "https://github.com/new.git", "foo"},
			Filename: `file`,
			Want: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"-mode=overwrite", "-out=-", "https://github.com/new.git", "foo"},
			Stdout: `<manifest>
  <imports>
    <import manifest="foo" remote="https://github.com/new.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"-mode=overwrite", "https://github.com/new.git", "foo"},
			Exist: `<manifest>
  <imports>
    <import manifest="bar" remote="https://github.com/exist.git"/>
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
	opts := gosh.Opts{Fatalf: t.Fatalf, Logf: t.Logf}
	sh := gosh.NewShell(opts)
	defer sh.Cleanup()
	jiriTool := sh.BuildGoPkg("v.io/jiri")
	for _, test := range tests {
		if err := testImport(opts, jiriTool, test); err != nil {
			t.Errorf("%v: %v", test.Args, err)
		}
	}
}

func testImport(opts gosh.Opts, jiriTool string, test importTestCase) error {
	sh := gosh.NewShell(opts)
	defer sh.Cleanup()
	jiriRoot := sh.MakeTempDir()
	sh.Pushd(jiriRoot)
	defer sh.Popd()
	filename := test.Filename
	if filename == "" {
		filename = ".jiri_manifest"
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
