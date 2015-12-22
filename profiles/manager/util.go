// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package manager

import (
	"fmt"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
)

// ensureAction ensures that the requested profile and target
// is installed/uninstalled, installing/uninstalling it if and only if necessary.
func ensureAction(jirix *jiri.X, pdb *profiles.DB, action profiles.Action, profile string, root jiri.RelPath, target profiles.Target) error {
	verb := ""
	switch action {
	case profiles.Install:
		verb = "install"
	case profiles.Uninstall:
		verb = "uninstall"
	default:
		return fmt.Errorf("unrecognised action %v", action)
	}
	if jirix.Verbose() || jirix.DryRun() {
		fmt.Fprintf(jirix.Stdout(), "%s %v %s\n", verb, action, target)
	}
	if t := pdb.LookupProfileTarget(profile, target); t != nil {
		if jirix.Verbose() {
			fmt.Fprintf(jirix.Stdout(), "%v %v is already %sed as %v\n", profile, target, verb, t)
		}
		return nil
	}
	mgr := LookupManager(profile)
	if mgr == nil {
		return fmt.Errorf("profile %v is not supported", profile)
	}
	version, err := mgr.VersionInfo().Select(target.Version())
	if err != nil {
		return err
	}
	target.SetVersion(version)
	if jirix.Verbose() || jirix.DryRun() {
		fmt.Fprintf(jirix.Stdout(), "%s %s %s\n", verb, profile, target.DebugString())
	}
	if action == profiles.Install {
		return mgr.Install(jirix, pdb, root, target)
	}
	return mgr.Uninstall(jirix, pdb, root, target)
}

// EnsureProfileTargetIsInstalled ensures that the requested profile and target
// is installed, installing it if only if necessary.
func EnsureProfileTargetIsInstalled(jirix *jiri.X, pdb *profiles.DB, profile string, root jiri.RelPath, target profiles.Target) error {
	return ensureAction(jirix, pdb, profiles.Install, profile, root, target)
}

// EnsureProfileTargetIsUninstalled ensures that the requested profile and target
// are no longer installed.
func EnsureProfileTargetIsUninstalled(jirix *jiri.X, pdb *profiles.DB, profile string, root jiri.RelPath, target profiles.Target) error {
	return ensureAction(jirix, pdb, profiles.Uninstall, profile, root, target)
}
