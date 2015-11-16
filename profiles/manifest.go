// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles

import (
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"v.io/jiri/tool"
)

const (
	defaultFileMode = os.FileMode(0644)
)

type Version int

const (
	// Original, old-style profiles without a version #
	Original Version = 0
	// First version of new-style profiles.
	V2 Version = 2
	// V3 added support for recording the options that were used to install profiles.
	V3 Version = 3
	// V4 adds support for relative path names in profiles and environment variables.
	V4 Version = 4
)

// Profile represents a suite of software that is managed by an implementation
// of profiles.Manager.
type Profile struct {
	Name    string
	Root    string
	targets OrderedTargets
}

func (p *Profile) Targets() OrderedTargets {
	r := make(OrderedTargets, len(p.targets), len(p.targets))
	for i, t := range p.targets {
		tmp := *t
		r[i] = &tmp
	}
	return r
}

type profilesSchema struct {
	XMLName  xml.Name         `xml:"profiles"`
	Version  Version          `xml:"version,attr"`
	Profiles []*profileSchema `xml:"profile"`
}

type profileSchema struct {
	XMLName xml.Name        `xml:"profile"`
	Name    string          `xml:"name,attr"`
	Root    string          `xml:"root,attr"`
	Targets []*targetSchema `xml:"target"`
}

type targetSchema struct {
	XMLName xml.Name `xml:"target"`
	// TODO(cnicolaou): remove this after this CL is checked in and no-one
	// is using Tags.
	Tag             string      `xml:"tag,attr"`
	Arch            string      `xml:"arch,attr"`
	OS              string      `xml:"os,attr"`
	InstallationDir string      `xml:"installation-directory,attr"`
	Version         string      `xml:"version,attr"`
	UpdateTime      time.Time   `xml:"date,attr"`
	Env             Environment `xml:"envvars"`
	CommandLineEnv  Environment `xml:"command-line"`
}

type profileDB struct {
	sync.Mutex
	version Version
	db      map[string]*Profile
}

func newDB() *profileDB {
	return &profileDB{db: make(map[string]*Profile), version: 0}
}

var (
	db = newDB()
)

// Profiles returns the names, in lexicographic order, of all of the currently
// available profiles as read or stored in the manifest. A profile name may
// be used to lookup a profile manager or the current state of a profile.
func Profiles() []string {
	return db.profiles()
}

func SchemaVersion() Version {
	return db.schemaVersion()
}

// LookupProfile returns the profile for the name profile or nil if one is
// not found.
func LookupProfile(name string) *Profile {
	return db.profile(name)
}

// LookupProfileTarget returns the target information stored for the name
// profile.
func LookupProfileTarget(name string, target Target) *Target {
	mgr := db.profile(name)
	if mgr == nil {
		return nil
	}
	return FindTarget(mgr.targets, &target)
}

// InstallProfile will create a new profile and store in the profiles manifest,
// it has no effect if the profile already exists.
func InstallProfile(name, root string) {
	db.installProfile(name, root)
}

// AddProfileTarget adds the specified target to the named profile.
// The UpdateTime of the newly installed target will be set to time.Now()
func AddProfileTarget(name string, target Target) error {
	return db.addProfileTarget(name, &target)
}

// RemoveProfileTarget removes the specified target from the named profile.
// If this is the last target for the profile then the profile will be deleted
// from the manifest. It returns true if the profile was so deleted or did
// not originally exist.
func RemoveProfileTarget(name string, target Target) bool {
	return db.removeProfileTarget(name, &target)
}

// UpdateProfileTarget updates the specified target from the named profile.
// The UpdateTime of the updated target will be set to time.Now()
func UpdateProfileTarget(name string, target Target) error {
	return db.updateProfileTarget(name, &target)
}

// Read reads the specified manifest file to obtain the current set of
// installed profiles.
func Read(ctx *tool.Context, filename string) error {
	return db.read(ctx, filename)
}

// Write writes the current set of installed profiles to the specified manifest
// file.
func Write(ctx *tool.Context, filename string) error {
	return db.write(ctx, filename)
}

func (pdb *profileDB) installProfile(name, root string) {
	pdb.Lock()
	defer pdb.Unlock()
	if p := pdb.db[name]; p == nil {
		pdb.db[name] = &Profile{Name: name, Root: root}
	}
}

