// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles

import "flag"

const (
	targetDefValue = "<runtime.GOARCH>-<runtime.GOOS>"
)

// RegisterTargetAndEnvFlags registers the commonly used --target and --env
// flags with the supplied FlagSet
func RegisterTargetAndEnvFlags(flags *flag.FlagSet, target *Target) {
	*target = DefaultTarget()
	flags.Var(target, "target", target.Usage())
	flags.Lookup("target").DefValue = targetDefValue
	flags.Var(&target.commandLineEnv, "env", target.commandLineEnv.Usage())
}

// RegisterProfilesFlag registers the --profiles flag
func RegisterProfilesFlag(flags *flag.FlagSet, profiles *string) {
	flags.StringVar(profiles, "profiles", "base,jiri", "a comma separated list of profiles to use")
}

// RegisterTargetFlag registers the commonly used --target flag with
// the supplied FlagSet.
func RegisterTargetFlag(flags *flag.FlagSet, target *Target) {
	*target = DefaultTarget()
	flags.Var(target, "target", target.Usage())
	flags.Lookup("target").DefValue = targetDefValue
}
