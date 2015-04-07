// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

// TODO(jsimsa): Switch this test to using lib/tool.FakeV23Root.

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"v.io/x/devtools/internal/tool"
)

func addRemote(t *testing.T, ctx *tool.Context, localProject, name, remoteProject string) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ctx.Run().Chdir(cwd)
	if err := ctx.Run().Chdir(localProject); err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Git().AddRemote(name, remoteProject); err != nil {
		t.Fatalf("%v", err)
	}
}

func checkReadme(t *testing.T, ctx *tool.Context, project, message string) {
	if _, err := os.Stat(project); err != nil {
		t.Fatalf("%v", err)
	}
	readmeFile := filepath.Join(project, "README")
	data, err := ioutil.ReadFile(readmeFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", readmeFile, err)
	}
	if got, want := data, []byte(message); bytes.Compare(got, want) != 0 {
		t.Fatalf("unexpected content %v:\ngot\n%s\nwant\n%s\n", project, got, want)
	}
}

func createLocalManifestCopy(t *testing.T, ctx *tool.Context, dir, manifestDir string) {
	// Load the remote manifest.
	manifestFile := filepath.Join(manifestDir, "v2", "default")
	data, err := ioutil.ReadFile(manifestFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", manifestFile, err)
	}
	manifest := Manifest{}
	if err := xml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n", err, data)
	}

	// Store the manifest locally.
	data, err = xml.Marshal(manifest)
	if err != nil {
		t.Fatalf("%v", err)
	}
	manifestFile, perm := filepath.Join(dir, ".local_manifest"), os.FileMode(0644)
	if err := ioutil.WriteFile(manifestFile, data, perm); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", manifestFile, err, perm)
	}
}

func createLocalManifestStub(t *testing.T, ctx *tool.Context, dir string) {
	// Create a manifest stub.
	manifest := Manifest{}
	manifest.Imports = append(manifest.Imports, Import{Name: "default"})

	// Store the manifest locally.
	data, err := xml.Marshal(manifest)
	if err != nil {
		t.Fatalf("%v", err)
	}
	manifestFile, perm := filepath.Join(dir, ".local_manifest"), os.FileMode(0644)
	if err := ioutil.WriteFile(manifestFile, data, perm); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", manifestFile, err, perm)
	}
}

func createRemoteManifest(t *testing.T, ctx *tool.Context, dir string, remotes []string) {
	manifestDir, perm := filepath.Join(dir, "v2"), os.FileMode(0755)
	if err := ctx.Run().MkdirAll(manifestDir, perm); err != nil {
		t.Fatalf("%v", err)
	}
	manifest := Manifest{}
	for i, remote := range remotes {
		project := Project{
			Name:     remote,
			Path:     localProjectName(i),
			Protocol: "git",
			Remote:   remote,
		}
		manifest.Projects = append(manifest.Projects, project)
	}
	commitManifest(t, ctx, &manifest, dir)
}

func commitManifest(t *testing.T, ctx *tool.Context, manifest *Manifest, manifestDir string) {
	data, err := xml.Marshal(*manifest)
	if err != nil {
		t.Fatalf("%v", err)
	}
	manifestFile, perm := filepath.Join(manifestDir, "v2", "default"), os.FileMode(0644)
	if err := ioutil.WriteFile(manifestFile, data, perm); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", manifestFile, err, perm)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ctx.Run().Chdir(cwd)
	if err := ctx.Run().Chdir(manifestDir); err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Git().CommitFile(manifestFile, "creating manifest"); err != nil {
		t.Fatalf("%v", err)
	}
}

func deleteProject(t *testing.T, ctx *tool.Context, manifestDir, project string) {
	manifestFile := filepath.Join(manifestDir, "v2", "default")
	data, err := ioutil.ReadFile(manifestFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", manifestFile, err)
	}
	manifest := Manifest{}
	if err := xml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n", err, data)
	}
	manifest.Projects = append(manifest.Projects, Project{Exclude: true, Name: project})
	commitManifest(t, ctx, &manifest, manifestDir)
}

func holdProjectBack(t *testing.T, ctx *tool.Context, manifestDir, project string) {
	// Identify the current revision.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ctx.Run().Chdir(cwd)
	if err := ctx.Run().Chdir(project); err != nil {
		t.Fatalf("%v", err)
	}
	revision, err := ctx.Git().LatestCommitID()
	if err != nil {
		t.Fatalf("%v", err)
	}

	// Fix the revision in the manifest file.
	manifestFile := filepath.Join(manifestDir, "v2", "default")
	data, err := ioutil.ReadFile(manifestFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", manifestFile, err)
	}
	manifest := Manifest{}
	if err := xml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n", err, data)
	}
	updated := false
	for i, p := range manifest.Projects {
		if p.Name == project {
			p.Revision = revision
			manifest.Projects[i] = p
			updated = true
			break
		}
	}
	if !updated {
		t.Fatalf("failed to fix revision for project %v", project)
	}
	commitManifest(t, ctx, &manifest, manifestDir)
}

