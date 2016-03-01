// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profilescmdline

import (
	"fmt"
	"strings"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/jiri/runutil"
)

// profileManager is implemented for both in-process and sub-command
// implemented profiles.
type profileManager interface {
	install(jirix *jiri.X, cl *installFlagValues, root jiri.RelPath) error
	uninstall(jirix *jiri.X, cl *uninstallFlagValues, root jiri.RelPath) error
	update(jirix *jiri.X, cl *updateFlagValues, root jiri.RelPath) error
	cleanup(jirix *jiri.X, cl *cleanupFlagValues, root jiri.RelPath) error
	mgrName() string
}

func newProfileManager(name string, db *profiles.DB) profileManager {
	installer, profile := profiles.SplitProfileName(name)
	installer = strings.TrimSpace(installer)
	if len(installer) == 0 || installer == profileInstaller {
		return &inproc{installer, profile, name, db}
	}
	return &subcommand{installer, profile, name, db}
}

type inproc struct {
	installer, name, qname string
	db                     *profiles.DB
}

func (ip *inproc) mgrName() string {
	return ip.qname
}

func (ip *inproc) install(jirix *jiri.X, cl *installFlagValues, root jiri.RelPath) error {
	mgr := profilesmanager.LookupManager(ip.qname)
	if mgr == nil {
		return fmt.Errorf("profile %v is not available via this installer %q", ip.qname, ip.installer)
	}
	def, err := targetAtDefaultVersion(mgr, cl.target)
	if err != nil {
		return err
	}
	err = mgr.Install(jirix, ip.db, root, def)
	logResult(jirix, "Install", mgr, def, err)
	return err
}

func (ip *inproc) uninstall(jirix *jiri.X, cl *uninstallFlagValues, root jiri.RelPath) error {
	profile := ip.db.LookupProfile(ip.installer, ip.name)
	if profile == nil {
		fmt.Fprintf(jirix.Stdout(), "%s is not installed\n", ip.qname)
		return nil
	}
	mgr := profilesmanager.LookupManager(ip.qname)
	var targets []*profiles.Target
	if cl.allTargets {
		targets = profile.Targets()
	} else {
		def, err := targetAtDefaultVersion(mgr, cl.target)
		if err != nil {
			return err
		}
		targets = []*profiles.Target{&def}
	}
	for _, target := range targets {
		if err := mgr.Uninstall(jirix, ip.db, root, *target); err != nil {
			logResult(jirix, "Uninstall", mgr, *target, err)
			return err
		}
		logResult(jirix, "Uninstall", mgr, *target, nil)
	}
	return nil
}

func (ip *inproc) update(jirix *jiri.X, cl *updateFlagValues, root jiri.RelPath) error {
	profile := ip.db.LookupProfile(ip.installer, ip.name)
	if profile == nil {
		// silently ignore uninstalled profile.
		return nil
	}
	mgr := profilesmanager.LookupManager(ip.qname)
	vi := mgr.VersionInfo()
	for _, target := range profile.Targets() {
		if vi.IsTargetOlderThanDefault(target.Version()) {
			// Check if default target is already installed.
			defTarget := *target
			defTarget.SetVersion(vi.Default())
			if profiles.FindTarget(profile.Targets(), &defTarget) != nil {
				// Default target is already installed.  Skip.
				continue
			}
			if cl.verbose {
				fmt.Fprintf(jirix.Stdout(), "Updating %s %s from %q to %s\n", ip.qname, target, target.Version(), vi)
			}
			err := mgr.Install(jirix, ip.db, root, defTarget)
			logResult(jirix, "Update", mgr, defTarget, err)
			if err != nil {
				return err
			}
		} else {
			if cl.verbose {
				fmt.Fprintf(jirix.Stdout(), "%s %s at %q is up to date(%s)\n", ip.qname, target, target.Version(), vi)
			}
		}
	}
	return nil
}

