// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"v.io/jiri/profiles"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/jiri/util"
	"v.io/x/lib/envvar"
)

func TestConfigHelper(t *testing.T) {
	ctx := tool.NewDefaultContext()
	ch, err := profiles.NewConfigHelper(ctx, "release/go/src/v.io/jiri/profiles/testdata/m2.xml")
	if err != nil {
		t.Fatal(err)
	}
	ch.Vars = envvar.VarsFromOS()
	ch.Delete("CGO_CFLAGS")
	ch.SetEnvFromProfiles(profiles.CommonConcatVariables(), map[string]bool{}, "go,syncbase", profiles.Target{Tag: "native"})
	if got, want := ch.Get("CGO_CFLAGS"), "-IX -IY -IA -IB"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEnvFromTarget(t *testing.T) {
	profiles.Clear()
	root, _ := project.JiriRoot()
	ctx := tool.NewDefaultContext()
	profiles.InstallProfile("a", "root")
	profiles.InstallProfile("b", "root")
	t1, t2 := &profiles.Target{}, &profiles.Target{}
	t1.Set("t1=cpu1-os1")
	t1.Env.Set("A=B C=D, B=C Z=Z")
	t2.Set("t1=cpu1-os1")
	t2.Env.Set("A=Z,B=Z,Z=Z")
	profiles.AddProfileTarget("a", *t1)
	profiles.AddProfileTarget("b", *t2)
	tmpdir, err := ioutil.TempDir(".", "pdb")
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join("release", "go", "src", "v.io", "jiri", "profiles", tmpdir, "manifest")
	if err := profiles.Write(ctx, filepath.Join(root, filename)); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	ch, err := profiles.NewConfigHelper(ctx, filename)
	if err != nil {
		t.Fatal(err)
	}
	ch.Vars = envvar.VarsFromSlice([]string{})
	ch.SetEnvFromProfiles(map[string]string{"A": " "}, map[string]bool{"Z": true}, "a,b", profiles.Target{Tag: "t1"})
	vars := ch.ToMap()
	if got, want := len(vars), 3; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := ch.Get("A"), "B C=D Z"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := ch.Get("B"), "Z"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func testSetPathHelper(t *testing.T, name string) {
	profiles.Clear()
	ctx := tool.NewDefaultContext()

	// Setup a fake JIRI_ROOT.
	root, err := project.NewFakeJiriRoot(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(ctx); err != nil {
			t.Fatalf("%v", err)
		}
	}()

	// Create a test project and identify it as a Go workspace.
	if err := root.CreateRemoteProject(ctx, "test"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := root.AddProject(ctx, project.Project{
		Name:   "test",
		Path:   "test",
		Remote: root.Projects["test"],
	}); err != nil {
		t.Fatalf("%v", err)
	}
	if err := root.UpdateUniverse(ctx, false); err != nil {
		t.Fatalf("%v", err)
	}
	var config *util.Config
	switch name {
	case "GOPATH":
		config = util.NewConfig(util.GoWorkspacesOpt([]string{"test", "does/not/exist"}))
	case "VDLPATH":
		config = util.NewConfig(util.VDLWorkspacesOpt([]string{"test", "does/not/exist"}))
	}

	oldRoot, err := project.JiriRoot()
	if err := os.Setenv("JIRI_ROOT", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("JIRI_ROOT", oldRoot)

	if err := profiles.Write(ctx, filepath.Join(root.Dir, "profiles-manifest")); err != nil {
		t.Fatal(err)
	}

	if err := util.SaveConfig(ctx, config); err != nil {
		t.Fatalf("%v", err)
	}

	// Retrieve Jiri_ROOT through JiriRoot() to account for symlinks.
	jiriRoot, err := project.JiriRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}

	ch, err := profiles.NewConfigHelper(ctx, "profiles-manifest")
	if err != nil {
		t.Fatal(err)
	}
	ch.Vars = envvar.VarsFromOS()
	ch.Set(name, "")

	var want string
	switch name {
	case "GOPATH":
		want = filepath.Join(jiriRoot, "test")
		ch.SetGoPath()
	case "VDLPATH":
		// Make a fake src directory.
		want = filepath.Join(jiriRoot, "test", "src")
		if err := ctx.Run().MkdirAll(want, 0755); err != nil {
			t.Fatalf("%v", err)
		}
		ch.SetVDLPath()
	}
	if got := ch.Get(name); got != want {
		t.Fatalf("unexpected value: got %v, want %v", got, want)
	}
}

func TestSetGoPath(t *testing.T) {
	testSetPathHelper(t, "GOPATH")
}

func TestSetVdlPath(t *testing.T) {
	testSetPathHelper(t, "VDLPATH")
}

func TestMergeEnv(t *testing.T) {
	a := []string{"A=B", "C=D"}
	b := []string{"W=X", "Y=Z", "GP=X"}
	env := envvar.VarsFromSlice(a)
	profiles.MergeEnv(map[string]string{}, map[string]bool{"GP": true}, env, b)
	if got, want := len(env.ToSlice()), 4; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := env.Get("W"), "X"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	profiles.MergeEnv(map[string]string{"W": " "}, map[string]bool{"GP": true}, env, []string{"W=an option"})
	if got, want := len(env.ToSlice()), 4; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := env.Get("W"), "X an option"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
