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

	"v.io/jiri"
	"v.io/jiri/gitutil"
	"v.io/jiri/jiritest"
	"v.io/jiri/project"
)

func checkReadme(t *testing.T, jirix *jiri.X, p project.Project, message string) {
	if _, err := jirix.NewSeq().Stat(p.Path); err != nil {
		t.Fatalf("%v", err)
	}
	readmeFile := filepath.Join(p.Path, "README")
	data, err := ioutil.ReadFile(readmeFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", readmeFile, err)
	}
	if got, want := data, []byte(message); bytes.Compare(got, want) != 0 {
		t.Fatalf("unexpected content in project %v:\ngot\n%s\nwant\n%s\n", p.Name, got, want)
	}
}

// Checks that /.jiri/ is ignored in a local project checkout
func checkMetadataIsIgnored(t *testing.T, jirix *jiri.X, p project.Project) {
	if _, err := jirix.NewSeq().Stat(p.Path); err != nil {
		t.Fatalf("%v", err)
	}
	gitInfoExcludeFile := filepath.Join(p.Path, ".git", "info", "exclude")
	data, err := ioutil.ReadFile(gitInfoExcludeFile)
	if err != nil {
		t.Fatalf("ReadFile(%v) failed: %v", gitInfoExcludeFile, err)
	}
	excludeString := "/.jiri/"
	if !strings.Contains(string(data), excludeString) {
		t.Fatalf("Did not find \"%v\" in exclude file", excludeString)
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

func projectName(i int) string {
	return fmt.Sprintf("project-%d", i)
}

func writeReadme(t *testing.T, jirix *jiri.X, projectDir, message string) {
	path, perm := filepath.Join(projectDir, "README"), os.FileMode(0644)
	if err := ioutil.WriteFile(path, []byte(message), perm); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", path, perm, err)
	}
	commitFile(t, jirix, projectDir, path, "creating README")
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
		s := jirix.NewSeq()
		name := projectName(i)
		path := filepath.Join(jirix.Root, name)
		if err := s.MkdirAll(path, 0755).Done(); err != nil {
			t.Fatal(err)
		}

		// Initialize empty git repository.  The commit is necessary, otherwise
		// "git rev-parse master" fails.
		git := gitutil.New(s, gitutil.RootDirOpt(path))
		if err := git.Init(path); err != nil {
			t.Fatal(err)
		}
		if err := git.Commit(); err != nil {
			t.Fatal(err)
		}

		// Write project metadata.
		p := project.Project{
			Path: path,
			Name: name,
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
				Name: projectName(0),
				Path: projectPaths[0],
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

// setupUniverse creates a fake jiri root with 3 remote projects.  Each project
// has a README with text "initial readme".
func setupUniverse(t *testing.T) ([]project.Project, *jiritest.FakeJiriRoot, func()) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	success := false
	defer func() {
		if !success {
			cleanup()
		}
	}()

	// Create some projects and add them to the remote manifest.
	numProjects := 3
	localProjects := []project.Project{}
	for i := 0; i < numProjects; i++ {
		name := projectName(i)
		path := fmt.Sprintf("path-%d", i)
		if err := fake.CreateRemoteProject(name); err != nil {
			t.Fatal(err)
		}
		p := project.Project{
			Name:   name,
			Path:   filepath.Join(fake.X.Root, path),
			Remote: fake.Projects[name],
		}
		localProjects = append(localProjects, p)
		if err := fake.AddProject(p); err != nil {
			t.Fatal(err)
		}
	}

	// Create initial commit in each repo.
	for _, remoteProjectDir := range fake.Projects {
		writeReadme(t, fake.X, remoteProjectDir, "initial readme")
	}

	success = true
	return localProjects, fake, cleanup
}

// TestUpdateUniverseSimple tests that UpdateUniverse will pull remote projects
// locally, and that jiri metadata is ignored in the repos.
func TestUpdateUniverseSimple(t *testing.T) {
	localProjects, fake, cleanup := setupUniverse(t)
	defer cleanup()
	s := fake.X.NewSeq()

	// Check that calling UpdateUniverse() creates local copies of the remote
	// repositories, and that jiri metadata is ignored by git.
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}
	for _, p := range localProjects {
		if err := s.AssertDirExists(p.Path).Done(); err != nil {
			t.Fatalf("expected project to exist at path %q but none found", p.Path)
		}
		checkReadme(t, fake.X, p, "initial readme")
		checkMetadataIsIgnored(t, fake.X, p)
	}
}

// TestUpdateUniverseWithRevision checks that UpdateUniverse will pull remote
// projects at the specified revision.
func TestUpdateUniverseWithRevision(t *testing.T) {
	localProjects, fake, cleanup := setupUniverse(t)
	defer cleanup()
	s := fake.X.NewSeq()

	// Set project 1's revision in the manifest to the current revision.
	git := gitutil.New(s, gitutil.RootDirOpt(fake.Projects[localProjects[1].Name]))
	rev, err := git.CurrentRevision()
	if err != nil {
		t.Fatal(err)
	}
	m, err := fake.ReadRemoteManifest()
	if err != nil {
		t.Fatal(err)
	}
	projects := []project.Project{}
	for _, p := range m.Projects {
		if p.Name == localProjects[1].Name {
			p.Revision = rev
		}
		projects = append(projects, p)
	}
	m.Projects = projects
	if err := fake.WriteRemoteManifest(m); err != nil {
		t.Fatal(err)
	}
	// Update README in all projects.
	for _, remoteProjectDir := range fake.Projects {
		writeReadme(t, fake.X, remoteProjectDir, "new revision")
	}
	// Check that calling UpdateUniverse() updates all projects except for
	// project 1.
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}
	for i, p := range localProjects {
		if i == 1 {
			checkReadme(t, fake.X, p, "initial readme")
		} else {
			checkReadme(t, fake.X, p, "new revision")
		}
	}
}

