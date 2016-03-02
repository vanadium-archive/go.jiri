// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jiritest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"v.io/jiri"
	"v.io/jiri/gitutil"
	"v.io/jiri/project"
	"v.io/jiri/util"
)

// FakeJiriRoot sets up a fake JIRI_ROOT under a tmp directory.
type FakeJiriRoot struct {
	X        *jiri.X
	Projects map[string]string
	remote   string
}

const (
	defaultDataDir  = "data"
	defaultManifest = "default"
	manifestProject = ".manifest"
	manifestVersion = "v2"
	toolsProject    = "tools"
)

// NewFakeJiriRoot returns a new FakeJiriRoot and a cleanup closure.  The
// closure must be run to cleanup temporary directories and restore the original
// environment; typically it is run as a defer function.
func NewFakeJiriRoot(t *testing.T) (*FakeJiriRoot, func()) {
	jirix, cleanup := NewX(t)
	fake := &FakeJiriRoot{
		X:        jirix,
		Projects: map[string]string{},
	}

	s := jirix.NewSeq()
	// Create fake remote manifest and tools projects.
	remoteDir, err := s.TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	fake.remote = remoteDir
	if err := fake.CreateRemoteProject(manifestProject); err != nil {
		t.Fatal(err)
	}
	if err := fake.CreateRemoteProject(toolsProject); err != nil {
		t.Fatal(err)
	}

	// Create a fake manifest.
	manifestDir := filepath.Join(remoteDir, manifestProject, manifestVersion)
	if err := s.MkdirAll(manifestDir, os.FileMode(0700)).Done(); err != nil {
		t.Fatal(err)
	}
	if err := fake.WriteRemoteManifest(&project.Manifest{}); err != nil {
		t.Fatal(err)
	}
	if err := gitutil.New(jirix.NewSeq()).CloneRecursive(fake.Projects[manifestProject], filepath.Join(jirix.Root, manifestProject)); err != nil {
		t.Fatal(err)
	}

	// Add the "tools" project and a fake "jiri" tool to the
	// manifests. This is necessary to make sure that the commonly
	// invoked DataDirPath() function, which uses the "jiri" tool
	// configuration for its default, works.
	if err := fake.AddProject(project.Project{
		Name:   toolsProject,
		Path:   toolsProject,
		Remote: fake.Projects[toolsProject],
	}); err != nil {
		t.Fatal(err)
	}
	if err := fake.AddTool(project.Tool{
		Name:    "jiri",
		Data:    defaultDataDir,
		Project: toolsProject,
	}); err != nil {
		t.Fatal(err)
	}

	// Update the contents of the fake JIRI_ROOT instance based on
	// the information recorded in the remote manifest.
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}

	// Create an empty config file, needed by many utilities.
	if err := util.SaveConfig(jirix, util.NewConfig()); err != nil {
		t.Fatal(err)
	}

	// Create the profiles database directory.
	profilesDB := filepath.Join(jirix.Root, ".jiri_root", "profile_db")
	if err := s.MkdirAll(profilesDB, os.FileMode(0700)).Done(); err != nil {
		t.Fatal(err)
	}

	return fake, func() {
		cleanup()
		if err := fake.X.NewSeq().RemoveAll(fake.remote).Done(); err != nil {
			t.Fatalf("RemoveAll(%q) failed: %v", fake.remote, err)
		}
	}
}

// AddProject adds the given project to a remote manifest.
func (fake FakeJiriRoot) AddProject(project project.Project) error {
	manifest, err := fake.ReadRemoteManifest()
	if err != nil {
		return err
	}
	manifest.Projects = append(manifest.Projects, project)
	if err := fake.WriteRemoteManifest(manifest); err != nil {
		return err
	}
	return nil
}

// AddTool adds the given tool to a remote manifest.
func (fake FakeJiriRoot) AddTool(tool project.Tool) error {
	manifest, err := fake.ReadRemoteManifest()
	if err != nil {
		return err
	}
	manifest.Tools = append(manifest.Tools, tool)
	if err := fake.WriteRemoteManifest(manifest); err != nil {
		return err
	}
	return nil
}

