// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profilesmanager_test

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/profiles/profilesmanager"
	"v.io/jiri/tool"
)

type myNewProfileMgr struct {
	installer, name, root string
	versionInfo           *profiles.VersionInfo
	profile               *profiles.Profile
}

func newProfileMgr(installer, name string) *myNewProfileMgr {
	supported := map[string]interface{}{
		"2": nil,
		"4": nil,
		"3": nil,
	}
	return &myNewProfileMgr{
		installer:   installer,
		name:        name,
		versionInfo: profiles.NewVersionInfo("test", supported, "3"),
	}
}

func (p *myNewProfileMgr) Name() string {
	return p.name
}

func (p *myNewProfileMgr) Installer() string {
	return p.installer
}

func (p *myNewProfileMgr) Info() string {
	return `
The myNewProfile is for testing purposes only
`
}

func (p *myNewProfileMgr) VersionInfo() *profiles.VersionInfo {
	return p.versionInfo
}

func (p *myNewProfileMgr) String() string {
	if p.profile == nil {
		return fmt.Sprintf("Profile: %s: not installed\n", p.name)
	}
	return fmt.Sprintf("Profile: %s: installed\n%s\n", p.name, p.profile.Targets())
}

func (p *myNewProfileMgr) AddFlags(*flag.FlagSet, profiles.Action) {
}

func (p *myNewProfileMgr) Install(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	p.profile = pdb.InstallProfile(p.name, "", "root")
	return pdb.AddProfileTarget(p.name, "", target)
}

func (p *myNewProfileMgr) Uninstall(jirix *jiri.X, pdb *profiles.DB, root jiri.RelPath, target profiles.Target) error {
	if pdb.RemoveProfileTarget(p.name, "", target) {
		p.profile = nil
	}
	return nil
}

func tmpFile() string {
	dirname, err := ioutil.TempDir("", "pdb")
	if err != nil {
		panic(err)
	}
	return filepath.Join(dirname, "manifest")
}

func ExampleManager() {
	pdb := profiles.NewDB()
	myInstaller := "myProject"
	myProfile := "myNewProfile"
	profileName := profiles.QualifiedProfileName(myInstaller, myProfile)
	var target profiles.Target

	init := func() {
		mgr := newProfileMgr(myInstaller, myProfile)
		profilesmanager.Register(mgr)
		flags := flag.NewFlagSet("example", flag.ContinueOnError)
		profiles.RegisterTargetAndEnvFlags(flags, &target)
		flags.Parse([]string{"--target=arm-linux@1", "--env=A=B,C=D", "--env=E=F"})
	}
	init()

	profileRoot := jiri.NewRelPath("profiles")
	mgr := profilesmanager.LookupManager(profileName)
	if mgr == nil {
		panic("manager not found for: " + profileName)
	}

	jirix := &jiri.X{Context: tool.NewDefaultContext()}
	// Install myNewProfile for target.
	if err := mgr.Install(jirix, pdb, profileRoot, target); err != nil {
		panic("failed to find manager for: " + profileName)
	}

	fmt.Println(mgr.String())

	filename := tmpFile()
	defer os.RemoveAll(filepath.Dir(filename))

	if err := pdb.Write(jirix, "test", filename); err != nil {
		panic(err)
	}

	// Start with a new profile data base.
	pdb = profiles.NewDB()
	// Read the profile database.
	pdb.Read(jirix, filename)

	mgr = profilesmanager.LookupManager(profileName)
	if mgr == nil {
		panic("manager not found for: " + profileName)
	}
	fmt.Println(mgr.String())
	mgr.Uninstall(jirix, pdb, profileRoot, target)
	fmt.Println(mgr.String())
	fmt.Println(mgr.VersionInfo().Supported())
	fmt.Println(mgr.VersionInfo().Default())

	// Output:
	// Profile: myNewProfile: installed
	// [arm-linux@1]
	//
	// Profile: myNewProfile: installed
	// [arm-linux@1]
	//
	// Profile: myNewProfile: not installed
	//
	// [4 3 2]
	// 3
}
