// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jiritest

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"

	"v.io/jiri/collect"
	"v.io/jiri/jiri"
	"v.io/jiri/project"
	"v.io/jiri/tool"
)

// FakeJiriRoot sets up a fake JIRI_ROOT under a tmp directory.
//
// TODO(toddw): Simplify usage, and consolidate with jiritest.NewX.
type FakeJiriRoot struct {
	X        *jiri.X
	Dir      string
	Projects map[string]string
	remote   string
	oldRoot  string
}

const (
	defaultDataDir  = "data"
	defaultManifest = "default"
	manifestProject = ".manifest"
	manifestVersion = "v2"
	toolsProject    = "tools"
)

// NewFakeJiriRoot is the FakeJiriRoot factory.
func NewFakeJiriRoot() (*FakeJiriRoot, error) {
	// Create a fake JIRI_ROOT.
	ctx := tool.NewDefaultContext()
	rootDir, err := ctx.NewSeq().TempDir("", "")
	if err != nil {
		return nil, err
	}
	oldRoot := os.Getenv(jiri.RootEnv)
	if err := os.Setenv(jiri.RootEnv, rootDir); err != nil {
		return nil, err
	}
	jirix := &jiri.X{Context: ctx, Root: rootDir}
	root := &FakeJiriRoot{
		X:        jirix,
		Dir:      rootDir,
		Projects: map[string]string{},
		oldRoot:  oldRoot,
	}

	// Create fake remote manifest and tools projects.
	remoteDir, err := jirix.Run().TempDir("", "")
	if err != nil {
		return nil, err
	}
	doCleanup := true
	defer func() {
		if doCleanup {
			root.Cleanup()
		}
	}()
	root.remote = remoteDir
	if err := root.CreateRemoteProject(manifestProject); err != nil {
		return nil, err
	}
	if err := root.CreateRemoteProject(toolsProject); err != nil {
		return nil, err
	}

	// Create a fake manifest.
	manifestDir := filepath.Join(remoteDir, manifestProject, manifestVersion)
	if err := jirix.Run().MkdirAll(manifestDir, os.FileMode(0700)); err != nil {
		return nil, err
	}
	if err := root.WriteRemoteManifest(&project.Manifest{}); err != nil {
		return nil, err
	}
	if err := jirix.Git().Clone(root.Projects[manifestProject], filepath.Join(root.Dir, manifestProject)); err != nil {
		return nil, err
	}

	// Add the "tools" project and a fake "jiri" tool to the
	// manifests. This is necessary to make sure that the commonly
	// invoked DataDirPath() function, which uses the "jiri" tool
	// configuration for its default, works.
	if err := root.AddProject(project.Project{
		Name:   toolsProject,
		Path:   toolsProject,
		Remote: root.Projects[toolsProject],
	}); err != nil {
		return nil, err
	}
	if err := root.AddTool(project.Tool{
		Name:    "jiri",
		Data:    defaultDataDir,
		Project: toolsProject,
	}); err != nil {
		return nil, err
	}

	// Add "gerrit" and "git" hosts to the manifest, as required by the "jiri"
	// tool.
	if err := root.AddHost(project.Host{
		Name:     "gerrit",
		Location: "git://example.com/gerrit",
	}); err != nil {
		return nil, err
	}
	if err := root.AddHost(project.Host{
		Name:     "git",
		Location: "git://example.com/git",
	}); err != nil {
		return nil, err
	}

	// Update the contents of the fake JIRI_ROOT instance based on
	// the information recorded in the remote manifest.
	if err := root.UpdateUniverse(false); err != nil {
		return nil, err
	}

	doCleanup = false
	return root, nil
}

// Cleanup cleans up the given Vanadium root fake.
func (root FakeJiriRoot) Cleanup() error {
	var errs []error
	collect.Errors(func() error {
		if root.Dir == "" {
			return nil
		}
		return root.X.Run().RemoveAll(root.Dir)
	}, &errs)
	collect.Errors(func() error {
		return root.X.Run().RemoveAll(root.remote)
	}, &errs)
	collect.Errors(func() error {
		if root.oldRoot == "" {
			return nil
		}
		return os.Setenv(jiri.RootEnv, root.oldRoot)
	}, &errs)
	if len(errs) != 0 {
		return fmt.Errorf("Cleanup() failed: %v", errs)
	}
	return nil
}

// AddHost adds the given host to a remote manifest.
func (root FakeJiriRoot) AddHost(host project.Host) error {
	manifest, err := root.ReadRemoteManifest()
	if err != nil {
		return err
	}
	manifest.Hosts = append(manifest.Hosts, host)
	if err := root.WriteRemoteManifest(manifest); err != nil {
		return err
	}
	return nil
}

