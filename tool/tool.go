// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tool

import (
	"flag"
)

// Version identifies the version of a tool.
var Version string = "manual-build"

// Name identifies the name of a tool.
var Name string = ""

var (
	// Flags for running commands.
	ColorFlag   bool
	VerboseFlag bool

	// Flags for working with projects.
	ManifestFlag string
)

// InitializeRunFlags initializes flags for running commands.
func InitializeRunFlags(flags *flag.FlagSet) {
	flags.BoolVar(&ColorFlag, "color", true, "Use color to format output.")
	flags.BoolVar(&VerboseFlag, "v", false, "Print verbose output.")
}

// InitializeRunFlags initializes flags for working with projects.
func InitializeProjectFlags(flags *flag.FlagSet) {
	flags.StringVar(&ManifestFlag, "manifest", "", "Name of the project manifest.")
}