// TestUpdateUniverseWithUncommitted checks that uncommitted files are not droped
// by UpdateUniverse(). This ensures that the "git reset --hard" mechanism used
// for pointing the master branch to a fixed revision does not lose work in
// progress.
func TestUpdateUniverseWithUncommitted(t *testing.T) {
	localProjects, fake, cleanup := setupUniverse(t)
	defer cleanup()
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}

	// Create an uncommitted file in project 1.
	file, perm, want := filepath.Join(localProjects[1].Path, "uncommitted_file"), os.FileMode(0644), []byte("uncommitted work")
	if err := ioutil.WriteFile(file, want, perm); err != nil {
		t.Fatalf("WriteFile(%v, %v) failed: %v", file, err, perm)
	}
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}
	got, err := ioutil.ReadFile(file)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if bytes.Compare(got, want) != 0 {
		t.Fatalf("unexpected content %v:\ngot\n%s\nwant\n%s\n", localProjects[1], got, want)
	}
}

// TestUpdateUniverseMovedProject checks that UpdateUniverse can move a
// project.
func TestUpdateUniverseMovedProject(t *testing.T) {
	localProjects, fake, cleanup := setupUniverse(t)
	defer cleanup()
	s := fake.X.NewSeq()
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}

	// Update the local path at which project 1 is located.
	m, err := fake.ReadRemoteManifest()
	if err != nil {
		t.Fatal(err)
	}
	oldProjectPath := localProjects[1].Path
	localProjects[1].Path = filepath.Join(fake.X.Root, "new-project-path")
	projects := []project.Project{}
	for _, p := range m.Projects {
		if p.Name == localProjects[1].Name {
			p.Path = localProjects[1].Path
		}
		projects = append(projects, p)
	}
	m.Projects = projects
	if err := fake.WriteRemoteManifest(m); err != nil {
		t.Fatal(err)
	}
	// Check that UpdateUniverse() moves the local copy of the project 1.
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}
	if err := s.AssertDirExists(oldProjectPath).Done(); err == nil {
		t.Fatalf("expected project %q at path %q not to exist but it did", localProjects[1].Name, oldProjectPath)
	}
	if err := s.AssertDirExists(localProjects[2].Path).Done(); err != nil {
		t.Fatalf("expected project %q at path %q to exist but it did not", localProjects[1].Name, localProjects[1].Path)
	}
	checkReadme(t, fake.X, localProjects[1], "initial readme")
}

