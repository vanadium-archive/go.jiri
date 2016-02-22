// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// The following enables go generate to generate the doc.go file.
//go:generate go run $JIRI_ROOT/release/go/src/v.io/x/lib/cmdline/testdata/gendoc.go -env=CMDLINE_PREFIX=jiri .

package main

import (
	"v.io/jiri/jiri"
	"v.io/jiri/profiles/profilescmdline"
	"v.io/jiri/profiles/profilesmanager"

	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"

	// Add profile manager implementations here.
	"v.io/jiri/profiles/profilescmdline/internal/example"
)

// commandLineDriver implements the command line for the 'profile-v23'
// subcommand.
var commandLineDriver = &cmdline.Command{
	Name:  "profile-i2",
	Short: "Manage i2 profiles",
	Long:  profilescmdline.HelpMsg(),
}

func main() {
	profilesmanager.Register(example.New("i2", "eg"))
	profilescmdline.RegisterManagementCommands(commandLineDriver, true, "i2", jiri.ProfilesDBDir, jiri.ProfilesRootDir)
	tool.InitializeRunFlags(&commandLineDriver.Flags)
	cmdline.Main(commandLineDriver)
}