func cleanupGC(jirix *jiri.X, db *profiles.DB, root jiri.RelPath, verbose bool, name string) error {
	mgr := profilesmanager.LookupManager(name)
	if mgr == nil {
		fmt.Fprintf(jirix.Stderr(), "%s is not linked into this binary\n", name)
		return nil
	}
	vi := mgr.VersionInfo()
	installer, profileName := profiles.SplitProfileName(name)
	profile := db.LookupProfile(installer, profileName)
	for _, target := range profile.Targets() {
		if vi.IsTargetOlderThanDefault(target.Version()) {
			err := mgr.Uninstall(jirix, db, root, *target)
			logResult(jirix, "Cleanup: -gc", mgr, *target, err)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func cleanupRmAll(jirix *jiri.X, db *profiles.DB, root jiri.RelPath) error {
	s := jirix.NewSeq()
	if err := s.AssertFileExists(db.Path()).Remove(db.Path()).Done(); err != nil && !runutil.IsNotExist(err) {
		return err
	} else {
		if err := s.AssertDirExists(db.Path()).RemoveAll(db.Path()).Done(); err != nil && !runutil.IsNotExist(err) {
			return err
		}
	}
	d := root.Abs(jirix)
	err := s.AssertDirExists(d).
		Run("chmod", "-R", "u+w", d).
		RemoveAll(d).
		Done()
	if err == nil || runutil.IsNotExist(err) {
		fmt.Fprintf(jirix.Stdout(), "success\n")
		return nil
	} else {
		fmt.Fprintf(jirix.Stdout(), "%v\n", err)
	}
	return err
}

func (ip *inproc) cleanup(jirix *jiri.X, cl *cleanupFlagValues, root jiri.RelPath) error {
	if cl.gc {
		if cl.verbose {
			fmt.Fprintf(jirix.Stdout(), "Removing targets older than the default version for %s\n", ip.qname)
		}
		if err := cleanupGC(jirix, ip.db, root, cl.verbose, ip.qname); err != nil {
			return fmt.Errorf("gc: %v", err)
		}
	}
	if cl.rmAll {
		if cl.verbose {
			fmt.Fprintf(jirix.Stdout(), "Removing profile manifest and all profile output files\n")
		}
		if err := cleanupRmAll(jirix, ip.db, root); err != nil {
			return err
		}
	}
	return nil
}

type subcommand struct {
	installer, profile, qname string
	db                        *profiles.DB
}

func (sc *subcommand) mgrName() string {
	return sc.qname
}

func (sc *subcommand) run(jirix *jiri.X, verb string, args []string) error {
	cl := []string{"profile-" + sc.installer, verb}
	cl = append(cl, args...)
	cl = append(cl, sc.qname)
	return jirix.NewSeq().Capture(jirix.Stdout(), jirix.Stderr()).Last("jiri", cl...)
}

func (sc *subcommand) install(jirix *jiri.X, cl *installFlagValues, root jiri.RelPath) error {
	return sc.run(jirix, "install", cl.args())
}

func (sc *subcommand) uninstall(jirix *jiri.X, cl *uninstallFlagValues, root jiri.RelPath) error {
	return sc.run(jirix, "uninstall", cl.args())
}

func (sc *subcommand) update(jirix *jiri.X, cl *updateFlagValues, root jiri.RelPath) error {
	return sc.run(jirix, "update", cl.args())
}

func (sc *subcommand) cleanup(jirix *jiri.X, cl *cleanupFlagValues, root jiri.RelPath) error {
	return sc.run(jirix, "cleanup", cl.args())
}

func logResult(jirix *jiri.X, action string, mgr profiles.Manager, target profiles.Target, err error) {
	fmt.Fprintf(jirix.Stdout(), "%s: %s %s: ", action, profiles.QualifiedProfileName(mgr.Installer(), mgr.Name()), target)
	if err == nil {
		fmt.Fprintf(jirix.Stdout(), "success\n")
	} else {
		fmt.Fprintf(jirix.Stdout(), "%v\n", err)
	}
}