// TestUpdateUniverseDeletedProject checks that UpdateUniverse will delete a
// project iff gc=true.
func TestUpdateUniverseDeletedProject(t *testing.T) {
	localProjects, fake, cleanup := setupUniverse(t)
	defer cleanup()
	s := fake.X.NewSeq()
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}

	// Delete project 1.
	m, err := fake.ReadRemoteManifest()
	if err != nil {
		t.Fatal(err)
	}
	projects := []project.Project{}
	for _, p := range m.Projects {
		if p.Name == localProjects[1].Name {
			continue
		}
		projects = append(projects, p)
	}
	m.Projects = projects
	if err := fake.WriteRemoteManifest(m); err != nil {
		t.Fatal(err)
	}
	// Check that UpdateUniverse() with gc=false does not delete the local copy
	// of the project.
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}
	if err := s.AssertDirExists(localProjects[1].Path).Done(); err != nil {
		t.Fatalf("expected project %q at path %q to exist but it did not", localProjects[1].Name, localProjects[1].Path)
	}
	checkReadme(t, fake.X, localProjects[1], "initial readme")
	// Check that UpdateUniverse() with gc=true does delete the local copy of
	// the project.
	if err := fake.UpdateUniverse(true); err != nil {
		t.Fatal(err)
	}
	if err := s.AssertDirExists(localProjects[1].Path).Done(); err == nil {
		t.Fatalf("expected project %q at path %q not to exist but it did", localProjects[1].Name, localProjects[3].Path)
	}
}

// TestUpdateUniverseNewProjectSamePath checks that UpdateUniverse can handle a
// new project with the same path as a deleted project, but a different path.
func TestUpdateUniverseNewProjectSamePath(t *testing.T) {
	localProjects, fake, cleanup := setupUniverse(t)
	defer cleanup()
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}

	// Delete a project 1 and create a new one with a different name but the
	// same path.
	m, err := fake.ReadRemoteManifest()
	if err != nil {
		t.Fatal(err)
	}
	newProjectName := "new-project-name"
	projects := []project.Project{}
	for _, p := range m.Projects {
		if p.Path == localProjects[1].Path {
			p.Name = newProjectName
		}
		projects = append(projects, p)
	}
	localProjects[1].Name = newProjectName
	m.Projects = projects
	if err := fake.WriteRemoteManifest(m); err != nil {
		t.Fatal(err)
	}
	// Check that UpdateUniverse() does not fail.
	if err := fake.UpdateUniverse(true); err != nil {
		t.Fatal(err)
	}
}

