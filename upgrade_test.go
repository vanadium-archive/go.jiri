// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"v.io/jiri/jiri"
	"v.io/x/lib/gosh"
)

type upgradeTestCase struct {
	Args        []string
	Exist       bool
	Local, Want string
	Stderr      string
}

func TestUpgrade(t *testing.T) {
	tests := []upgradeTestCase{
		{
			Stderr: `must specify upgrade kind`,
		},
		{
			Args:   []string{"foo"},
			Stderr: `unknown upgrade kind "foo"`,
		},
		// Test v23 upgrades.
		{
			Args:   []string{"v23"},
			Exist:  true,
			Stderr: `.jiri_manifest already exists`,
		},
		{
			Args: []string{"v23"},
			Want: `<manifest>
  <imports>
    <import manifest="v2/default" remote="https://vanadium.googlesource.com/manifest"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"v23"},
			Local: `<manifest>
  <imports>
    <import name="default"/>
  </imports>
</manifest>
`,
			Want: `<manifest>
  <imports>
    <import manifest="v2/default" remote="https://vanadium.googlesource.com/manifest"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"v23"},
			Local: `<manifest>
  <imports>
    <import name="private"/>
  </imports>
</manifest>
`,
			Want: `<manifest>
  <imports>
    <import manifest="v2/private" remote="https://vanadium.googlesource.com/manifest"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"v23"},
			Local: `<manifest>
  <imports>
    <import name="private"/>
    <import name="infrastructure"/>
    <import name="default"/>
  </imports>
</manifest>
`,
			Want: `<manifest>
  <imports>
    <import manifest="v2/private" remote="https://vanadium.googlesource.com/manifest"/>
    <fileimport file="manifest/v2/infrastructure"/>
    <fileimport file="manifest/v2/default"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"v23"},
			Local: `<manifest>
  <imports>
    <import name="default"/>
    <import name="infrastructure"/>
    <import name="private"/>
  </imports>
</manifest>
`,
			Want: `<manifest>
  <imports>
    <import manifest="v2/default" remote="https://vanadium.googlesource.com/manifest"/>
    <fileimport file="manifest/v2/infrastructure"/>
    <fileimport file="manifest/v2/private"/>
  </imports>
</manifest>
`,
		},
		// Test fuchsia upgrades.
		{
			Args:   []string{"fuchsia"},
			Exist:  true,
			Stderr: `.jiri_manifest already exists`,
		},
		{
			Args: []string{"fuchsia"},
			Want: `<manifest>
  <imports>
    <import manifest="v2/default" remote="https://github.com/effenel/fnl-start.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"fuchsia"},
			Local: `<manifest>
  <imports>
    <import name="default"/>
  </imports>
</manifest>
`,
			Want: `<manifest>
  <imports>
    <import manifest="v2/default" remote="https://github.com/effenel/fnl-start.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"fuchsia"},
			Local: `<manifest>
  <imports>
    <import name="private"/>
  </imports>
</manifest>
`,
			Want: `<manifest>
  <imports>
    <import manifest="v2/private" remote="https://github.com/effenel/fnl-start.git"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"fuchsia"},
			Local: `<manifest>
  <imports>
    <import name="private"/>
    <import name="infrastructure"/>
    <import name="default"/>
  </imports>
</manifest>
`,
			Want: `<manifest>
  <imports>
    <import manifest="v2/private" remote="https://github.com/effenel/fnl-start.git"/>
    <fileimport file="manifest/v2/infrastructure"/>
    <fileimport file="manifest/v2/default"/>
  </imports>
</manifest>
`,
		},
		{
			Args: []string{"fuchsia"},
			Local: `<manifest>
  <imports>
    <import name="default"/>
    <import name="infrastructure"/>
    <import name="private"/>
  </imports>
</manifest>
`,
			Want: `<manifest>
  <imports>
    <import manifest="v2/default" remote="https://github.com/effenel/fnl-start.git"/>
    <fileimport file="manifest/v2/infrastructure"/>
    <fileimport file="manifest/v2/private"/>
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
		if err := testUpgrade(opts, jiriTool, test); err != nil {
			t.Errorf("%v: %v", test.Args, err)
		}
	}
}

