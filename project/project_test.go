// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// TODO(jsimsa): Switch this test to using FakeJiriRoot.

package project

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"v.io/jiri/jiri"
	"v.io/jiri/jiri/jiritest"
)

func addRemote(t *testing.T, jirix *jiri.X, localProject, name, remoteProject string) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer jirix.Run().Chdir(cwd)
	if err := jirix.Run().Chdir(localProject); err != nil {
		t.Fatalf("%v", err)
	}
	if err := jirix.Git().AddRemote(name, remoteProject); err != nil {
		t.Fatalf("%v", err)
	}
}

func checkReadme(t *testing.T, jirix *jiri.X, project, message string) {
	if _, err := jirix.Run().Stat(project); err != nil {
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

// Checks that /.jiri/ is ignored in a local project checkout
func checkGitIgnore(t *testing.T, jirix *jiri.X, project string) {
	if _, err := jirix.Run().Stat(project); err != nil {
		t.Fatalf("%v", err)
	}
	gitInfoExcludeFile := filepath.Join(project, ".git", "info", "exclude")
	data, err := ioutil.ReadFile(gitInfoExcludeFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", gitInfoExcludeFile, err)
	}
	excludeString := "/.jiri/"
	if !strings.Contains(string(data), excludeString) {
		t.Fatalf("Did not find \"%v\" in exclude file", excludeString)
	}
}

func createLocalManifestCopy(t *testing.T, jirix *jiri.X, dir, manifestDir string) {
	// Load the remote manifest.
	manifestFile := filepath.Join(manifestDir, "v2", "default")
	data, err := ioutil.ReadFile(manifestFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", manifestFile, err)
	}
	manifest := Manifest{}
	if err := xml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, data)
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

func createLocalManifestStub(t *testing.T, jirix *jiri.X, dir string) {
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

func createRemoteManifest(t *testing.T, jirix *jiri.X, dir string, remotes []string) {
	manifestDir, perm := filepath.Join(dir, "v2"), os.FileMode(0755)
	if err := jirix.Run().MkdirAll(manifestDir, perm); err != nil {
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
	manifest.Hosts = []Host{
		Host{
			Name:     "gerrit",
			Location: "git://example.com/gerrit",
		},
		Host{
			Name:     "git",
			Location: "git://example.com/git",
		},
	}
	commitManifest(t, jirix, &manifest, dir)
}

func commitManifest(t *testing.T, jirix *jiri.X, manifest *Manifest, manifestDir string) {
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
	defer jirix.Run().Chdir(cwd)
	if err := jirix.Run().Chdir(manifestDir); err != nil {
		t.Fatalf("%v", err)
	}
	if err := jirix.Git().CommitFile(manifestFile, "creating manifest"); err != nil {
		t.Fatalf("%v", err)
	}
}

func deleteProject(t *testing.T, jirix *jiri.X, manifestDir, project string) {
	manifestFile := filepath.Join(manifestDir, "v2", "default")
	data, err := ioutil.ReadFile(manifestFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", manifestFile, err)
	}
	manifest := Manifest{}
	if err := xml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, data)
	}
	manifest.Projects = append(manifest.Projects, Project{Exclude: true, Name: project})
	commitManifest(t, jirix, &manifest, manifestDir)
}

// Identify the current revision for a given project.
func currentRevision(t *testing.T, jirix *jiri.X, project string) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer jirix.Run().Chdir(cwd)
	if err := jirix.Run().Chdir(project); err != nil {
		t.Fatalf("%v", err)
	}
	revision, err := jirix.Git().CurrentRevision()
	if err != nil {
		t.Fatalf("%v", err)
	}
	return revision
}

// Fix the revision in the manifest file.
func setRevisionForProject(t *testing.T, jirix *jiri.X, manifestDir, project, revision string) {
	manifestFile := filepath.Join(manifestDir, "v2", "default")
	data, err := ioutil.ReadFile(manifestFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", manifestFile, err)
	}
	manifest := Manifest{}
	if err := xml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, data)
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
	commitManifest(t, jirix, &manifest, manifestDir)
}

func holdProjectBack(t *testing.T, jirix *jiri.X, manifestDir, project string) {
	revision := currentRevision(t, jirix, project)
	setRevisionForProject(t, jirix, manifestDir, project, revision)
}