// DisableRemoteManifestPush disables pushes to the remote manifest
// repository.
func (fake FakeJiriRoot) DisableRemoteManifestPush() error {
	dir := gitutil.RootDirOpt(filepath.Join(fake.remote, manifestProject))
	if err := gitutil.New(fake.X.NewSeq(), dir).CheckoutBranch("master"); err != nil {
		return err
	}
	return nil
}

// EnableRemoteManifestPush enables pushes to the remote manifest
// repository.
func (fake FakeJiriRoot) EnableRemoteManifestPush() error {
	dir := gitutil.RootDirOpt(filepath.Join(fake.remote, manifestProject))
	if !gitutil.New(fake.X.NewSeq(), dir).BranchExists("non-master") {
		if err := gitutil.New(fake.X.NewSeq(), dir).CreateBranch("non-master"); err != nil {
			return err
		}
	}
	if err := gitutil.New(fake.X.NewSeq(), dir).CheckoutBranch("non-master"); err != nil {
		return err
	}
	return nil
}

// CreateRemoteProject creates a new remote project.
func (fake FakeJiriRoot) CreateRemoteProject(name string) error {
	projectDir := filepath.Join(fake.remote, name)
	if err := fake.X.NewSeq().MkdirAll(projectDir, os.FileMode(0700)).Done(); err != nil {
		return err
	}
	if err := gitutil.New(fake.X.NewSeq()).Init(projectDir); err != nil {
		return err
	}
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(projectDir)).CommitWithMessage("initial commit"); err != nil {
		return err
	}
	fake.Projects[name] = projectDir
	return nil
}

func getManifest(jirix *jiri.X) string {
	manifest := jirix.Manifest()
	if manifest != "" {
		return manifest
	}
	return defaultManifest
}

// ReadLocalManifest read a manifest from the local manifest project.
func (fake FakeJiriRoot) ReadLocalManifest() (*project.Manifest, error) {
	path := filepath.Join(fake.X.Root, manifestProject, manifestVersion, getManifest(fake.X))
	return project.ManifestFromFile(fake.X, path)
}

// ReadRemoteManifest read a manifest from the remote manifest project.
func (fake FakeJiriRoot) ReadRemoteManifest() (*project.Manifest, error) {
	path := filepath.Join(fake.remote, manifestProject, manifestVersion, getManifest(fake.X))
	return project.ManifestFromFile(fake.X, path)
}

// UpdateUniverse synchronizes the content of the Vanadium fake based
// on the content of the remote manifest.
func (fake FakeJiriRoot) UpdateUniverse(gc bool) error {
	oldRoot := os.Getenv(jiri.RootEnv)
	if err := os.Setenv(jiri.RootEnv, fake.X.Root); err != nil {
		return fmt.Errorf("Setenv() failed: %v", err)
	}
	defer os.Setenv(jiri.RootEnv, oldRoot)
	if err := project.UpdateUniverse(fake.X, gc); err != nil {
		return err
	}
	return nil
}

// WriteLocalManifest writes the given manifest to the local
// manifest project.
func (fake FakeJiriRoot) WriteLocalManifest(manifest *project.Manifest) error {
	dir := filepath.Join(fake.X.Root, manifestProject)
	path := filepath.Join(dir, manifestVersion, getManifest(fake.X))
	return fake.writeManifest(manifest, dir, path)
}

// WriteRemoteManifest writes the given manifest to the remote
// manifest project.
func (fake FakeJiriRoot) WriteRemoteManifest(manifest *project.Manifest) error {
	dir := filepath.Join(fake.remote, manifestProject)
	path := filepath.Join(dir, manifestVersion, getManifest(fake.X))
	return fake.writeManifest(manifest, dir, path)
}

func (fake FakeJiriRoot) writeManifest(manifest *project.Manifest, dir, path string) error {
	if err := manifest.ToFile(fake.X, path); err != nil {
		return err
	}
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(dir)).Add(path); err != nil {
		return err
	}
	if err := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(dir)).Commit(); err != nil {
		return err
	}
	return nil
}