func testUpgrade(opts gosh.Opts, jiriTool string, test upgradeTestCase) error {
	sh := gosh.NewShell(opts)
	defer sh.Cleanup()
	jiriRoot := sh.MakeTempDir()
	sh.Pushd(jiriRoot)
	// Set up an existing file or local_manifest, if they were specified
	if test.Exist {
		if err := ioutil.WriteFile(".jiri_manifest", []byte("<manifest/>"), 0644); err != nil {
			return err
		}
	}
	if test.Local != "" {
		if err := ioutil.WriteFile(".local_manifest", []byte(test.Local), 0644); err != nil {
			return err
		}
	}
	// Run upgrade and check the error.
	sh.Vars[jiri.RootEnv] = jiriRoot
	cmd := sh.Cmd(jiriTool, append([]string{"upgrade"}, test.Args...)...)
	if test.Stderr != "" {
		cmd.ExitErrorIsOk = true
	}
	_, stderr := cmd.StdoutStderr()
	if got, want := stderr, test.Stderr; !strings.Contains(got, want) || (got != "" && want == "") {
		return fmt.Errorf("stderr got %q, want substr %q", got, want)
	}
	// Make sure the right file is generated.
	if test.Want != "" {
		data, err := ioutil.ReadFile(".jiri_manifest")
		if err != nil {
			return err
		}
		if got, want := string(data), test.Want; got != want {
			return fmt.Errorf("GOT\n%s\nWANT\n%s", got, want)
		}
	}
	// Make sure the .local_manifest file is backed up.
	if test.Local != "" && test.Stderr == "" {
		data, err := ioutil.ReadFile(".local_manifest.BACKUP")
		if err != nil {
			return fmt.Errorf("local manifest backup got error: %v", err)
		}
		if got, want := string(data), test.Local; got != want {
			return fmt.Errorf("local manifest backup GOT\n%s\nWANT\n%s", got, want)
		}
	}
	return nil
}

func TestUpgradeRevert(t *testing.T) {
	sh := gosh.NewShell(gosh.Opts{Fatalf: t.Fatalf, Logf: t.Logf})
	defer sh.Cleanup()
	jiriRoot := sh.MakeTempDir()
	sh.Pushd(jiriRoot)
	jiriTool := sh.BuildGoPkg("v.io/jiri")
	localData := `<manifest/>`
	jiriData := `<manifest>
  <imports>
    <import manifest="v2/default" remote="https://vanadium.googlesource.com/manifest"/>
  </imports>
</manifest>
`
	// Set up an existing local_manifest.
	if err := ioutil.WriteFile(".local_manifest", []byte(localData), 0644); err != nil {
		t.Errorf("couldn't write local manifest: %v", err)
	}
	// Run a regular upgrade first, and make sure files are as expected.
	sh.Vars[jiri.RootEnv] = jiriRoot
	sh.Cmd(jiriTool, "upgrade", "v23").Run()
	gotJiri, err := ioutil.ReadFile(".jiri_manifest")
	if err != nil {
		t.Errorf("couldn't read jiri manifest: %v", err)
	}
	if got, want := string(gotJiri), jiriData; got != want {
		t.Errorf("jiri manifest GOT\n%s\nWANT\n%s", got, want)
	}
	gotBackup, err := ioutil.ReadFile(".local_manifest.BACKUP")
	if err != nil {
		t.Errorf("couldn't read local manifest backup: %v", err)
	}
	if got, want := string(gotBackup), localData; got != want {
		t.Errorf("local manifest backup GOT\n%s\nWANT\n%s", got, want)
	}
	// Now run a revert, and make sure files are as expected.
	sh.Cmd(jiriTool, "upgrade", "-revert").Run()
	if _, err := os.Stat(".jiri_manifest"); !os.IsNotExist(err) {
		t.Errorf(".jiri_manifest still exists after revert: %v", err)
	}
	if _, err := os.Stat(".local_manifest.BACKUP"); !os.IsNotExist(err) {
		t.Errorf(".local_manifest.BACKUP still exists after revert: %v", err)
	}
	gotLocal, err := ioutil.ReadFile(".local_manifest")
	if err != nil {
		t.Errorf("couldn't read local manifest: %v", err)
	}
	if got, want := string(gotLocal), localData; got != want {
		t.Errorf("local manifest GOT\n%s\nWANT\n%s", got, want)
	}
}
