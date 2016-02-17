// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"v.io/jiri/jiri"
	"v.io/jiri/profiles/profilescmdline"
	"v.io/x/lib/cmdline"
)

var cmdProfile = &cmdline.Command{
	Name:  "profile",
	Short: "Display information about installed profiles",
	Long:  "Display information about installed profiles and their configuration.",
}

func init() {
	profilescmdline.RegisterReaderCommands(cmdProfile, jiri.ProfilesDBDir)
	profilescmdline.RegisterManagementCommands(cmdProfile, "", jiri.ProfilesDBDir, jiri.ProfilesRootDir)
}
