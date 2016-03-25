// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles_test

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"v.io/jiri/profiles"
)

func ExampleProfileTarget() {
	var target profiles.Target
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	profiles.RegisterTargetAndEnvFlags(flags, &target)
	flags.Parse([]string{"--target=arm-linux", "--env=A=B,C=D", "--env=E=F"})
	fmt.Println(target.String())
	fmt.Println(target.DebugString())
	// Output:
	// arm-linux@
	// arm-linux@ dir: --env=A=B,C=D,E=F envvars:[]
}

func TestProfileTargetArgs(t *testing.T) {
	for i, c := range []struct {
		arg, arch, os, version string
		err                    bool
	}{
		{"a-b", "a", "b", "", false},
		{"a-b@3", "a", "b", "3", false},
		{"", "", "", "", true},
		{"a", "", "", "", true},
		{"a-", "", "", "", true},
		{"-a", "", "", "", true},
		{"a-", "", "", "", true},
	} {
		target := &profiles.Target{}
		if err := target.Set(c.arg); err != nil {
			if !c.err {
				t.Errorf("%d: %v", i, err)
			}
			continue
		}
		if got, want := target.Arch(), c.arch; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
		if got, want := target.OS(), c.os; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
	}
}

func TestProfileEnvArgs(t *testing.T) {
	for i, c := range []struct {
		args []string
		env  string
		err  bool
	}{
		{[]string{}, "", false},
		{[]string{"a="}, "a=", false},
		{[]string{"a=b"}, "a=b", false},
		{[]string{"a=b,c=d"}, "a=b,c=d", false},
		{[]string{"a=b", "c=d"}, "a=b,c=d", false},
		{[]string{"=b"}, "", true},
		{[]string{"b"}, "", true},
	} {
		target := &profiles.Target{}
		for _, arg := range c.args {
			if err := target.Env.Set(arg); err != nil {
				if !c.err {
					t.Errorf("%d: %v", i, err)
				}
				continue
			}
		}
		if got, want := target.Env.String(), c.env; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
	}
}