func localProjectName(i int) string {
	return "test-local-project-" + fmt.Sprintf("%d", i)
}

func moveProject(t *testing.T, jirix *jiri.X, manifestDir, project, dst string) {
	manifestFile := filepath.Join(manifestDir, "v2", "default")
	data, err := ioutil.ReadFile(manifestFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", manifestFile, err)
	}
	manifest := Manifest{}
	if err := xml.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("Unmarshal() failed: %v\n%v", err, data)
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
	commitManifest(t, jirix, &manifest, manifestDir)
}

func remoteProjectName(i int) string {
	return "test-remote-project-" + fmt.Sprintf("%d", i)
}

func setupNewProject(t *testing.T, jirix *jiri.X, dir, name string, ignore bool) string {
	projectDir, perm := filepath.Join(dir, name), os.FileMode(0755)
	if err := jirix.Run().MkdirAll(projectDir, perm); err != nil {
		t.Fatalf("%v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer jirix.Run().Chdir(cwd)
	if err := jirix.Run().Chdir(projectDir); err != nil {
		t.Fatalf("%v", err)
	}
	if err := jirix.Git().Init(projectDir); err != nil {
		t.Fatalf("%v", err)
	}
	if ignore {
		ignoreFile := filepath.Join(projectDir, ".gitignore")
		if err := jirix.Run().WriteFile(ignoreFile, []byte(metadataDirName), os.FileMode(0644)); err != nil {
			t.Fatalf("%v", err)
		}
		if err := jirix.Git().Add(ignoreFile); err != nil {
			t.Fatalf("%v", err)
		}
	}
	if err := jirix.Git().Commit(); err != nil {
		t.Fatalf("%v", err)
	}
	return projectDir
}

func writeEmptyMetadata(t *testing.T, jirix *jiri.X, projectDir string) {
	if err := jirix.Run().Chdir(projectDir); err != nil {
		t.Fatalf("%v", err)
	}
	metadataDir := filepath.Join(projectDir, metadataDirName)
	if err := jirix.Run().MkdirAll(metadataDir, os.FileMode(0755)); err != nil {
		t.Fatalf("%v", err)
	}
	bytes, err := xml.Marshal(Project{})
	if err != nil {
		t.Fatalf("Marshal() failed: %v", err)
	}
	metadataFile := filepath.Join(metadataDir, metadataFileName)
	if err := jirix.Run().WriteFile(metadataFile, bytes, os.FileMode(0644)); err != nil {
		t.Fatalf("%v", err)
	}
}

func writeReadme(t *testing.T, jirix *jiri.X, projectDir, message string) {
	path, perm := filepath.Join(projectDir, "README"), os.FileMode(0644)
	if err := ioutil.WriteFile(path, []byte(message), perm); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", path, perm, err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer jirix.Run().Chdir(cwd)
	if err := jirix.Run().Chdir(projectDir); err != nil {
		t.Fatalf("%v", err)
	}
	if err := jirix.Git().CommitFile(path, "creating README"); err != nil {
		t.Fatalf("%v", err)
	}
}

func createAndCheckoutBranch(t *testing.T, jirix *jiri.X, projectDir, branch string) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer jirix.Run().Chdir(cwd)
	if err := jirix.Run().Chdir(projectDir); err != nil {
		t.Fatalf("%v", err)
	}
	if err := jirix.Git().CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}
}

func resetToOriginMaster(t *testing.T, jirix *jiri.X, projectDir string) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer jirix.Run().Chdir(cwd)
	if err := jirix.Run().Chdir(projectDir); err != nil {
		t.Fatalf("%v", err)
	}
	if err := jirix.Git().Reset("origin/master"); err != nil {
		t.Fatalf("%v", err)
	}
}

func checkProjectsMatchPaths(t *testing.T, gotProjects Projects, wantProjectPaths []string) {
	gotProjectPaths := []string{}
	for _, p := range gotProjects {
		gotProjectPaths = append(gotProjectPaths, p.Path)
	}
	sort.Strings(gotProjectPaths)
	sort.Strings(wantProjectPaths)
	if !reflect.DeepEqual(gotProjectPaths, wantProjectPaths) {
		t.Errorf("project paths got %v, want %v", gotProjectPaths, wantProjectPaths)
	}
}

