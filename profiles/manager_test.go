// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"v.io/jiri/profiles"
	"v.io/jiri/tool"
)

type myNewProfile struct {
	name, root, status string
	versionInfo        *profiles.VersionInfo
}

func newProfile(name string) *myNewProfile {
	supported := map[string]interface{}{
		"2": nil,
		"4": nil,
		"3": nil,
	}
	return &myNewProfile{name: name, versionInfo: profiles.NewVersionInfo("test", supported, "3")}
}

func (p *myNewProfile) Name() string {
	return p.name
}

func (p *myNewProfile) SetRoot(root string) {
	p.root = root
}

func (p *myNewProfile) Root() string {
	return p.root
}

func (p *myNewProfile) Info() string {
	return `
The myNewProfile is for testing purposes only
`
}

func (p *myNewProfile) VersionInfo() *profiles.VersionInfo {
	return p.versionInfo
}

func (p *myNewProfile) String() string {
	profile := profiles.LookupProfile(p.name)
	if profile == nil {
		return fmt.Sprintf("Profile: %s: %s\n", p.name, p.status)
	}
	return fmt.Sprintf("Profile: %s: %s\n%s\n", p.name, p.status, profile.Targets())
}

func (p *myNewProfile) AddFlags(*flag.FlagSet, profiles.Action) {
}

func (p *myNewProfile) Install(ctx *tool.Context, target profiles.Target) error {
	p.status = "installed"
	profiles.AddProfileTarget(p.name, target)
	return nil
}

func (p *myNewProfile) Uninstall(ctx *tool.Context, target profiles.Target) error {
	profiles.RemoveProfileTarget(p.name, target)
	if profiles.LookupProfile(p.name) == nil {
		p.status = "uninstalled"
	}
	return nil
}

func ExampleManager() {
	myProfile := "myNewProfile"
	var target profiles.Target

	init := func() {
		profiles.Register(myProfile, newProfile(myProfile))
		flags := flag.NewFlagSet("example", flag.ContinueOnError)
		profiles.RegisterTargetAndEnvFlags(flags, &target)
		flags.Parse([]string{"--target=myTarget=arm-linux", "--env=A=B,C=D", "--env=E=F"})
	}
	init()

	mgr := profiles.LookupManager(myProfile)
	if mgr == nil {
		panic("manager not found for: " + myProfile)
	}

	ctx := tool.NewDefaultContext()
	// Install myNewProfile for target.
	if err := mgr.Install(ctx, target); err != nil {
		panic("failed to find manager for: " + myProfile)
	}

	fmt.Println(mgr.String())

	filename := tmpFile()
	defer os.RemoveAll(filepath.Dir(filename))

	if err := profiles.Write(ctx, filename); err != nil {
		panic(err)
	}

	// Clear the profile manifest information in order to mimic starting
	// a new process and reading the manifest file.
	profiles.Clear()

	// Read the profile manifest.
	profiles.Read(ctx, filename)

	mgr = profiles.LookupManager(myProfile)
	if mgr == nil {
		panic("manager not found for: " + myProfile)
	}

	fmt.Println(mgr.String())
	mgr.Uninstall(ctx, target)
	fmt.Println(mgr.String())
	fmt.Println(mgr.VersionInfo().Supported())
	fmt.Println(mgr.VersionInfo().Default())

	// Output:
	// Profile: myNewProfile: installed
	// [myTarget=arm-linux@]
	//
	// Profile: myNewProfile: installed
	// [myTarget=arm-linux@]
	//
	// Profile: myNewProfile: uninstalled
	//
	// [4 3 2]
	// 3
}