func localProjectName(i int) string {
	return "test-local-project-" + fmt.Sprintf("%d", i+1)
}

func moveProject(t *testing.T, ctx *tool.Context, manifestDir, project, dst string) {
	manifestFile := filepath.Join(manifestDir, "v2", "default")
	data, err := ioutil.ReadFile(manifestFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", manifestFile, err)
	}
	manifest := Manifest{}
	if err := xml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n", err, data)
	}
	updated := false
	for i, p := range manifest.Projects {
		if p.Name == project {
			p.Path = dst
			manifest.Projects[i] = p
			updated = true
			break
		}
	}
	if !updated {
		t.Fatalf("failed to set path for project %v", project)
	}
	commitManifest(t, ctx, &manifest, manifestDir)
}

func remoteProjectName(i int) string {
	return "test-remote-project-" + fmt.Sprintf("%d", i+1)
}

func setupNewProject(t *testing.T, ctx *tool.Context, dir, name string) string {
	projectDir, perm := filepath.Join(dir, name), os.FileMode(0755)
	if err := ctx.Run().MkdirAll(projectDir, perm); err != nil {
		t.Fatalf("%v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ctx.Run().Chdir(cwd)
	if err := ctx.Run().Chdir(projectDir); err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Git().Init(projectDir); err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Git().Commit(); err != nil {
		t.Fatalf("%v", err)
	}
	return projectDir
}

func writeEmptyMetadata(t *testing.T, ctx *tool.Context, projectDir string) {
	if err := ctx.Run().Chdir(projectDir); err != nil {
		t.Fatalf("%v", err)
	}
	metadataDir := filepath.Join(projectDir, ".v23")
	if err := ctx.Run().MkdirAll(metadataDir, os.FileMode(0755)); err != nil {
		t.Fatalf("%v", err)
	}
	bytes, err := xml.Marshal(Project{})
	if err != nil {
		t.Fatalf("Marshal() failed: %v", err)
	}
	metadataFile := filepath.Join(metadataDir, "metadata.v2")
	if err := ctx.Run().WriteFile(metadataFile, bytes, os.FileMode(0644)); err != nil {
		t.Fatalf("%v", err)
	}
}

func writeReadme(t *testing.T, ctx *tool.Context, projectDir, message string) {
	path, perm := filepath.Join(projectDir, "README"), os.FileMode(0644)
	if err := ioutil.WriteFile(path, []byte(message), perm); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", path, perm, err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer ctx.Run().Chdir(cwd)
	if err := ctx.Run().Chdir(projectDir); err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Git().CommitFile(path, "creating README"); err != nil {
		t.Fatalf("%v", err)
	}
}

// TestUpdateUniverse is a comprehensive test of the "v23 update"
// logic that handles projects.
//
// TODO(jsimsa): Add tests for the logic that updates tools.
func TestUpdateUniverse(t *testing.T) {
	// Setup an instance of vanadium universe, creating the remote
	// repositories for the manifest and projects under the
	// "remote" directory, which is ignored from the consideration
	// of LocalProjects().
	ctx := tool.NewDefaultContext()
	rootDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer ctx.Run().RemoveAll(rootDir)
	localDir := filepath.Join(rootDir, "local")
	remoteDir := filepath.Join(rootDir, "remote")
	localManifest := setupNewProject(t, ctx, localDir, ".manifest")
	writeEmptyMetadata(t, ctx, localManifest)
	remoteManifest := setupNewProject(t, ctx, remoteDir, "test-remote-manifest")
	addRemote(t, ctx, localManifest, "origin", remoteManifest)
	numProjects, remoteProjects := 4, []string{}
	for i := 0; i < numProjects; i++ {
		remoteProject := setupNewProject(t, ctx, remoteDir, remoteProjectName(i))
		remoteProjects = append(remoteProjects, remoteProject)
	}
	createRemoteManifest(t, ctx, remoteManifest, remoteProjects)
	oldRoot := os.Getenv("V23_ROOT")
	if err := os.Setenv("V23_ROOT", localDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("V23_ROOT", oldRoot)

	// Check that calling UpdateUniverse() creates local copies of
	// the remote repositories, advancing projects to HEAD or to
	// the fixed revision set in the manifest.
	for _, remoteProject := range remoteProjects {
		writeReadme(t, ctx, remoteProject, "revision 1")
	}
	holdProjectBack(t, ctx, remoteManifest, remoteProjects[0])
	for _, remoteProject := range remoteProjects {
		writeReadme(t, ctx, remoteProject, "revision 2")
	}
	if err := UpdateUniverse(ctx, false); err != nil {
		t.Fatalf("%v", err)
	}
	checkCreateFn := func(i int, revision string) {
		localProject := filepath.Join(localDir, localProjectName(i))
		if i == 0 {
			checkReadme(t, ctx, localProject, "revision 1")
		} else {
			checkReadme(t, ctx, localProject, revision)
		}
	}
	for i, _ := range remoteProjects {
		checkCreateFn(i, "revision 2")
	}

	// Commit more work to the remote repositories and check that
	// calling UpdateUnivers() advances project to HEAD or to the
	// fixed revision set in the manifest.
	holdProjectBack(t, ctx, remoteManifest, remoteProjects[1])
	for _, remoteProject := range remoteProjects {
		writeReadme(t, ctx, remoteProject, "revision 3")
	}
	if err := UpdateUniverse(ctx, false); err != nil {
		t.Fatalf("%v", err)
	}
	checkUpdateFn := func(i int, revision string) {
		if i == 1 {
			checkReadme(t, ctx, filepath.Join(localDir, localProjectName(i)), "revision 2")
		} else {
			checkCreateFn(i, revision)
		}
	}
	for i, _ := range remoteProjects {
		checkUpdateFn(i, "revision 3")
	}

	// Create an uncommitted file and make sure UpdateUniverse()
	// does not drop it. This ensures that the "git reset --hard"
	// mechanism used for pointing the master branch to a fixed
	// revision does not lose work in progress.
	file, perm, want := filepath.Join(remoteProjects[1], "uncommitted_file"), os.FileMode(0644), []byte("uncommitted work")
	if err := ioutil.WriteFile(file, want, perm); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", file, err, perm)
	}
	if err := UpdateUniverse(ctx, false); err != nil {
		t.Fatalf("%v", err)
	}
	got, err := ioutil.ReadFile(file)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if bytes.Compare(got, want) != 0 {
		t.Fatalf("unexpected content %v:\ngot\n%s\nwant\n%s\n", remoteProjects[1], got, want)
	}

	// Update the local path at which a remote project is to be
	// located and check that UpdateUniverse() moves the local
	// copy of the project.
	destination := filepath.Join("test", localProjectName(2))
	moveProject(t, ctx, remoteManifest, remoteProjects[2], destination)
	if err := UpdateUniverse(ctx, false); err != nil {
		t.Fatalf("%v", err)
	}
	checkMoveFn := func(i int, revision string) {
		if i == 2 {
			checkReadme(t, ctx, filepath.Join(localDir, destination), revision)
		} else {
			checkUpdateFn(i, revision)
		}
	}
	for i, _ := range remoteProjects {
		checkMoveFn(i, "revision 3")
	}

	// Delete a remote project and check that UpdateUniverse()
	// deletes the local copy of the project.
	deleteProject(t, ctx, remoteManifest, remoteProjects[3])
	if err := UpdateUniverse(ctx, true); err != nil {
		t.Fatalf("%v", err)
	}
	checkDeleteFn := func(i int, revision string) {
		if i == 3 {
			localProject := filepath.Join(localDir, localProjectName(i))
			if _, err := os.Stat(localProject); err == nil {
				t.Fatalf("project %v has not been deleted", localProject)
			} else {
				if !os.IsNotExist(err) {
					t.Fatalf("%v", err)
				}
			}
		} else {
			checkMoveFn(i, revision)
		}
	}
	for i, _ := range remoteProjects {
		checkDeleteFn(i, "revision 3")
	}

	// Create a local manifest that imports the remote manifest
	// and check that UpdateUniverse() has no effect.
	createLocalManifestStub(t, ctx, localDir)
	if err := UpdateUniverse(ctx, true); err != nil {
		t.Fatalf("%v", err)
	}
	for i, _ := range remoteProjects {
		checkDeleteFn(i, "revision 3")
	}

	// Create a local manifest that matches the remote manifest,
	// then revert the remote manifest to its initial version and
	// check that UpdateUniverse() has no effect.
	createLocalManifestCopy(t, ctx, localDir, remoteManifest)
	createRemoteManifest(t, ctx, remoteManifest, remoteProjects)
	if err := UpdateUniverse(ctx, true); err != nil {
		t.Fatalf("%v", err)
	}
	for i, _ := range remoteProjects {
		checkDeleteFn(i, "revision 3")
	}
}