func (pdb *profileDB) addProfileTarget(name string, target *Target) error {
	pdb.Lock()
	defer pdb.Unlock()
	target.UpdateTime = time.Now()
	if pi, present := pdb.db[name]; present {
		for _, t := range pi.Targets() {
			if target.Match(t) {
				return fmt.Errorf("%s is already used by profile %s %s", target, name, pi.Targets())
			}
		}
		pi.targets = InsertTarget(pi.targets, target)
		return nil
	}
	pdb.db[name] = &Profile{Name: name}
	pdb.db[name].targets = InsertTarget(nil, target)
	return nil
}

func (pdb *profileDB) updateProfileTarget(name string, target *Target) error {
	pdb.Lock()
	defer pdb.Unlock()
	target.UpdateTime = time.Now()
	pi, present := pdb.db[name]
	if !present {
		return fmt.Errorf("profile %v is not installed", name)
	}
	for _, t := range pi.targets {
		if target.Match(t) {
			*t = *target
			t.UpdateTime = time.Now()
			return nil
		}
	}
	return fmt.Errorf("profile %v does not have target: %v", name, target)
}

func (pdb *profileDB) removeProfileTarget(name string, target *Target) bool {
	pdb.Lock()
	defer pdb.Unlock()

	pi, present := pdb.db[name]
	if !present {
		return true
	}
	pi.targets = RemoveTarget(pi.targets, target)
	if len(pi.targets) == 0 {
		delete(pdb.db, name)
		return true
	}
	return false
}

func (pdb *profileDB) profiles() []string {
	pdb.Lock()
	defer pdb.Unlock()
	return pdb.profilesUnlocked()

}

func (pdb *profileDB) profilesUnlocked() []string {
	names := make([]string, 0, len(pdb.db))
	for name := range pdb.db {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (pdb *profileDB) profile(name string) *Profile {
	pdb.Lock()
	defer pdb.Unlock()
	return pdb.db[name]
}

func (pdb *profileDB) read(ctx *tool.Context, filename string) error {
	pdb.Lock()
	defer pdb.Unlock()
	pdb.db = make(map[string]*Profile)

	data, err := ctx.Run().ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(ctx.Stderr(), "WARNING: %v doesn't exist\n", filename)
			return nil
		}
		return err
	}

	var schema profilesSchema
	if err := xml.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("Unmarshal(%v) failed: %v", string(data), err)
	}
	pdb.version = schema.Version
	for _, profile := range schema.Profiles {
		name := profile.Name
		pdb.db[name] = &Profile{
			Name: name,
			Root: profile.Root,
		}
		for _, target := range profile.Targets {
			pdb.db[name].targets = append(pdb.db[name].targets, &Target{
				arch:            target.Arch,
				opsys:           target.OS,
				Env:             target.Env,
				commandLineEnv:  target.CommandLineEnv,
				version:         target.Version,
				UpdateTime:      target.UpdateTime,
				InstallationDir: target.InstallationDir,
				isSet:           true,
			})
		}
	}
	return nil
}

func (pdb *profileDB) write(ctx *tool.Context, filename string) error {
	pdb.Lock()
	defer pdb.Unlock()

	var schema profilesSchema
	schema.Version = V3
	for i, name := range pdb.profilesUnlocked() {
		profile := pdb.db[name]
		schema.Profiles = append(schema.Profiles, &profileSchema{
			Name: name,
			Root: profile.Root,
		})

		for _, target := range profile.targets {
			sort.Strings(target.Env.Vars)
			if len(target.version) == 0 {
				return fmt.Errorf("missing version for profile %s target: %s", name, target)
			}
			schema.Profiles[i].Targets = append(schema.Profiles[i].Targets,
				&targetSchema{
					Tag:             "",
					Arch:            target.arch,
					OS:              target.opsys,
					Env:             target.Env,
					CommandLineEnv:  target.commandLineEnv,
					Version:         target.version,
					InstallationDir: target.InstallationDir,
					UpdateTime:      target.UpdateTime,
				})
		}
	}

	data, err := xml.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent() failed: %v", err)
	}

	oldName := filename + ".prev"
	newName := filename + fmt.Sprintf(".%d", time.Now().UnixNano())

	if err := ctx.Run().WriteFile(newName, data, defaultFileMode); err != nil {
		return err
	}

	if ctx.Run().FileExists(filename) {
		if err := ctx.Run().Rename(filename, oldName); err != nil {
			return err
		}
	}

	if err := ctx.Run().Rename(newName, filename); err != nil {
		return err
	}

	return nil
}

func (pdb *profileDB) schemaVersion() Version {
	pdb.Lock()
	defer pdb.Unlock()
	return pdb.version
}
