// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package commandline_test

import (
	"fmt"
	"testing"

	"v.io/jiri/profiles/commandline"
	"v.io/jiri/profiles/reader"
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
	commandline.Reset()
	p := parent
	args := []string{"--profiles-db=foo", "--skip-profiles"}
	var rf commandline.ReaderFlagValues
	// If RegisterReaderCommandsUsingParent is called, the common reader
	// flags are hosted by the parent command.
	commandline.RegisterReaderCommandsUsingParent(&p, &rf, "")
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
	if got, want := rf.ProfilesMode, reader.SkipProfiles; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	commandline.Reset()
	p = parent
	commandline.RegisterReaderFlags(&p.Flags, &rf, "")
	if got, want := len(p.Children), 0; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if err := p.Flags.Parse(args); err != nil {
		t.Fatal(err)
	}
	if got, want := rf.DBFilename, "foo"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	commandline.Reset()
	p = parent
	// If RegisterReaderCommands is not called, the common reader
	// flags are hosted by the subcommands.
	commandline.RegisterReaderCommands(&p, "")
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
	commandline.Reset()
	p := parent
	var rf commandline.ReaderFlagValues
	commandline.RegisterReaderCommandsUsingParent(&p, &rf, "")
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

	commandline.Reset()
	p = parent
	commandline.RegisterReaderCommands(&p, "")
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