// TestLocalProjects tests the behavior of the LocalProjects method with
// different ScanModes.
func TestLocalProjects(t *testing.T) {
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()

	manifestDir := setupNewProject(t, jirix, jirix.Root, ".manifest", false)

	// Create some projects.
	numProjects, projectPaths := 3, []string{}
	for i := 0; i < numProjects; i++ {
		projectName := localProjectName(i)
		projectPath := setupNewProject(t, jirix, jirix.Root, projectName, true)
		project := Project{
			Path:     projectPath,
			Name:     projectName,
			Protocol: "git",
		}
		if err := writeMetadata(jirix, project, projectPath); err != nil {
			t.Fatalf("writeMetadata %v %v) failed: %v\n", project, projectPath, err)
		}
		projectPaths = append(projectPaths, projectPath)
	}

	// Create manifest but only tell it about the first project.
	createRemoteManifest(t, jirix, manifestDir, projectPaths[:1])

	// LocalProjects with scanMode = FastScan should only find the first
	// project.
	foundProjects, err := LocalProjects(jirix, FastScan)
	if err != nil {
		t.Fatalf("LocalProjects(%v) failed: %v", FastScan, err)
	}
	checkProjectsMatchPaths(t, foundProjects, projectPaths[:1])

	// LocalProjects with scanMode = FullScan should find all projects.
	foundProjects, err = LocalProjects(jirix, FullScan)
	if err != nil {
		t.Fatalf("LocalProjects(%v) failed: %v", FastScan, err)
	}
	checkProjectsMatchPaths(t, foundProjects, projectPaths[:])

	// Check that deleting a project forces LocalProjects to run a full scan,
	// even if FastScan is specified.
	if err := jirix.Run().RemoveAll(projectPaths[0]); err != nil {
		t.Fatalf("RemoveAll(%v) failed: %v", projectPaths[0])
	}
	foundProjects, err = LocalProjects(jirix, FastScan)
	if err != nil {
		t.Fatalf("LocalProjects(%v) failed: %v", FastScan, err)
	}
	checkProjectsMatchPaths(t, foundProjects, projectPaths[1:])
}

