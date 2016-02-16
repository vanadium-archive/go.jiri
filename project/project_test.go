// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"v.io/jiri/gitutil"
	"v.io/jiri/jiri"
	"v.io/jiri/jiritest"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
)

func addRemote(t *testing.T, jirix *jiri.X, localProject, name, remoteProject string) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer jirix.NewSeq().Chdir(cwd)
	if err := jirix.NewSeq().Chdir(localProject).Done(); err != nil {
		t.Fatalf("%v", err)
	}
	if err := gitutil.New(jirix.NewSeq()).AddRemote(name, remoteProject); err != nil {
		t.Fatalf("%v", err)
	}
}

func checkReadme(t *testing.T, jirix *jiri.X, project, message string) {
	if _, err := jirix.NewSeq().Stat(project); err != nil {
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
	if _, err := jirix.NewSeq().Stat(project); err != nil {
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
	m, err := project.ManifestFromFile(jirix, filepath.Join(manifestDir, "v2", "default"))
	if err != nil {
		t.Fatal(err)
	}
	// Store the manifest locally.
	if err := m.ToFile(jirix, filepath.Join(dir, ".local_manifest")); err != nil {
		t.Fatal(err)
	}
}

func createLocalManifestStub(t *testing.T, jirix *jiri.X, dir string) {
	// Create a manifest stub.
	manifest := project.Manifest{}
	imp := project.Import{}
	imp.Name = "default"
	manifest.Imports = append(manifest.Imports, imp)
	// Store the manifest locally.
	if err := manifest.ToFile(jirix, filepath.Join(dir, ".local_manifest")); err != nil {
		t.Fatal(err)
	}
}

func commitFile(t *testing.T, jirix *jiri.X, dir, file, msg string) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer jirix.NewSeq().Chdir(cwd)
	if err := jirix.NewSeq().Chdir(dir).Done(); err != nil {
		t.Fatal(err)
	}
	if err := gitutil.New(jirix.NewSeq()).CommitFile(file, msg); err != nil {
		t.Fatal(err)
	}
}

func createRemoteManifest(t *testing.T, jirix *jiri.X, dir string, remotes []string) {
	manifestDir, perm := filepath.Join(dir, "v2"), os.FileMode(0755)
	if err := jirix.NewSeq().MkdirAll(manifestDir, perm).Done(); err != nil {
		t.Fatalf("%v", err)
	}
	manifest := project.Manifest{}
	for i, remote := range remotes {
		project := project.Project{
			Name:     remote,
			Path:     localProjectName(i),
			Protocol: "git",
			Remote:   remote,
		}
		manifest.Projects = append(manifest.Projects, project)
	}
	commitManifest(t, jirix, &manifest, dir)
}

func commitManifest(t *testing.T, jirix *jiri.X, manifest *project.Manifest, manifestDir string) {
	manifestFile := filepath.Join(manifestDir, "v2", "default")
	if err := manifest.ToFile(jirix, manifestFile); err != nil {
		t.Fatal(err)
	}
	commitFile(t, jirix, manifestDir, manifestFile, "creating manifest")
}

func createProject(t *testing.T, jirix *jiri.X, manifestDir, name, remote, path string) {
	m, err := project.ManifestFromFile(jirix, filepath.Join(manifestDir, "v2", "default"))
	if err != nil {
		t.Fatal(err)
	}
	m.Projects = append(m.Projects, project.Project{Name: name, Remote: remote, Path: path})
	commitManifest(t, jirix, m, manifestDir)
}

func deleteProject(t *testing.T, jirix *jiri.X, manifestDir, remote string) {
	m, err := project.ManifestFromFile(jirix, filepath.Join(manifestDir, "v2", "default"))
	if err != nil {
		t.Fatal(err)
	}
	deleteKey := project.MakeProjectKey(remote, remote)
	var projects []project.Project
	for _, p := range m.Projects {
		if p.Key() != deleteKey {
			projects = append(projects, p)
		}
	}
	m.Projects = projects
	commitManifest(t, jirix, m, manifestDir)
}

// Identify the current revision for a given project.
func currentRevision(t *testing.T, jirix *jiri.X, name string) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer jirix.NewSeq().Chdir(cwd)
	if err := jirix.NewSeq().Chdir(name).Done(); err != nil {
		t.Fatalf("%v", err)
	}
	revision, err := gitutil.New(jirix.NewSeq()).CurrentRevision()
	if err != nil {
		t.Fatalf("%v", err)
	}
	return revision
}

