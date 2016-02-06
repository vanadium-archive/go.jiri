// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	// V4 adds support for relative path names in profiles and environment variable.
	V4 Version = 4
	// V5 adds support for multiple profile installers.
	V5 Version = 5
)

type profilesSchema struct {
	XMLName xml.Name `xml:"profiles"`
	// The Version of the schema used for this file.
	Version Version `xml:"version,attr,omitempty"`
	// The name of the installer that created the profiles in this file.
	Installer string           `xml:"installer,attr,omitempty"`
	Profiles  []*profileSchema `xml:"profile"`
}

type profileSchema struct {
	XMLName xml.Name        `xml:"profile"`
	Name    string          `xml:"name,attr"`
	Root    string          `xml:"root,attr"`
	Targets []*targetSchema `xml:"target"`
}

type targetSchema struct {
	XMLName         xml.Name    `xml:"target"`
	Arch            string      `xml:"arch,attr"`
	OS              string      `xml:"os,attr"`
	InstallationDir string      `xml:"installation-directory,attr"`
	Version         string      `xml:"version,attr"`
	UpdateTime      time.Time   `xml:"date,attr"`
	Env             Environment `xml:"envvars"`
	CommandLineEnv  Environment `xml:"command-line"`
}

type DB struct {
	mu      sync.Mutex
	version Version
	path    string
	db      map[string]*Profile
}

// NewDB returns a new instance of a profile database.
func NewDB() *DB {
	return &DB{db: make(map[string]*Profile), version: V5}
}

// Path returns the directory or filename that this database was read from.
func (pdb *DB) Path() string {
	return pdb.path
}

// InstallProfile will create a new profile to the profiles database,
// it has no effect if the profile already exists. It returns the profile
// that was either newly created or already installed.
func (pdb *DB) InstallProfile(installer, name, root string) *Profile {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	qname := QualifiedProfileName(installer, name)
	if p := pdb.db[qname]; p == nil {
		pdb.db[qname] = &Profile{name: qname, root: root}
	}
	return pdb.db[qname]
}

// AddProfileTarget adds the specified target to the named profile.
// The UpdateTime of the newly installed target will be set to time.Now()
func (pdb *DB) AddProfileTarget(installer, name string, target Target) error {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	target.UpdateTime = time.Now()
	qname := QualifiedProfileName(installer, name)
	if pi, present := pdb.db[qname]; present {
		for _, t := range pi.Targets() {
			if target.Match(t) {
				return fmt.Errorf("%s is already used by profile %s %s", target, qname, pi.Targets())
			}
		}
		pi.targets = InsertTarget(pi.targets, &target)
		return nil
	}
	return fmt.Errorf("profile %v is not installed", qname)
}

// UpdateProfileTarget updates the specified target from the named profile.
// The UpdateTime of the updated target will be set to time.Now()
func (pdb *DB) UpdateProfileTarget(installer, name string, target Target) error {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	target.UpdateTime = time.Now()
	qname := QualifiedProfileName(installer, name)
	pi, present := pdb.db[qname]
	if !present {
		return fmt.Errorf("profile %v is not installed", qname)
	}
	for _, t := range pi.targets {
		if target.Match(t) {
			*t = target
			t.UpdateTime = time.Now()
			return nil
		}
	}
	return fmt.Errorf("profile %v does not have target: %v", qname, target)
}

