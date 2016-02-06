// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package example

import (
	"flag"
	"fmt"
	"path/filepath"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesutil"
)

type exampleManager struct {
	installer, name, root string
	versionInfo           *profiles.VersionInfo
	profile               *profiles.Profile
}

func newExampleMgr(installer, name string) *exampleManager {
	supported := map[string]interface{}{
		"2": nil,
		"4": nil,
		"3": nil,
	}
	return &exampleManager{
		installer:   installer,
		name:        name,
		versionInfo: profiles.NewVersionInfo("example", supported, "3"),
	}
}

func New(installer, name string) profiles.Manager {
	return newExampleMgr(installer, name)
}

func (eg *exampleManager) Name() string {
	return eg.name
}

func (eg *exampleManager) Installer() string {
	return eg.installer
}

func (eg *exampleManager) Info() string {
	return `
The example profile is for testing/example purposes only
`
}

func (eg *exampleManager) VersionInfo() *profiles.VersionInfo {
	return eg.versionInfo
}

func (eg *exampleManager) String() string {
	return fmt.Sprintf("Profile: %s installed by %s: %s\n", eg.name, eg.installer, eg.versionInfo)
}

func (eg *exampleManager) AddFlags(*flag.FlagSet, profiles.Action) {
}

func (eg *exampleManager) filename(root jiri.RelPath, target profiles.Target) jiri.RelPath {
	r := root.Join("eg")
	dir := target.TargetSpecificDirname()
	return r.Join(dir)
}

func (eg *exampleManager) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	version, err := eg.versionInfo.Select(target.Version())
	if err != nil {
		return err
	}
	target.SetVersion(version)
	dir := eg.filename(root, target).Abs(jirix)
	if err := jirix.NewSeq().
		MkdirAll(dir, profilesutil.DefaultDirPerm).
		WriteFile(filepath.Join(dir, "version"), []byte(version), profilesutil.DefaultFilePerm).
		WriteFile(filepath.Join(dir, version), []byte(version), profilesutil.DefaultFilePerm).
		Done(); err != nil {
		return err
	}
	eg.profile = pdb.InstallProfile(eg.installer, eg.name, string(root))
	target.InstallationDir = string(root)
	return pdb.AddProfileTarget(eg.installer, eg.name, target)
}

func (eg *exampleManager) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	version, err := eg.versionInfo.Select(target.Version())
	if err != nil {
		return err
	}
	dir := eg.filename(root, target).Abs(jirix)
	if err := jirix.NewSeq().WriteFile(filepath.Join(dir, "version"), []byte(version), profilesutil.DefaultFilePerm).
		WriteFile(filepath.Join(dir, version), []byte(version), profilesutil.DefaultFilePerm).
		Remove(filepath.Join(dir, version)).
		Done(); err != nil {
		return err
	}
	if pdb.RemoveProfileTarget(eg.installer, eg.name, target) {
		eg.profile = nil
	}
	return nil
}