// Set the revision in the manifest file.
func setRevisionForProject(t *testing.T, jirix *jiri.X, manifestDir, name, revision string) {
	m, err := project.ManifestFromFile(jirix, filepath.Join(manifestDir, "v2", "default"))
	if err != nil {
		t.Fatal(err)
	}
	updated := false
	for i, p := range m.Projects {
		if p.Name == name {
			p.Revision = revision
			m.Projects[i] = p
			updated = true
			break
		}
	}
	if !updated {
		t.Fatalf("failed to set revision for project %v", name)
	}
	commitManifest(t, jirix, m, manifestDir)
}

// Set the remote branch in the manifest file.
func setRemoteBranchForProject(t *testing.T, jirix *jiri.X, manifestDir, name, remoteBranch string) {
	m, err := project.ManifestFromFile(jirix, filepath.Join(manifestDir, "v2", "default"))
	if err != nil {
		t.Fatal(err)
	}
	updated := false
	for i, p := range m.Projects {
		if p.Name == name {
			p.RemoteBranch = remoteBranch
			m.Projects[i] = p
			updated = true
			break
		}
	}
	if !updated {
		t.Fatalf("failed to fix remote branch for project %v", name)
	}
	commitManifest(t, jirix, m, manifestDir)
}

func holdProjectBack(t *testing.T, jirix *jiri.X, manifestDir, name string) {
	revision := currentRevision(t, jirix, name)
	setRevisionForProject(t, jirix, manifestDir, name, revision)
}

func localProjectName(i int) string {
	return "test-local-project-" + fmt.Sprintf("%d", i)
}

func moveProject(t *testing.T, jirix *jiri.X, manifestDir, name, dst string) {
	m, err := project.ManifestFromFile(jirix, filepath.Join(manifestDir, "v2", "default"))
	if err != nil {
		t.Fatal(err)
	}
	updated := false
	for i, p := range m.Projects {
		if p.Name == name {
			p.Path = dst
			m.Projects[i] = p
			updated = true
			break
		}
	}
	if !updated {
		t.Fatalf("failed to set path for project %v", name)
	}
	commitManifest(t, jirix, m, manifestDir)
}

func remoteProjectName(i int) string {
	return "test-remote-project-" + fmt.Sprintf("%d", i)
}

func setupNewProject(t *testing.T, jirix *jiri.X, dir, name string, ignore bool) string {
	projectDir, perm := filepath.Join(dir, name), os.FileMode(0755)
	s := jirix.NewSeq()
	if err := s.MkdirAll(projectDir, perm).Done(); err != nil {
		t.Fatalf("%v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer jirix.NewSeq().Chdir(cwd)
	if err := s.Chdir(projectDir).Done(); err != nil {
		t.Fatalf("%v", err)
	}
	if err := gitutil.New(jirix.NewSeq()).Init(projectDir); err != nil {
		t.Fatalf("%v", err)
	}
	if ignore {
		ignoreFile := filepath.Join(projectDir, ".gitignore")
		if err := s.WriteFile(ignoreFile, []byte(jiri.ProjectMetaDir), os.FileMode(0644)).Done(); err != nil {
			t.Fatalf("%v", err)
		}
		if err := gitutil.New(jirix.NewSeq()).Add(ignoreFile); err != nil {
			t.Fatalf("%v", err)
		}
	}
	if err := gitutil.New(jirix.NewSeq()).Commit(); err != nil {
		t.Fatalf("%v", err)
	}
	return projectDir
}

func writeEmptyMetadata(t *testing.T, jirix *jiri.X, projectDir string) {
	metadataFile := filepath.Join(projectDir, jiri.ProjectMetaDir, jiri.ProjectMetaFile)
	p := project.Project{}
	if err := p.ToFile(jirix, metadataFile); err != nil {
		t.Fatal(err)
	}
}

func writeReadme(t *testing.T, jirix *jiri.X, projectDir, message string) {
	path, perm := filepath.Join(projectDir, "README"), os.FileMode(0644)
	if err := ioutil.WriteFile(path, []byte(message), perm); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", path, perm, err)
	}
	commitFile(t, jirix, projectDir, path, "creating README")
}

func createAndCheckoutBranch(t *testing.T, jirix *jiri.X, projectDir, branch string) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer jirix.NewSeq().Chdir(cwd)
	if err := jirix.NewSeq().Chdir(projectDir).Done(); err != nil {
		t.Fatalf("%v", err)
	}
	if err := gitutil.New(jirix.NewSeq()).CreateAndCheckoutBranch(branch); err != nil {
		t.Fatalf("%v", err)
	}
}

