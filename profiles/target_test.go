// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles_test

import (
	"flag"
	"fmt"
	"runtime"
	"testing"

	"v.io/jiri/profiles"
)

func ExampleProfileTarget() {
	var target profiles.Target
	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.Var(&target, "target", target.Usage())
	flags.Var(&target.Env, "env", target.Env.Usage())
	flags.Parse([]string{"--target=name=arm-linux", "--env=A=B,C=D", "--env=E=F"})
	fmt.Println(target.String())
	// Output:
	// tag:name arch:arm os:linux version: installdir: env:[A=B C=D E=F]
}

func TestProfileTargetArgs(t *testing.T) {
	for i, c := range []struct {
		arg, tag, arch, os string
		err                bool
	}{
		{"a-b", "", "a", "b", false},
		{"t=a-b", "t", "a", "b", false},
		{"a", "a", "", "", false},
		{"", "", "", "", true},
		{"a-", "", "", "", true},
		{"-a", "", "", "", true},
		{"t=a", "", "", "", true},
		{"t=a-", "", "", "", true},
		{"t=-a", "", "", "", true},
	} {
		target := &profiles.Target{}
		if err := target.Set(c.arg); err != nil {
			if !c.err {
				t.Errorf("%d: %v", i, err)
			}
			continue
		}
		if got, want := target.Tag, c.tag; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
		if got, want := target.Arch, c.arch; got != want {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
		if got, want := target.OS, c.os; got != want {
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

func TestProfileEquality(t *testing.T) {
	type s []string
	for i, c := range []struct {
		args1, args2 []string
		equal        bool
	}{
		{s{""}, s{""}, true},
		{s{"--t=tag=a-b"}, s{"--t=tag=c-d"}, true}, // tag trumps all else.
		{s{"--t=a-b"}, s{"--t=a-b"}, true},
		{s{"--t=a-b", "-e=a=b,c=d"}, s{"--t=a-b", "--e=c=d,a=b"}, true},
		{s{"--t=a-b", "-e=a=b,c=d"}, s{"--t=a-b", "--e=a=b,c=d"}, true},
		{s{"--t=a-b", "-e=a=b,c=d"}, s{"--t=a-b", "--e=c=d", "--e=a=b"}, true},
		{s{"--t=a-b"}, s{"--t=c-b"}, false},
		{s{"--t=a-b", "-e=a=b,c=d"}, s{"--t=c-b"}, false},
	} {
		t1, t2 := &profiles.Target{}, &profiles.Target{}
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
		if got, want := t1.Equals(t2), c.equal; got != want {
			t.Errorf("%v --- %v", t1, t2)
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
	}
}

func TestTargetVersion(t *testing.T) {
	t1, t2 := &profiles.Target{}, &profiles.Target{}
	t1.Set("tag=cpu,os")
	t2.Set("tag=cpu,os")
	if got, want := t1.Equals(t2), true; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	t2.Version = "bar"
	if got, want := t1.Equals(t2), false; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDefaults(t *testing.T) {
	t1 := &profiles.Target{}
	native := fmt.Sprintf("tag:native arch:%s os:%s version: installdir: env:[]", runtime.GOARCH, runtime.GOOS)
	if got, want := t1.String(), native; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	t1.Set("tag=cpu-os")
	if got, want := t1.String(), "tag:tag arch:cpu os:os version: installdir: env:[]"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
