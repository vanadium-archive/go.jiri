// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles_test

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"v.io/jiri/profiles"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/jiri/util"
	"v.io/x/lib/envvar"
)

func TestConfigHelper(t *testing.T) {
	ctx := tool.NewDefaultContext()
	root, err := project.JiriRoot()
	if err != nil {
		t.Fatal(err)
	}
	ch, err := profiles.NewConfigHelper(ctx, profiles.UseProfiles, filepath.Join(root, "release/go/src/v.io/jiri/profiles/testdata/m2.xml"))
	if err != nil {
		t.Fatal(err)
	}
	ch.Vars = envvar.VarsFromOS()
	ch.Delete("CGO_CFLAGS")
	native, _ := profiles.NewTarget("native")
	ch.MergeEnvFromProfiles(profiles.JiriMergePolicies(), native, "go", "syncbase")
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
	t1.Set("cpu1-os1@1")
	t1.Env.Set("A=B C=D,B=C Z=Z")
	t2.Set("cpu1-os1@1")
	t2.Env.Set("A=Z,B=Z,Z=Z1")
	profiles.AddProfileTarget("a", *t1)
	profiles.AddProfileTarget("b", *t2)
	tmpdir, err := ioutil.TempDir(".", "pdb")
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "release", "go", "src", "v.io", "jiri", "profiles", tmpdir, "manifest")
	if err := profiles.Write(ctx, filename); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)
	ch, err := profiles.NewConfigHelper(ctx, profiles.UseProfiles, filename)
	if err != nil {
		t.Fatal(err)
	}
	ch.Vars = envvar.VarsFromSlice([]string{})
	t1Target, _ := profiles.NewTarget("cpu1-os1@1")
	ch.MergeEnvFromProfiles(map[string]profiles.MergePolicy{
		"A": profiles.AppendFlag,
		"B": profiles.UseLast,
		"Z": profiles.IgnoreBaseUseLast},
		t1Target, "a", "b")
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

func TestMergeEnv(t *testing.T) {
	base := []string{"FS1=A", "IF=A", "A=B", "B=A", "C=D", "P=A", "V=A", "P1=A", "V1=A", "IA=A", "IB=A", "IC=A", "ID=A", "IE=A", "IG1=A"}
	b := []string{"FS1=B", "FS2=B", "IF=B", "A=B1", "B=B", "C=D1", "P=B", "V=B", "P1=B", "V1=B", "W=X", "Y=Z", "GP=X", "IA=B", "IB=B", "IC=B", "ID=B", "IE=B", "IG2=A"}
	c := []string{"FS1=C", "FS2=C", "FS3=C", "A=BL", "B=C", "C=DL", "P=C", "V=C", "P1=C", "V1=C", "Y=ZL", "GP=XL", "IA=C", "IB=C", "IC=C", "ID=C", "IE=C", "IG3=B"}
	env := envvar.VarsFromSlice(base)

	policies := map[string]profiles.MergePolicy{
		"GP":  profiles.UseLast,
		"P":   profiles.PrependPath,
		"V":   profiles.PrependFlag,
		"P1":  profiles.AppendPath,
		"V1":  profiles.AppendFlag,
		"A":   profiles.IgnoreBaseUseLast,
		"B":   profiles.UseBaseIgnoreProfiles,
		"IA":  profiles.IgnoreBaseAppendPath,
		"IB":  profiles.IgnoreBaseAppendFlag,
		"IC":  profiles.IgnoreBasePrependPath,
		"ID":  profiles.IgnoreBasePrependFlag,
		"IE":  profiles.IgnoreBaseUseLast,
		"IF":  profiles.IgnoreBaseUseFirst,
		"IG1": profiles.IgnoreVariable,
		"IG2": profiles.IgnoreVariable,
		"IG3": profiles.IgnoreVariable,
		"C":   profiles.UseLast,
		"Y":   profiles.UseLast,
	}
	profiles.MergeEnv(policies, env, b, c)

	expected := []string{"B=A", "A=BL", "C=DL", "GP=XL", "P1=A:B:C", "P=C:B:A",
		"V1=A B C", "V=C B A", "W=X", "Y=ZL",
		"IA=B:C", "IB=B C", "IC=C:B", "ID=C B", "IE=C",
		"FS1=A", "FS2=B", "FS3=C", "IF=B",
	}
	sort.Strings(expected)
	if got, want := env.ToSlice(), expected; len(got) != len(want) {
		sort.Strings(got)
		t.Errorf("got: %v", got)
		t.Errorf("want: %v", want)
		t.Errorf("got %v, want %v", len(got), len(want))
	}
	for _, g := range env.ToSlice() {
		found := false
		for _, w := range expected {
			if g == w {
				found = true
			}
		}
		if !found {
			t.Errorf("failed to find %v in %v", g, expected)
		}
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

	ch, err := profiles.NewConfigHelper(ctx, profiles.UseProfiles, filepath.Join(jiriRoot, "profiles-manifest"))
	if err != nil {
		t.Fatal(err)
	}

	var got, want string
	switch name {
	case "GOPATH":
		want = "GOPATH=" + filepath.Join(jiriRoot, "test")
		got = ch.GoPath()
	case "VDLPATH":
		// Make a fake src directory.
		want = filepath.Join(jiriRoot, "test", "src")
		if err := ctx.Run().MkdirAll(want, 0755); err != nil {
			t.Fatalf("%v", err)
		}
		want = "VDLPATH=" + want
		got = ch.VDLPath()
	}
	if got != want {
		t.Fatalf("unexpected value: got %v, want %v", got, want)
	}
}

func TestGoPath(t *testing.T) {
	testSetPathHelper(t, "GOPATH")
}

func TestVDLPath(t *testing.T) {
	testSetPathHelper(t, "VDLPATH")
}

func TestMergePolicyFlags(t *testing.T) {
	mp := profiles.MergePolicies{}
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.Var(mp, "p", mp.Usage())
	all := []string{"-p=:a", "-p=+b", "-p=^c", "-p=^:d", "-p=^e:", "-p=^+f", "-p=^g+", "-p=last*", "-p=xx:", "-p=yy+", "-p=zz^"}
	if err := fs.Parse(all); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		k string
		p profiles.MergePolicy
	}{
		{"a", profiles.AppendPath},
		{"b", profiles.AppendFlag},
		{"c", profiles.IgnoreBaseUseFirst},
		{"d", profiles.IgnoreBaseAppendPath},
		{"e", profiles.IgnoreBasePrependPath},
		{"f", profiles.IgnoreBaseAppendFlag},
		{"g", profiles.IgnoreBasePrependFlag},
		{"last", profiles.UseLast},
		{"xx", profiles.PrependPath},
		{"yy", profiles.PrependFlag},
		{"zz", profiles.UseBaseIgnoreProfiles},
	} {
		if got, want := mp[c.k], c.p; got != want {
			t.Errorf("(%s) got %v, want %v", c.k, got, want)
		}
	}

	mp = profiles.MergePolicies{}
	fs1 := flag.NewFlagSet("test1", flag.ContinueOnError)
	fs1.Var(mp, "p", mp.Usage())
	if err := fs1.Parse([]string{"-p=yy+,zz^"}); err != nil {
		t.Fatal(err)
	}
	if got, want := len(mp), 2; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	for i, cl := range append(all, "-p=+b,^c,zz^") {
		mp := profiles.MergePolicies{}
		fs := flag.NewFlagSet(fmt.Sprintf("t%d", i), flag.ContinueOnError)
		fs.Var(mp, "p", mp.Usage())
		err := fs.Parse([]string{cl})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := "-p="+mp.String(), cl; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
	}
}