func resetToOriginMaster(t *testing.T, jirix *jiri.X, projectDir string) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer jirix.NewSeq().Chdir(cwd)
	if err := jirix.NewSeq().Chdir(projectDir).Done(); err != nil {
		t.Fatalf("%v", err)
	}
	if err := gitutil.New(jirix.NewSeq()).Reset("origin/master"); err != nil {
		t.Fatalf("%v", err)
	}
}

func checkProjectsMatchPaths(t *testing.T, gotProjects project.Projects, wantProjectPaths []string) {
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

	// Create some projects.
	numProjects, projectPaths := 3, []string{}
	for i := 0; i < numProjects; i++ {
		name := localProjectName(i)
		path := setupNewProject(t, jirix, jirix.Root, name, true)
		p := project.Project{
			Path:     path,
			Name:     name,
			Protocol: "git",
		}
		if err := project.InternalWriteMetadata(jirix, p, path); err != nil {
			t.Fatalf("writeMetadata %v %v) failed: %v\n", p, path, err)
		}
		projectPaths = append(projectPaths, path)
	}

	// Create a latest update snapshot but only tell it about the first project.
	manifest := project.Manifest{
		Projects: []project.Project{
			{
				Name:     projectPaths[0],
				Path:     localProjectName(0),
				Protocol: "git",
				Remote:   projectPaths[0],
			},
		},
	}
	if err := jirix.NewSeq().MkdirAll(jirix.UpdateHistoryDir(), 0755).Done(); err != nil {
		t.Fatalf("MkdirAll(%v) failed: %v", jirix.UpdateHistoryDir(), err)
	}
	if err := manifest.ToFile(jirix, jirix.UpdateHistoryLatestLink()); err != nil {
		t.Fatalf("manifest.ToFile(%v) failed: %v", jirix.UpdateHistoryLatestLink(), err)
	}

	// LocalProjects with scanMode = FastScan should only find the first
	// project.
	foundProjects, err := project.LocalProjects(jirix, project.FastScan)
	if err != nil {
		t.Fatalf("LocalProjects(%v) failed: %v", project.FastScan, err)
	}
	checkProjectsMatchPaths(t, foundProjects, projectPaths[:1])

	// LocalProjects with scanMode = FullScan should find all projects.
	foundProjects, err = project.LocalProjects(jirix, project.FullScan)
	if err != nil {
		t.Fatalf("LocalProjects(%v) failed: %v", project.FastScan, err)
	}
	checkProjectsMatchPaths(t, foundProjects, projectPaths[:])

	// Check that deleting a project forces LocalProjects to run a full scan,
	// even if FastScan is specified.
	if err := jirix.NewSeq().RemoveAll(projectPaths[0]).Done(); err != nil {
		t.Fatalf("RemoveAll(%v) failed: %v", projectPaths[0])
	}
	foundProjects, err = project.LocalProjects(jirix, project.FastScan)
	if err != nil {
		t.Fatalf("LocalProjects(%v) failed: %v", project.FastScan, err)
	}
	checkProjectsMatchPaths(t, foundProjects, projectPaths[1:])
}