// AddProject adds the given project to a remote manifest.
func (root FakeJiriRoot) AddProject(project project.Project) error {
	manifest, err := root.ReadRemoteManifest()
	if err != nil {
		return err
	}
	manifest.Projects = append(manifest.Projects, project)
	if err := root.WriteRemoteManifest(manifest); err != nil {
		return err
	}
	return nil
}

// AddTool adds the given tool to a remote manifest.
func (root FakeJiriRoot) AddTool(tool project.Tool) error {
	manifest, err := root.ReadRemoteManifest()
	if err != nil {
		return err
	}
	manifest.Tools = append(manifest.Tools, tool)
	if err := root.WriteRemoteManifest(manifest); err != nil {
		return err
	}
	return nil
}

// DisableRemoteManifestPush disables pushes to the remote manifest
// repository.
func (root FakeJiriRoot) DisableRemoteManifestPush() error {
	dir := tool.RootDirOpt(filepath.Join(root.remote, manifestProject))
	if err := root.X.Git(dir).CheckoutBranch("master"); err != nil {
		return err
	}
	return nil
}

// EnableRemoteManifestPush enables pushes to the remote manifest
// repository.
func (root FakeJiriRoot) EnableRemoteManifestPush() error {
	dir := tool.RootDirOpt(filepath.Join(root.remote, manifestProject))
	if !root.X.Git(dir).BranchExists("non-master") {
		if err := root.X.Git(dir).CreateBranch("non-master"); err != nil {
			return err
		}
	}
	if err := root.X.Git(dir).CheckoutBranch("non-master"); err != nil {
		return err
	}
	return nil
}

// CreateRemoteProject creates a new remote project.
func (root FakeJiriRoot) CreateRemoteProject(name string) error {
	projectDir := filepath.Join(root.remote, name)
	if err := root.X.Run().MkdirAll(projectDir, os.FileMode(0700)); err != nil {
		return err
	}
	if err := root.X.Git().Init(projectDir); err != nil {
		return err
	}
	if err := root.X.Git(tool.RootDirOpt(projectDir)).CommitWithMessage("initial commit"); err != nil {
		return err
	}
	root.Projects[name] = projectDir
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
func (root FakeJiriRoot) ReadLocalManifest() (*project.Manifest, error) {
	path := filepath.Join(root.Dir, manifestProject, manifestVersion, getManifest(root.X))
	return root.readManifest(path)
}

// ReadRemoteManifest read a manifest from the remote manifest project.
func (root FakeJiriRoot) ReadRemoteManifest() (*project.Manifest, error) {
	path := filepath.Join(root.remote, manifestProject, manifestVersion, getManifest(root.X))
	return root.readManifest(path)
}

func (root FakeJiriRoot) readManifest(path string) (*project.Manifest, error) {
	bytes, err := root.X.Run().ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest project.Manifest
	if err := xml.Unmarshal(bytes, &manifest); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(bytes), err)
	}
	return &manifest, nil
}

// UpdateUniverse synchronizes the content of the Vanadium root based
// on the content of the remote manifest.
func (root FakeJiriRoot) UpdateUniverse(gc bool) error {
	oldRoot := os.Getenv("JIRI_ROOT")
	if err := os.Setenv("JIRI_ROOT", root.Dir); err != nil {
		return fmt.Errorf("Setenv() failed: %v", err)
	}
	defer os.Setenv("JIRI_ROOT", oldRoot)
	if err := project.UpdateUniverse(root.X, gc); err != nil {
		return err
	}
	return nil
}

// WriteLocalManifest writes the given manifest to the local
// manifest project.
func (root FakeJiriRoot) WriteLocalManifest(manifest *project.Manifest) error {
	dir := filepath.Join(root.Dir, manifestProject)
	path := filepath.Join(dir, manifestVersion, getManifest(root.X))
	return root.writeManifest(manifest, dir, path)
}

// WriteRemoteManifest writes the given manifest to the remote
// manifest project.
func (root FakeJiriRoot) WriteRemoteManifest(manifest *project.Manifest) error {
	dir := filepath.Join(root.remote, manifestProject)
	path := filepath.Join(dir, manifestVersion, getManifest(root.X))
	return root.writeManifest(manifest, dir, path)
}

func (root FakeJiriRoot) writeManifest(manifest *project.Manifest, dir, path string) error {
	bytes, err := xml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("Marshal(%v) failed: %v", manifest, err)
	}
	if err := root.X.Run().WriteFile(path, bytes, os.FileMode(0600)); err != nil {
		return err
	}
	if err := root.X.Git(tool.RootDirOpt(dir)).Add(path); err != nil {
		return err
	}
	if err := root.X.Git(tool.RootDirOpt(dir)).Commit(); err != nil {
		return err
	}
	return nil
}