// TestUpdateUniverseRemoteBranch checks that UpdateUniverse can pull from a
// non-master remote branch.
func TestUpdateUniverseRemoteBranch(t *testing.T) {
	localProjects, fake, cleanup := setupUniverse(t)
	defer cleanup()
	s := fake.X.NewSeq()
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}

	// Commit to master branch of a project 1.
	writeReadme(t, fake.X, fake.Projects[localProjects[1].Name], "master commit")
	// Create and checkout a new branch of project 1 and make a new commit.
	git := gitutil.New(s, gitutil.RootDirOpt(fake.Projects[localProjects[1].Name]))
	if err := git.CreateAndCheckoutBranch("non-master"); err != nil {
		t.Fatal(err)
	}
	writeReadme(t, fake.X, fake.Projects[localProjects[1].Name], "non-master commit")
	// Point the manifest to the new non-master branch.
	m, err := fake.ReadRemoteManifest()
	if err != nil {
		t.Fatal(err)
	}
	projects := []project.Project{}
	for _, p := range m.Projects {
		if p.Name == localProjects[1].Name {
			p.RemoteBranch = "non-master"
		}
		projects = append(projects, p)
	}
	m.Projects = projects
	if err := fake.WriteRemoteManifest(m); err != nil {
		t.Fatal(err)
	}
	// Check that UpdateUniverse pulls the commit from the non-master branch.
	if err := fake.UpdateUniverse(false); err != nil {
		t.Fatal(err)
	}
	checkReadme(t, fake.X, localProjects[1], "non-master commit")
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
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	// Set up two remote manifest projects, remote1 and remote1.
	if err := fake.CreateRemoteProject("remote1"); err != nil {
		t.Fatal(err)
	}
	if err := fake.CreateRemoteProject("remote2"); err != nil {
		t.Fatal(err)
	}
	remote1 := fake.Projects["remote1"]
	remote2 := fake.Projects["remote2"]

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
	if err := jiriManifest.ToFile(fake.X, fake.X.JiriManifestFile()); err != nil {
		t.Fatal(err)
	}
	if err := manifestA.ToFile(fake.X, fileA); err != nil {
		t.Fatal(err)
	}
	if err := manifestB.ToFile(fake.X, fileB); err != nil {
		t.Fatal(err)
	}
	commitFile(t, fake.X, remote1, fileA, "commit A")
	commitFile(t, fake.X, remote2, fileB, "commit B")

	// The update should complain about the cycle.
	err := project.UpdateUniverse(fake.X, false)
	if got, want := fmt.Sprint(err), "import cycle detected in remote manifest imports"; !strings.Contains(got, want) {
		t.Errorf("got error %v, want substr %v", got, want)
	}
}

func TestFileAndRemoteImportCycle(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	// Set up two remote manifest projects, remote1 and remote2.
	// Set up two remote manifest projects, remote1 and remote1.
	if err := fake.CreateRemoteProject("remote1"); err != nil {
		t.Fatal(err)
	}
	if err := fake.CreateRemoteProject("remote2"); err != nil {
		t.Fatal(err)
	}
	remote1 := fake.Projects["remote1"]
	remote2 := fake.Projects["remote2"]
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
	if err := jiriManifest.ToFile(fake.X, fake.X.JiriManifestFile()); err != nil {
		t.Fatal(err)
	}
	if err := manifestA.ToFile(fake.X, fileA); err != nil {
		t.Fatal(err)
	}
	if err := manifestB.ToFile(fake.X, fileB); err != nil {
		t.Fatal(err)
	}
	if err := manifestC.ToFile(fake.X, fileC); err != nil {
		t.Fatal(err)
	}
	if err := manifestD.ToFile(fake.X, fileD); err != nil {
		t.Fatal(err)
	}
	commitFile(t, fake.X, remote1, fileA, "commit A")
	commitFile(t, fake.X, remote2, fileB, "commit B")
	commitFile(t, fake.X, remote2, fileC, "commit C")
	commitFile(t, fake.X, remote1, fileD, "commit D")

	// The update should complain about the cycle.
	err := project.UpdateUniverse(fake.X, false)
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
				Path:         filepath.Join(jirix.Root, "path1"),
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
				Path:         filepath.Join(jirix.Root, "path2"),
				GitHooks:     filepath.Join(jirix.Root, "git-hooks"),
				RunHook:      filepath.Join(jirix.Root, "run-hook"),
				Protocol:     "git",
				Remote:       "remote2",
				RemoteBranch: "branch2",
				Revision:     "rev2",
			},
			`<project name="project2" path="path2" remote="remote2" remotebranch="branch2" revision="rev2" githooks="git-hooks" runhook="run-hook"/>
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