// TestUpdateUniverse is a comprehensive test of the "jiri update"
// logic that handles projects.
//
// TODO(jsimsa): Add tests for the logic that updates tools.
func TestUpdateUniverse(t *testing.T) {
	// Setup an instance of jiri universe, creating the remote repositories for
	// the manifest and projects under the "remote" directory, which is ignored
	// from the consideration of LocalProjects().
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()

	localDir := filepath.Join(jirix.Root, "local")
	remoteDir := filepath.Join(jirix.Root, "remote")
	if err := os.Setenv("JIRI_ROOT", localDir); err != nil {
		t.Fatalf("%v", err)
	}

	localManifest := setupNewProject(t, jirix, localDir, ".manifest", false)
	writeEmptyMetadata(t, jirix, localManifest)
	remoteManifest := setupNewProject(t, jirix, remoteDir, "test-remote-manifest", false)
	addRemote(t, jirix, localManifest, "origin", remoteManifest)
	numProjects, remoteProjects := 5, []string{}
	for i := 0; i < numProjects; i++ {
		remoteProject := setupNewProject(t, jirix, remoteDir, remoteProjectName(i), true)
		remoteProjects = append(remoteProjects, remoteProject)
	}
	createRemoteManifest(t, jirix, remoteManifest, remoteProjects)

	// Check that calling UpdateUniverse() creates local copies of
	// the remote repositories, advancing projects to HEAD or to
	// the fixed revision set in the manifest.
	for _, remoteProject := range remoteProjects {
		writeReadme(t, jirix, remoteProject, "revision 1")
	}
	holdProjectBack(t, jirix, remoteManifest, remoteProjects[0])
	for _, remoteProject := range remoteProjects {
		writeReadme(t, jirix, remoteProject, "revision 2")
	}
	if err := UpdateUniverse(jirix, false); err != nil {
		t.Fatalf("%v", err)
	}
	checkCreateFn := func(i int, revision string) {
		localProject := filepath.Join(localDir, localProjectName(i))
		checkGitIgnore(t, jirix, localProject)
		if i == 0 {
			checkReadme(t, jirix, localProject, "revision 1")
		} else {
			checkReadme(t, jirix, localProject, revision)
		}
	}
	for i, _ := range remoteProjects {
		checkCreateFn(i, "revision 2")
	}

	// Commit more work to the remote repositories and check that
	// calling UpdateUniverse() advances project to HEAD or to the
	// fixed revision set in the manifest.
	holdProjectBack(t, jirix, remoteManifest, remoteProjects[1])
	for _, remoteProject := range remoteProjects {
		writeReadme(t, jirix, remoteProject, "revision 3")
	}
	if err := UpdateUniverse(jirix, false); err != nil {
		t.Fatalf("%v", err)
	}
	checkUpdateFn := func(i int, revision string) {
		if i == 1 {
			checkReadme(t, jirix, filepath.Join(localDir, localProjectName(i)), "revision 2")
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
	if err := UpdateUniverse(jirix, false); err != nil {
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
	moveProject(t, jirix, remoteManifest, remoteProjects[2], destination)
	if err := UpdateUniverse(jirix, false); err != nil {
		t.Fatalf("%v", err)
	}
	checkMoveFn := func(i int, revision string) {
		if i == 2 {
			checkReadme(t, jirix, filepath.Join(localDir, destination), revision)
		} else {
			checkUpdateFn(i, revision)
		}
	}
	for i, _ := range remoteProjects {
		checkMoveFn(i, "revision 3")
	}

	// Delete a remote project and check that UpdateUniverse()
	// deletes the local copy of the project.
	deleteProject(t, jirix, remoteManifest, remoteProjects[3])
	if err := UpdateUniverse(jirix, true); err != nil {
		t.Fatalf("%v", err)
	}
	checkDeleteFn := func(i int, revision string) {
		if i == 3 {
			localProject := filepath.Join(localDir, localProjectName(i))
			if _, err := jirix.Run().Stat(localProject); err == nil {
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

	// Commit to a non-master branch of a remote project and check that
	// UpdateUniverse() can update the local project to point to a revision on
	// that branch.
	writeReadme(t, jirix, remoteProjects[4], "master commit")
	createAndCheckoutBranch(t, jirix, remoteProjects[4], "non_master")
	writeReadme(t, jirix, remoteProjects[4], "non master commit")
	remoteBranchRevision := currentRevision(t, jirix, remoteProjects[4])
	setRevisionForProject(t, jirix, remoteManifest, remoteProjects[4], remoteBranchRevision)
	if err := UpdateUniverse(jirix, true); err != nil {
		t.Fatalf("%v", err)
	}
	localProject := filepath.Join(localDir, localProjectName(4))
	localBranchRevision := currentRevision(t, jirix, localProject)
	if localBranchRevision != remoteBranchRevision {
		t.Fatalf("project 4 is at revision %v, expected %v\n", localBranchRevision, remoteBranchRevision)
	}
	// Reset back to origin/master so the next update without a "revision" works.
	resetToOriginMaster(t, jirix, localProject)

	// Create a local manifest that imports the remote manifest
	// and check that UpdateUniverse() has no effect.
	createLocalManifestStub(t, jirix, localDir)
	if err := UpdateUniverse(jirix, true); err != nil {
		t.Fatalf("%v", err)
	}

	checkRebaseFn := func(i int, revision string) {
		if i == 4 {
			checkReadme(t, jirix, localProject, "non master commit")
		} else {
			checkDeleteFn(i, revision)
		}
	}
	for i, _ := range remoteProjects {
		checkRebaseFn(i, "revision 3")
	}

	// Create a local manifest that matches the remote manifest,
	// then revert the remote manifest to its initial version and
	// check that UpdateUniverse() has no effect.
	createLocalManifestCopy(t, jirix, localDir, remoteManifest)
	createRemoteManifest(t, jirix, remoteManifest, remoteProjects)
	if err := UpdateUniverse(jirix, true); err != nil {
		t.Fatalf("%v", err)
	}
	for i, _ := range remoteProjects {
		checkRebaseFn(i, "revision 3")
	}
}

// TestUnsupportedProtocolErr checks that calling
// UnsupportedPrototoclErr.Error() does not result in an infinite loop.
func TestUnsupportedPrototocolErr(t *testing.T) {
	err := UnsupportedProtocolErr("foo")
	_ = err.Error()
}