// RemoveProfileTarget removes the specified target from the named profile.
// If this is the last target for the profile then the profile will be deleted
// from the database. It returns true if the profile was so deleted or did
// not originally exist.
func (pdb *DB) RemoveProfileTarget(installer, name string, target Target) bool {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	qname := QualifiedProfileName(installer, name)
	pi, present := pdb.db[qname]
	if !present {
		return true
	}
	pi.targets = RemoveTarget(pi.targets, &target)
	if len(pi.targets) == 0 {
		delete(pdb.db, qname)
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

// LookupProfile returns the profile for the supplied installer and profile
// name or nil if one is not found.
func (pdb *DB) LookupProfile(installer, name string) *Profile {
	qname := QualifiedProfileName(installer, name)
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	return pdb.db[qname]
}

// LookupProfileTarget returns the target information stored for the
// supplied installer, profile name and target.
func (pdb *DB) LookupProfileTarget(installer, name string, target Target) *Target {
	qname := QualifiedProfileName(installer, name)
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	mgr := pdb.db[qname]
	if mgr == nil {
		return nil
	}
	return FindTarget(mgr.targets, &target)
}

// EnvFromProfile obtains the environment variable settings from the specified
// profile and target. It returns nil if the target and/or profile could not
// be found.
func (pdb *DB) EnvFromProfile(installer, name string, target Target) []string {
	t := pdb.LookupProfileTarget(installer, name, target)
	if t == nil {
		return nil
	}
	return t.Env.Vars
}

func getDBFilenames(jirix *jiri.X, path string) (bool, []string, error) {
	s := jirix.NewSeq()
	isdir, err := s.IsDir(path)
	if err != nil {
		return false, nil, err
	}
	if !isdir {
		return false, []string{path}, nil
	}
	fis, err := s.ReadDir(path)
	if err != nil {
		return true, nil, err
	}
	paths := []string{}
	for _, fi := range fis {
		if strings.HasSuffix(fi.Name(), ".prev") {
			continue
		}
		paths = append(paths, filepath.Join(path, fi.Name()))
	}
	return true, paths, nil
}

// Read reads the specified database directory or file to obtain the current
// set of installed profiles into the receiver database. It is not
// an error if the database does not exist, instead, an empty database
// is returned.
func (pdb *DB) Read(jirix *jiri.X, path string) error {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()
	pdb.db = make(map[string]*Profile)
	isDir, filenames, err := getDBFilenames(jirix, path)
	if err != nil {
		return err
	}
	pdb.path = path
	s := jirix.NewSeq()
	for i, filename := range filenames {
		data, err := s.ReadFile(filename)
		if err != nil {
			// It's not an error if the database doesn't exist yet, it'll
			// just have no data in it and then be written out. This is the
			// case when starting with a new/empty repo. The original profiles
			// implementation behaved this way and I've tried to maintain it
			// without having to special case all of the call sites.
			if runutil.IsNotExist(err) {
				continue
			}
			return err
		}
		var schema profilesSchema
		if err := xml.Unmarshal(data, &schema); err != nil {
			return fmt.Errorf("Unmarshal(%v) failed: %v", string(data), err)
		}
		if isDir {
			if schema.Version < V5 {
				return fmt.Errorf("Profile database files must be at version %d (not %d) when more than one is found in a directory", V5, schema.Version)
			}
			if i >= 1 && pdb.version != schema.Version {
				return fmt.Errorf("Profile database files must have the same version (%d != %d) when more than one is found in a directory", pdb.version, schema.Version)
			}
		}
		pdb.version = schema.Version
		for _, p := range schema.Profiles {
			qname := QualifiedProfileName(schema.Installer, p.Name)
			pdb.db[qname] = &Profile{
				// Use the unqualified name in each profile since the
				// reader will read the installer from the xml installer
				// tag.
				name:      p.Name,
				installer: schema.Installer,
				root:      p.Root,
			}
			for _, target := range p.Targets {
				pdb.db[qname].targets = append(pdb.db[qname].targets, &Target{
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
	}
	return nil
}

// Write writes the current set of installed profiles to the specified
// database location. No data will be written and an error returned if the
// path is a directory and installer is an empty string.
func (pdb *DB) Write(jirix *jiri.X, installer, path string) error {
	pdb.mu.Lock()
	defer pdb.mu.Unlock()

	if len(path) == 0 {
		return fmt.Errorf("please specify a profiles database path")
	}

	s := jirix.NewSeq()
	isdir, err := s.IsDir(path)
	if err != nil && !runutil.IsNotExist(err) {
		return err
	}
	filename := path
	if isdir {
		if installer == "" {
			return fmt.Errorf("no installer specified for directory path %v", path)
		}
		filename = filepath.Join(filename, installer)
	}

	var schema profilesSchema
	schema.Version = V5
	schema.Installer = installer
	for _, name := range pdb.profilesUnlocked() {
		profileInstaller, profileName := SplitProfileName(name)
		if profileInstaller != installer {
			continue
		}
		profile := pdb.db[name]
		current := &profileSchema{Name: profileName, Root: profile.root}
		schema.Profiles = append(schema.Profiles, current)

		for _, target := range profile.targets {
			sort.Strings(target.Env.Vars)
			if len(target.version) == 0 {
				return fmt.Errorf("missing version for profile %s target: %s", name, target)
			}
			current.Targets = append(current.Targets,
				&targetSchema{
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
