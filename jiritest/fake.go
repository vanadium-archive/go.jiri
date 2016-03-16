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
	defaultDataDir      = "data"
	manifestFileName    = "public"
	manifestProjectName = "manifest"
	manifestProjectPath = "manifest"
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
	if err := fake.CreateRemoteProject(manifestProjectPath); err != nil {
		t.Fatal(err)
	}
	// Create a fake manifest.
	manifestDir := filepath.Join(remoteDir, manifestProjectPath)
	if err := s.MkdirAll(manifestDir, os.FileMode(0700)).Done(); err != nil {
		t.Fatal(err)
	}
	if err := fake.WriteRemoteManifest(&project.Manifest{}); err != nil {
		t.Fatal(err)
	}
	// Add the "manifest" project to the manifest.
	if err := fake.AddProject(project.Project{
		Name:   manifestProjectName,
		Path:   manifestProjectPath,
		Remote: fake.Projects[manifestProjectName],
	}); err != nil {
		t.Fatal(err)
	}
	// Create a .jiri_manifest file which imports the manifest created above.
	if err := fake.WriteJiriManifest(&project.Manifest{
		Imports: []project.Import{
			project.Import{
				Manifest: manifestFileName,
				Name:     manifestProjectName,
				Remote:   filepath.Join(fake.remote, manifestProjectPath),
			},
		},
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
	dir := gitutil.RootDirOpt(filepath.Join(fake.remote, manifestProjectPath))
	if err := gitutil.New(fake.X.NewSeq(), dir).CheckoutBranch("master"); err != nil {
		return err
	}
	return nil
}

// EnableRemoteManifestPush enables pushes to the remote manifest
// repository.
func (fake FakeJiriRoot) EnableRemoteManifestPush() error {
	dir := gitutil.RootDirOpt(filepath.Join(fake.remote, manifestProjectPath))
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

// ReadRemoteManifest read a manifest from the remote manifest project.
func (fake FakeJiriRoot) ReadRemoteManifest() (*project.Manifest, error) {
	path := filepath.Join(fake.remote, manifestProjectPath, manifestFileName)
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

// ReadJiriManifest reads the .jiri_manifest manifest.
func (fake FakeJiriRoot) ReadJiriManifest() (*project.Manifest, error) {
	return project.ManifestFromFile(fake.X, fake.X.JiriManifestFile())
}

// WriteJiriManifest writes the given manifest to the .jiri_manifest file.
func (fake FakeJiriRoot) WriteJiriManifest(manifest *project.Manifest) error {
	return manifest.ToFile(fake.X, fake.X.JiriManifestFile())
}

// WriteRemoteManifest writes the given manifest to the remote
// manifest project.
func (fake FakeJiriRoot) WriteRemoteManifest(manifest *project.Manifest) error {
	dir := filepath.Join(fake.remote, manifestProjectPath)
	path := filepath.Join(dir, manifestFileName)
	return fake.writeManifest(manifest, dir, path)
}

func (fake FakeJiriRoot) writeManifest(manifest *project.Manifest, dir, path string) error {
	git := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(dir))
	if err := manifest.ToFile(fake.X, path); err != nil {
		return err
	}
	if err := git.Add(path); err != nil {
		return err
	}
	if err := git.Commit(); err != nil {
		return err
	}
	return nil
}
