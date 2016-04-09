// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profilescmdline_test

import (
	"fmt"
	"testing"

	"v.io/jiri/profiles/profilescmdline"
	"v.io/jiri/profiles/profilesreader"
	"v.io/x/lib/cmdline"
)

var parent = cmdline.Command{
	Name:     "test",
	Short:    "test",
	Long:     "test",
	ArgsName: "test",
	ArgsLong: "test",
}

func TestReaderParent(t *testing.T) {
	profilescmdline.Reset()
	p := parent
	args := []string{"--profiles-db=foo", "--skip-profiles"}
	var rf profilescmdline.ReaderFlagValues
	// If RegisterReaderCommandsUsingParent is called, the common reader
	// flags are hosted by the parent command.
	profilescmdline.RegisterReaderCommandsUsingParent(&p, &rf, "", "")
	if got, want := len(p.Children), 2; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if err := p.Children[0].Flags.Parse(args); err == nil {
		t.Errorf("this should have failed")
	}
	if err := p.Flags.Parse(args); err != nil {
		t.Error(err)
	}
	if got, want := rf.DBFilename, "foo"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := rf.ProfilesMode, profilesreader.SkipProfiles; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	profilescmdline.Reset()
	p = parent
	profilescmdline.RegisterReaderFlags(&p.Flags, &rf, "", "")
	if got, want := len(p.Children), 0; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if err := p.Flags.Parse(args); err != nil {
		t.Fatal(err)
	}
	if got, want := rf.DBFilename, "foo"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	profilescmdline.Reset()
	p = parent
	// If RegisterReaderCommands is not called, the common reader
	// flags are hosted by the subcommands.
	profilescmdline.RegisterReaderCommands(&p, "", "")
	if err := p.Flags.Parse(args); err == nil {
		t.Fatal(fmt.Errorf("this should have failed"))
	}
	if err := p.Children[0].Flags.Parse([]string{"--profiles=a,b"}); err != nil {
		t.Fatal(err)
	}
	// NOTE, that we can't access the actual values of the flags when they
	// are hosted by the subcommands.
}

func TestSubcommandFlags(t *testing.T) {
	profilescmdline.Reset()
	p := parent
	var rf profilescmdline.ReaderFlagValues
	profilescmdline.RegisterReaderCommandsUsingParent(&p, &rf, "", "")
	if got, want := len(p.Children), 2; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	args := []string{"--info", "Profile.Root"}
	if err := p.Flags.Parse(args); err == nil {
		t.Error("this should have failed")
	}
	if err := p.Children[0].Flags.Parse(args); err != nil {
		t.Error(err)
	}

	profilescmdline.Reset()
	p = parent
	profilescmdline.RegisterReaderCommands(&p, "", "")
	if got, want := len(p.Children), 2; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if err := p.Flags.Parse(args); err == nil {
		t.Error("this should have failed")
	}
	if err := p.Children[0].Flags.Parse(args); err != nil {
		t.Error(err)
	}
}
