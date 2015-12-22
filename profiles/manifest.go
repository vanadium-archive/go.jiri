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

	"v.io/jiri/jiri"
	"v.io/jiri/runutil"
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

type DB struct {
	mu       sync.Mutex
	version  Version
	filename string
	db       map[string]*Profile
}

// NewDB returns a new instance of a profile database.
func NewDB() *DB {
	return &DB{db: make(map[string]*Profile), version: V4}
}

// Filename returns the filename that this database was read from.
func (pdb *DB) Filename() string {
	return pdb.filename
}

// InstallProfile will create a new profile to the profiles database,
// it has no effect if the profile already exists. It returns the profile
// that was either newly created or already installed.
func (pdb *DB) InstallProfile(name, root string) *Profile {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	if p := pdb.db[name]; p == nil {
		pdb.db[name] = &Profile{name: name, root: root}
	}
	return pdb.db[name]
}

// AddProfileTarget adds the specified target to the named profile.
// The UpdateTime of the newly installed target will be set to time.Now()
func (pdb *DB) AddProfileTarget(name string, target Target) error {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	target.UpdateTime = time.Now()
	if pi, present := pdb.db[name]; present {
		for _, t := range pi.Targets() {
			if target.Match(t) {
				return fmt.Errorf("%s is already used by profile %s %s", target, name, pi.Targets())
			}
		}
		pi.targets = InsertTarget(pi.targets, &target)
		return nil
	}
	return fmt.Errorf("profile %v is not installed", name)
}

// UpdateProfileTarget updates the specified target from the named profile.
// The UpdateTime of the updated target will be set to time.Now()
func (pdb *DB) UpdateProfileTarget(name string, target Target) error {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	target.UpdateTime = time.Now()
	pi, present := pdb.db[name]
	if !present {
		return fmt.Errorf("profile %v is not installed", name)
	}
	for _, t := range pi.targets {
		if target.Match(t) {
			*t = target
			t.UpdateTime = time.Now()
			return nil
		}
	}
	return fmt.Errorf("profile %v does not have target: %v", name, target)
}

// RemoveProfileTarget removes the specified target from the named profile.
// If this is the last target for the profile then the profile will be deleted
// from the database. It returns true if the profile was so deleted or did
// not originally exist.
func (pdb *DB) RemoveProfileTarget(name string, target Target) bool {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()

	pi, present := pdb.db[name]
	if !present {
		return true
	}
	pi.targets = RemoveTarget(pi.targets, &target)
	if len(pi.targets) == 0 {
		delete(pdb.db, name)
		return true
	}
	return false
}

// Names returns the names, in lexicographic order, of all of the currently
// available profiles.
func (pdb *DB) Names() []string {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	return pdb.profilesUnlocked()
}

// Profiles returns all currently installed the profiles, in lexicographic order.
func (pdb *DB) Profiles() []*Profile {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	names := pdb.profilesUnlocked()
	r := make([]*Profile, len(names), len(names))
	for i, name := range names {
		r[i] = pdb.db[name]
	}
	return r
}

func (pdb *DB) profilesUnlocked() []string {
	names := make([]string, 0, len(pdb.db))
	for name := range pdb.db {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// LookupProfile returns the profile for the name profile or nil if one is
// not found.
func (pdb *DB) LookupProfile(name string) *Profile {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	return pdb.db[name]
}

// LookupProfileTarget returns the target information stored for the name
// profile.
func (pdb *DB) LookupProfileTarget(name string, target Target) *Target {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	mgr := pdb.db[name]
	if mgr == nil {
		return nil
	}
	return FindTarget(mgr.targets, &target)
}

// EnvFromProfile obtains the environment variable settings from the specified
// profile and target. It returns nil if the target and/or profile could not
// be found.
func (pdb *DB) EnvFromProfile(name string, target Target) []string {
	t := pdb.LookupProfileTarget(name, target)
	if t == nil {
		return nil
	}
	return t.Env.Vars
}

// Read reads the specified database file to obtain the current set of
// installed profiles into the receiver database.
func (pdb *DB) Read(jirix *jiri.X, filename string) error {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	pdb.db = make(map[string]*Profile)

	data, err := jirix.NewSeq().ReadFile(filename)
	if err != nil {
		if runutil.IsNotExist(err) {
			return nil
		}
		return err
	}

	var schema profilesSchema
	if err := xml.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("Unmarshal(%v) failed: %v", string(data), err)
	}
	pdb.version = schema.Version
	pdb.filename = filename
	for _, p := range schema.Profiles {
		name := p.Name
		pdb.db[name] = &Profile{
			name: name,
			root: p.Root,
		}
		for _, target := range p.Targets {
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

// Write writes the current set of installed profiles to the specified
// database file.
func (pdb *DB) Write(jirix *jiri.X, filename string) error {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()

	if len(filename) == 0 {
		return fmt.Errorf("please specify a filename")
	}

	var schema profilesSchema
	schema.Version = V4
	for i, name := range pdb.profilesUnlocked() {
		profile := pdb.db[name]
		schema.Profiles = append(schema.Profiles, &profileSchema{
			Name: name,
			Root: profile.root,
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

	s := jirix.NewSeq()
	if err := s.WriteFile(newName, data, defaultFileMode).
		AssertFileExists(filename).
		Rename(filename, oldName).Done(); err != nil && !runutil.IsNotExist(err) {
		return err
	}

	if err := s.Rename(newName, filename).Done(); err != nil {
		return err
	}

	return nil
}

// SchemaVersion returns the version of the xml schema used to implement
// the database.
func (pdb *DB) SchemaVersion() Version {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	return pdb.version
}