// TestUpdateUniverse is a comprehensive test of the "jiri update"
// logic that handles projects.
//
// TODO(jsimsa): Add tests for the logic that updates tools.
func TestUpdateUniverse(t *testing.T) {
	// Setup an instance of jiri universe, creating the remote repositories for
	// the manifest and projects under the ".remote" directory, which is ignored
	// from the consideration of LocalProjects().
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()

	localDir := jirix.Root
	remoteDir := filepath.Join(jirix.Root, ".remote")

	localManifest := setupNewProject(t, jirix, localDir, ".manifest", false)
	writeEmptyMetadata(t, jirix, localManifest)
	remoteManifest := setupNewProject(t, jirix, remoteDir, "test-remote-manifest", false)
	addRemote(t, jirix, localManifest, "origin", remoteManifest)
	numProjects, remoteProjects := 6, []string{}
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
	if err := project.UpdateUniverse(jirix, false); err != nil {
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
	if err := project.UpdateUniverse(jirix, false); err != nil {
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
	if err := project.UpdateUniverse(jirix, false); err != nil {
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
	if err := project.UpdateUniverse(jirix, false); err != nil {
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
	if err := project.UpdateUniverse(jirix, true); err != nil {
		t.Fatalf("%v", err)
	}
	checkDeleteFn := func(i int, revision string) {
		if i == 3 {
			localProject := filepath.Join(localDir, localProjectName(i))
			if _, err := jirix.NewSeq().Stat(localProject); err == nil {
				t.Fatalf("project %v has not been deleted", localProject)
			} else {
				if !runutil.IsNotExist(err) {
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

	// Delete a project and create a new one with a different name but the same
	// path.  Check that UpdateUniverse() does not fail.
	deleteProject(t, jirix, remoteManifest, remoteProjects[4])
	createProject(t, jirix, remoteManifest, "new.project", remoteProjects[4], localProjectName(4))
	if err := project.UpdateUniverse(jirix, true); err != nil {
		t.Fatalf("%v", err)
	}

	// Commit to master and non-master branches of a remote project.
	nonMasterBranch := "non_master"
	writeReadme(t, jirix, remoteProjects[5], "master commit")
	createAndCheckoutBranch(t, jirix, remoteProjects[5], nonMasterBranch)
	writeReadme(t, jirix, remoteProjects[5], "non master commit")
	remoteBranchRevision := currentRevision(t, jirix, remoteProjects[5])

	// Set the revision to the non-master revision, but keep the remote branch
	// set to master.
	setRevisionForProject(t, jirix, remoteManifest, remoteProjects[5], remoteBranchRevision)
	// Check that UpdateUniverse() fails when updating to a revision that does
	// not occur on the remote branch (master).
	if err := project.UpdateUniverse(jirix, true); err == nil {
		t.Fatalf("expected project.UpdateUniverse() with revision that does not occur on remote branch to fail, but it did not")
	}

	// Set the project remote branch to the non-master branch with the revision.
	setRemoteBranchForProject(t, jirix, remoteManifest, remoteProjects[5], nonMasterBranch)
	// Check that UpdateUniverse() can update the local project to point to a
	// revision on the non-master remote branch.
	if err := project.UpdateUniverse(jirix, true); err != nil {
		t.Fatalf("%v", err)
	}
	localProject := filepath.Join(localDir, localProjectName(5))
	localBranchRevision := currentRevision(t, jirix, localProject)
	if localBranchRevision != remoteBranchRevision {
		t.Fatalf("project 5 is at revision %v, expected %v\n", localBranchRevision, remoteBranchRevision)
	}
	// Reset back to origin/master so the next update without a "revision" works.
	resetToOriginMaster(t, jirix, localProject)

	// Create a local manifest that imports the remote manifest
	// and check that UpdateUniverse() has no effect.
	createLocalManifestStub(t, jirix, localDir)
	if err := project.UpdateUniverse(jirix, true); err != nil {
		t.Fatalf("%v", err)
	}

	checkRebaseFn := func(i int, revision string) {
		if i == 5 {
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
	if err := project.UpdateUniverse(jirix, true); err != nil {
		t.Fatalf("%v", err)
	}
	for i, _ := range remoteProjects {
		checkRebaseFn(i, "revision 3")
	}
}

func TestFileImportCycle(t *testing.T) {
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()

	// Set up the cycle .jiri_manifest -> A -> B -> A
	jiriManifest := project.Manifest{
		LocalImports: []project.LocalImport{
			{File: "A"},
		},
	}
	manifestA := project.Manifest{
		LocalImports: []project.LocalImport{
			{File: "B"},
		},
	}
	manifestB := project.Manifest{
		LocalImports: []project.LocalImport{
			{File: "A"},
		},
	}
	if err := jiriManifest.ToFile(jirix, jirix.JiriManifestFile()); err != nil {
		t.Fatal(err)
	}
	if err := manifestA.ToFile(jirix, filepath.Join(jirix.Root, "A")); err != nil {
		t.Fatal(err)
	}
	if err := manifestB.ToFile(jirix, filepath.Join(jirix.Root, "B")); err != nil {
		t.Fatal(err)
	}

	// The update should complain about the cycle.
	err := project.UpdateUniverse(jirix, false)
	if got, want := fmt.Sprint(err), "import cycle detected in local manifest files"; !strings.Contains(got, want) {
		t.Errorf("got error %v, want substr %v", got, want)
	}
}

func TestRemoteImportCycle(t *testing.T) {
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()
	remoteDir := filepath.Join(jirix.Root, ".remote")

	// Set up two remote manifest projects, remote1 and remote2.
	remote1 := setupNewProject(t, jirix, remoteDir, "remote1", true)
	remote2 := setupNewProject(t, jirix, remoteDir, "remote2", true)
	fileA, fileB := filepath.Join(remote1, "A"), filepath.Join(remote2, "B")

	// Set up the cycle .jiri_manifest -> remote1+A -> remote2+B -> remote1+A
	jiriManifest := project.Manifest{
		Imports: []project.Import{
			{Manifest: "A", Name: "n1", Remote: remote1},
		},
	}
	manifestA := project.Manifest{
		Imports: []project.Import{
			{Manifest: "B", Name: "n2", Remote: remote2},
		},
	}
	manifestB := project.Manifest{
		Imports: []project.Import{
			{Manifest: "A", Name: "n3", Remote: remote1},
		},
	}
	if err := jiriManifest.ToFile(jirix, jirix.JiriManifestFile()); err != nil {
		t.Fatal(err)
	}
	if err := manifestA.ToFile(jirix, fileA); err != nil {
		t.Fatal(err)
	}
	if err := manifestB.ToFile(jirix, fileB); err != nil {
		t.Fatal(err)
	}
	commitFile(t, jirix, remote1, fileA, "commit A")
	commitFile(t, jirix, remote2, fileB, "commit B")

	// The update should complain about the cycle.
	err := project.UpdateUniverse(jirix, false)
	if got, want := fmt.Sprint(err), "import cycle detected in remote manifest imports"; !strings.Contains(got, want) {
		t.Errorf("got error %v, want substr %v", got, want)
	}
}

func TestFileAndRemoteImportCycle(t *testing.T) {
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()
	remoteDir := filepath.Join(jirix.Root, ".remote")

	// Set up two remote manifest projects, remote1 and remote2.
	remote1 := setupNewProject(t, jirix, remoteDir, "remote1", true)
	remote2 := setupNewProject(t, jirix, remoteDir, "remote2", true)
	fileA, fileD := filepath.Join(remote1, "A"), filepath.Join(remote1, "D")
	fileB, fileC := filepath.Join(remote2, "B"), filepath.Join(remote2, "C")

	// Set up the cycle .jiri_manifest -> remote1+A -> remote2+B -> C -> remote1+D -> A
	jiriManifest := project.Manifest{
		Imports: []project.Import{
			{Manifest: "A", Root: "r1", Name: "n1", Remote: remote1},
		},
	}
	manifestA := project.Manifest{
		Imports: []project.Import{
			{Manifest: "B", Root: "r2", Name: "n2", Remote: remote2},
		},
	}
	manifestB := project.Manifest{
		LocalImports: []project.LocalImport{
			{File: "C"},
		},
	}
	manifestC := project.Manifest{
		Imports: []project.Import{
			{Manifest: "D", Root: "r3", Name: "n3", Remote: remote1},
		},
	}
	manifestD := project.Manifest{
		LocalImports: []project.LocalImport{
			{File: "A"},
		},
	}
	if err := jiriManifest.ToFile(jirix, jirix.JiriManifestFile()); err != nil {
		t.Fatal(err)
	}
	if err := manifestA.ToFile(jirix, fileA); err != nil {
		t.Fatal(err)
	}
	if err := manifestB.ToFile(jirix, fileB); err != nil {
		t.Fatal(err)
	}
	if err := manifestC.ToFile(jirix, fileC); err != nil {
		t.Fatal(err)
	}
	if err := manifestD.ToFile(jirix, fileD); err != nil {
		t.Fatal(err)
	}
	commitFile(t, jirix, remote1, fileA, "commit A")
	commitFile(t, jirix, remote2, fileB, "commit B")
	commitFile(t, jirix, remote2, fileC, "commit C")
	commitFile(t, jirix, remote1, fileD, "commit D")

	// The update should complain about the cycle.
	err := project.UpdateUniverse(jirix, false)
	if got, want := fmt.Sprint(err), "import cycle detected"; !strings.Contains(got, want) {
		t.Errorf("got error %v, want substr %v", got, want)
	}
}

// TestUnsupportedProtocolErr checks that calling
// UnsupportedPrototoclErr.Error() does not result in an infinite loop.
func TestUnsupportedPrototocolErr(t *testing.T) {
	err := project.UnsupportedProtocolErr("foo")
	_ = err.Error()
}

type binDirTest struct {
	Name        string
	Setup       func(old, new string) error
	Teardown    func(old, new string) error
	Error       string
	CheckBackup bool
}

func TestTransitionBinDir(t *testing.T) {
	tests := []binDirTest{
		{
			"No old dir",
			func(old, new string) error { return nil },
			nil,
			"",
			false,
		},
		{
			"Empty old dir",
			func(old, new string) error {
				return os.MkdirAll(old, 0777)
			},
			nil,
			"",
			true,
		},
		{
			"Populated old dir",
			func(old, new string) error {
				if err := os.MkdirAll(old, 0777); err != nil {
					return err
				}
				return ioutil.WriteFile(filepath.Join(old, "tool"), []byte("foo"), 0777)
			},
			nil,
			"",
			true,
		},
		{
			"Symlinked old dir",
			func(old, new string) error {
				if err := os.MkdirAll(filepath.Dir(old), 0777); err != nil {
					return err
				}
				return os.Symlink(new, old)
			},
			nil,
			"",
			false,
		},
		{
			"Symlinked old dir pointing elsewhere",
			func(old, new string) error {
				if err := os.MkdirAll(filepath.Dir(old), 0777); err != nil {
					return err
				}
				return os.Symlink(filepath.Dir(new), old)
			},
			nil,
			"",
			true,
		},
		{
			"Unreadable old dir parent",
			func(old, new string) error {
				if err := os.MkdirAll(old, 0777); err != nil {
					return err
				}
				return os.Chmod(filepath.Dir(old), 0222)
			},
			func(old, new string) error {
				return os.Chmod(filepath.Dir(old), 0777)
			},
			"Failed to stat old bin dir",
			false,
		},
		{
			"Unwritable old dir",
			func(old, new string) error {
				if err := os.MkdirAll(old, 0777); err != nil {
					return err
				}
				return os.Chmod(old, 0444)
			},
			func(old, new string) error {
				return os.Chmod(old, 0777)
			},
			"Failed to backup old bin dir",
			false,
		},
		{
			"Unreadable backup dir parent",
			func(old, new string) error {
				if err := os.MkdirAll(old, 0777); err != nil {
					return err
				}
				return os.Chmod(filepath.Dir(new), 0222)
			},
			func(old, new string) error {
				return os.Chmod(filepath.Dir(new), 0777)
			},
			"Failed to stat backup bin dir",
			false,
		},
		{
			"Existing backup dir",
			func(old, new string) error {
				if err := os.MkdirAll(old, 0777); err != nil {
					return err
				}
				return os.MkdirAll(new+".BACKUP", 0777)
			},
			nil,
			"Backup bin dir",
			false,
		},
	}
	for _, test := range tests {
		jirix, cleanup := jiritest.NewX(t)
		if err := testTransitionBinDir(jirix, test); err != nil {
			t.Errorf("%s: %v", test.Name, err)
		}
		cleanup()
	}
}

func testTransitionBinDir(jirix *jiri.X, test binDirTest) (e error) {
	oldDir, newDir := filepath.Join(jirix.Root, "devtools", "bin"), jirix.BinDir()
	// The new bin dir always exists.
	if err := os.MkdirAll(newDir, 0777); err != nil {
		return fmt.Errorf("make new dir failed: %v", err)
	}
	if err := test.Setup(oldDir, newDir); err != nil {
		return fmt.Errorf("setup failed: %v", err)
	}
	if test.Teardown != nil {
		defer func() {
			if err := test.Teardown(oldDir, newDir); err != nil && e == nil {
				e = fmt.Errorf("teardown failed: %v", err)
			}
		}()
	}
	oldInfo, _ := os.Stat(oldDir)
	switch err := project.TransitionBinDir(jirix); {
	case err != nil && test.Error == "":
		return fmt.Errorf("got error %q, want success", err)
	case err != nil && !strings.Contains(fmt.Sprint(err), test.Error):
		return fmt.Errorf("got error %q, want prefix %q", err, test.Error)
	case err == nil && test.Error != "":
		return fmt.Errorf("got no error, want %q", test.Error)
	case err == nil && test.Error == "":
		// Make sure the symlink exists and is correctly linked.
		link, err := os.Readlink(oldDir)
		if err != nil {
			return fmt.Errorf("old dir isn't a symlink: %v", err)
		}
		if got, want := link, newDir; got != want {
			return fmt.Errorf("old dir symlink got %v, want %v", got, want)
		}
		if test.CheckBackup {
			// Make sure the oldDir was backed up correctly.
			backupDir := filepath.Join(jirix.RootMetaDir(), "bin.BACKUP")
			backupInfo, err := os.Stat(backupDir)
			if err != nil {
				return fmt.Errorf("stat backup dir failed: %v", err)
			}
			if !os.SameFile(oldInfo, backupInfo) {
				return fmt.Errorf("old dir wasn't backed up correctly")
			}
		}
	}
	return nil
}

func TestManifestToFromBytes(t *testing.T) {
	tests := []struct {
		Manifest project.Manifest
		XML      string
	}{
		{
			project.Manifest{},
			`<manifest>
</manifest>
`,
		},
		{
			project.Manifest{
				Imports: []project.Import{
					{
						Manifest:     "manifest1",
						Name:         "remoteimport1",
						Protocol:     "git",
						Remote:       "remote1",
						RemoteBranch: "master",
					},
					{
						Manifest:     "manifest2",
						Name:         "remoteimport2",
						Protocol:     "git",
						Remote:       "remote2",
						RemoteBranch: "branch2",
					},
					{
						Name: "oldimport",
					},
				},
				LocalImports: []project.LocalImport{
					{File: "fileimport"},
				},
				Projects: []project.Project{
					{
						Name:         "project1",
						Path:         "path1",
						Protocol:     "git",
						Remote:       "remote1",
						RemoteBranch: "master",
						Revision:     "HEAD",
						GerritHost:   "https://test-review.googlesource.com",
						GitHooks:     "path/to/githooks",
						RunHook:      "path/to/hook",
					},
					{
						Name:         "project2",
						Path:         "path2",
						Protocol:     "git",
						Remote:       "remote2",
						RemoteBranch: "branch2",
						Revision:     "rev2",
					},
				},
				Tools: []project.Tool{
					{
						Data:    "tooldata",
						Name:    "tool",
						Project: "toolproject",
					},
				},
			},
			`<manifest>
  <imports>
    <import manifest="manifest1" name="remoteimport1" remote="remote1"/>
    <import manifest="manifest2" name="remoteimport2" remote="remote2" remotebranch="branch2"/>
    <import name="oldimport"/>
    <localimport file="fileimport"/>
  </imports>
  <projects>
    <project name="project1" path="path1" remote="remote1" gerrithost="https://test-review.googlesource.com" githooks="path/to/githooks" runhook="path/to/hook"/>
    <project name="project2" path="path2" remote="remote2" remotebranch="branch2" revision="rev2"/>
  </projects>
  <tools>
    <tool data="tooldata" name="tool" project="toolproject"/>
  </tools>
</manifest>
`,
		},
	}
	for _, test := range tests {
		gotBytes, err := test.Manifest.ToBytes()
		if err != nil {
			t.Errorf("%+v ToBytes failed: %v", test.Manifest, err)
		}
		if got, want := string(gotBytes), test.XML; got != want {
			t.Errorf("%+v ToBytes GOT\n%v\nWANT\n%v", test.Manifest, got, want)
		}
		manifest, err := project.ManifestFromBytes([]byte(test.XML))
		if err != nil {
			t.Errorf("%+v FromBytes failed: %v", test.Manifest, err)
		}
		if got, want := manifest, &test.Manifest; !reflect.DeepEqual(got, want) {
			t.Errorf("%+v FromBytes got %#v, want %#v", test.Manifest, got, want)
		}
	}
}

func TestProjectToFromFile(t *testing.T) {
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()

	tests := []struct {
		Project project.Project
		XML     string
	}{
		{
			// Default fields are dropped when marshaled, and added when unmarshaled.
			project.Project{
				Name:         "project1",
				Path:         "path1",
				Protocol:     "git",
				Remote:       "remote1",
				RemoteBranch: "master",
				Revision:     "HEAD",
			},
			`<project name="project1" path="path1" remote="remote1"/>
`,
		},
		{
			project.Project{
				Name:         "project2",
				Path:         "path2",
				Protocol:     "git",
				Remote:       "remote2",
				RemoteBranch: "branch2",
				Revision:     "rev2",
			},
			`<project name="project2" path="path2" remote="remote2" remotebranch="branch2" revision="rev2"/>
`,
		},
	}
	for index, test := range tests {
		filename := filepath.Join(jirix.Root, fmt.Sprintf("test-%d", index))
		if err := test.Project.ToFile(jirix, filename); err != nil {
			t.Errorf("%+v ToFile failed: %v", test.Project, err)
		}
		gotBytes, err := jirix.NewSeq().ReadFile(filename)
		if err != nil {
			t.Errorf("%+v ReadFile failed: %v", test.Project, err)
		}
		if got, want := string(gotBytes), test.XML; got != want {
			t.Errorf("%+v ToFile GOT\n%v\nWANT\n%v", test.Project, got, want)
		}
		project, err := project.ProjectFromFile(jirix, filename)
		if err != nil {
			t.Errorf("%+v FromFile failed: %v", test.Project, err)
		}
		if got, want := project, &test.Project; !reflect.DeepEqual(got, want) {
			t.Errorf("%+v FromFile got %#v, want %#v", test.Project, got, want)
		}
	}
}

func TestProjectFromFileBackwardsCompatible(t *testing.T) {
	jirix, cleanup := jiritest.NewX(t)
	defer cleanup()

	tests := []struct {
		XML     string
		Project project.Project
	}{
		// Make sure <Project> opening tag is accepted.
		{
			`<Project name="project" path="path" remote="remote"/>`,
			project.Project{
				Name:         "project",
				Path:         "path",
				Protocol:     "git",
				Remote:       "remote",
				RemoteBranch: "master",
				Revision:     "HEAD",
			},
		},
		// Make sure <Project> opening and closing tags are accepted.
		{
			`<Project name="project" path="path" remote="remote"></Project>`,
			project.Project{
				Name:         "project",
				Path:         "path",
				Protocol:     "git",
				Remote:       "remote",
				RemoteBranch: "master",
				Revision:     "HEAD",
			},
		},
		// Make sure "this_attribute_should_be_ignored" is silently ignored.
		{
			`<Project this_attribute_should_be_ignored="junk" name="project" path="path" remote="remote" remotebranch="branch" revision="rev"></Project>`,
			project.Project{
				Name:         "project",
				Path:         "path",
				Protocol:     "git",
				Remote:       "remote",
				RemoteBranch: "branch",
				Revision:     "rev",
			},
		},
	}
	for index, test := range tests {
		filename := filepath.Join(jirix.Root, fmt.Sprintf("test-%d", index))
		if err := jirix.NewSeq().WriteFile(filename, []byte(test.XML), 0644).Done(); err != nil {
			t.Errorf("%+v WriteFile failed: %v", test.Project, err)
		}
		project, err := project.ProjectFromFile(jirix, filename)
		if err != nil {
			t.Errorf("%+v FromFile failed: %v", test.Project, err)
		}
		if got, want := project, &test.Project; !reflect.DeepEqual(got, want) {
			t.Errorf("%+v FromFile got %#v, want %#v", test.Project, got, want)
		}
	}
}