func TestCopyCommandLineEnv(t *testing.T) {
	env := "A=B,C=D"
	target, _ := profiles.NewTarget("a=a-o", env)
	clenv := target.CommandLineEnv()
	if got, want := strings.Join(clenv.Vars, ","), env; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	// Make sure we can't mutate the command line env stored in target.
	clenv.Vars[0] = "oops"
	clenv2 := target.CommandLineEnv()
	if got, want := strings.Join(clenv2.Vars, ","), env; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTargetEquality(t *testing.T) {
	type s []string
	for i, c := range []struct {
		args1, args2 []string
		equal        bool
	}{
		{s{""}, s{""}, true},
		{s{"--t=a-b"}, s{"--t=a-b"}, true},
		{s{"--t=a-b@foo"}, s{"--t=a-b@foo"}, true},
		{s{"--t=a-b@foo"}, s{"--t=a-b"}, false},
		{s{"--t=a-b", "-e=a=b,c=d"}, s{"--t=a-b", "--e=c=d,a=b"}, true},
		{s{"--t=a-b", "-e=a=b,c=d"}, s{"--t=a-b", "--e=a=b,c=d"}, true},
		{s{"--t=a-b", "-e=a=b,c=d"}, s{"--t=a-b", "--e=c=d", "--e=a=b"}, true},
		{s{"--t=a-b"}, s{"--t=c-b"}, false},
		{s{"--t=a-b", "-e=a=b,c=d"}, s{"--t=c-b"}, false},
	} {
		t1, t2 := &profiles.Target{}, &profiles.Target{}
		t1.Env.Vars = []string{"Oh"} // Env vars don't matter.
		fl1 := flag.NewFlagSet("test1", flag.ContinueOnError)
		fl2 := flag.NewFlagSet("test2", flag.ContinueOnError)
		fl1.Var(t1, "t", t1.Usage())
		fl1.Var(&t1.Env, "e", t1.Env.Usage())
		fl2.Var(t2, "t", t2.Usage())
		fl2.Var(&t2.Env, "e", t2.Env.Usage())
		if err := fl1.Parse(c.args1); err != nil {
			t.Errorf("%d: %v\n", i, err)
			continue
		}
		if err := fl2.Parse(c.args2); err != nil {
			t.Errorf("%d: %v\n", i, err)
			continue
		}
		if got, want := t1.Match(t2), c.equal; got != want {
			t.Errorf("%v --- %v", t1, t2)
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
	}
}

func TestTargetVersion(t *testing.T) {
	t1, t2 := &profiles.Target{}, &profiles.Target{}
	t1.Set("cpu,os")
	t2.Set("cpu,os")
	if got, want := t1.Match(t2), true; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	t2.Set("cpu,os@var")
	if got, want := t1.Match(t2), true; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	t2.Set("cpu,os")
	t1.Set("cpu,os@baz")
	if got, want := t1.Match(t2), false; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDefaults(t *testing.T) {
	t1 := &profiles.Target{}
	native := fmt.Sprintf("%s-%s@", runtime.GOARCH, runtime.GOOS)
	if got, want := t1.String(), native; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	t1.Set("cpu-os")
	if got, want := t1.String(), "cpu-os@"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFindTarget(t *testing.T) {
	t1 := &profiles.Target{}
	t1.Set("a-o")
	ts := []*profiles.Target{t1}
	prev := os.Getenv("GOARCH")
	if len(prev) > 0 {
		// Clear GOARCH so that DefaultTarget is not set.
		os.Setenv("GOARCH", "")
		defer os.Setenv("GOARCH", prev)
	}
	def := profiles.DefaultTarget()
	if got, want := profiles.FindTargetWithDefault(ts, &def), t1; !got.Match(want) {
		t.Errorf("got %v, want %v", got, want)
	}
	t2 := &profiles.Target{}
	t2.Set("a-o1")
	ts = append(ts, t2)
	if got := profiles.FindTarget(ts, &def); got != nil {
		t.Errorf("got %v, want nil", got)
	}

	w := &profiles.Target{}
	w.Set("a-o")
	if got, want := profiles.FindTarget(ts, w), t1; !got.Match(want) {
		t.Errorf("got %v, want %v", got, want)
	}

	w.Set("a-o1")
	if got, want := profiles.FindTarget(ts, w), t2; !got.Match(want) {
		t.Errorf("got %v, want %v", got, want)
	}

	w.Set("c-d")
	if got := profiles.FindTarget(ts, w); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestOrderedTargets(t *testing.T) {
	for i, c := range []struct {
		a, b string
		r    bool
	}{
		{"x-b", "y-b", true},
		{"x-b", "w-b", false},
		{"x-b", "x-a", false},
		{"x-b", "x-c", true},
		{"x-b", "x-b@1", true},
		{"x-b@1", "x-b@", false},
		{"x-b@1", "x-b@2", false},
		{"x-b@12", "x-b@2", true},
		{"x-b@2", "x-b@1", true},
		{"x-b@1.2", "x-b@1.1", true},
		{"x-b@1.2.c", "x-b@1.2.b", true},
		{"x-b@1.2.1", "x-b@1.2", true},
		{"x-b@1.2.1.3", "x-b@1.2", true},
		{"x-b@2.2", "x-b@1.2.3.4", true},
		{"x-b", "x-b", false},
	} {
		a, err := profiles.NewTarget(c.a, "")
		if err != nil {
			t.Fatal(err)
		}
		b, _ := profiles.NewTarget(c.b, "")
		if err != nil {
			t.Fatal(err)
		}
		if got, want := a.Less(&b), c.r; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
	}

	ol := profiles.Targets{}
	data := []string{"a-b@2", "x-y", "a-b@12", "a-b@3", "a-b@0", "x-y@3", "x-y@2"}
	for _, s := range data {
		target, err := profiles.NewTarget(s)
		if err != nil {
			t.Fatal(err)
		}
		ol = profiles.InsertTarget(ol, &target)
	}
	if got, want := len(ol), len(data); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, _ := range ol[:len(ol)-1] {
		j := i + 1
		if !ol.Less(i, j) {
			t.Errorf("%v is not less than %v", ol[i], ol[j])
		}
	}
	if got, want := ol[0].String(), "a-b@12"; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got, want := ol[len(ol)-1].String(), "x-y@2"; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
	t2, _ := profiles.NewTarget("a-b@12")
	ol = profiles.RemoveTarget(ol, &t2)
}
