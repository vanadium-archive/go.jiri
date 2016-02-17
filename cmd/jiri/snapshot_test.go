// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"v.io/jiri/gitutil"
	"v.io/jiri/jiri"
	"v.io/jiri/jiritest"
	"v.io/jiri/project"
	"v.io/jiri/tool"
)

func createLabelDir(t *testing.T, jirix *jiri.X, snapshotDir, name string, snapshots []string) {
	if snapshotDir == "" {
		snapshotDir = filepath.Join(jirix.Root, defaultSnapshotDir)
	}
	s := jirix.NewSeq()
	labelDir, perm := filepath.Join(snapshotDir, "labels", name), os.FileMode(0700)
	if err := s.MkdirAll(labelDir, perm).Done(); err != nil {
		t.Fatalf("MkdirAll(%v, %v) failed: %v", labelDir, perm, err)
	}
	for i, snapshot := range snapshots {
		path := filepath.Join(labelDir, snapshot)
		_, err := os.Create(path)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if i == 0 {
			symlinkPath := filepath.Join(snapshotDir, name)
			if err := s.Symlink(path, symlinkPath).Done(); err != nil {
				t.Fatalf("Symlink(%v, %v) failed: %v", path, symlinkPath, err)
			}
		}
	}
}

func generateOutput(labels []label) string {
	output := ""
	for _, label := range labels {
		output += fmt.Sprintf("snapshots of label %q:\n", label.name)
		for _, snapshot := range label.snapshots {
			output += fmt.Sprintf("  %v\n", snapshot)
		}
	}
	return output
}

type config struct {
	remote bool
	dir    string
}

type label struct {
	name      string
	snapshots []string
}

func TestList(t *testing.T) {
	resetFlags()
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	snapshotDir1 := "" // Should use default dir.
	snapshotDir2 := filepath.Join(fake.X.Root, "some/other/dir")

	// Create a test suite.
	tests := []config{
		config{
			dir: snapshotDir1,
		},
		config{
			dir: snapshotDir2,
		},
	}
	labels := []label{
		label{
			name:      "beta",
			snapshots: []string{"beta-1", "beta-2", "beta-3"},
		},
		label{
			name:      "stable",
			snapshots: []string{"stable-1", "stable-2", "stable-3"},
		},
	}

	for _, test := range tests {
		snapshotDirFlag = test.dir
		// Create the snapshots directory and populate it with the
		// data specified by the test suite.
		for _, label := range labels {
			createLabelDir(t, fake.X, test.dir, label.name, label.snapshots)
		}

		// Check that running "jiri snapshot list" with no arguments
		// returns the expected output.
		var stdout bytes.Buffer
		fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &stdout})
		if err := runSnapshotList(fake.X, nil); err != nil {
			t.Fatalf("%v", err)
		}
		got, want := stdout.String(), generateOutput(labels)
		if got != want {
			t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
		}

		// Check that running "jiri snapshot list" with one argument
		// returns the expected output.
		stdout.Reset()
		if err := runSnapshotList(fake.X, []string{"stable"}); err != nil {
			t.Fatalf("%v", err)
		}
		got, want = stdout.String(), generateOutput(labels[1:])
		if got != want {
			t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
		}

		// Check that running "jiri snapshot list" with
		// multiple arguments returns the expected output.
		stdout.Reset()
		if err := runSnapshotList(fake.X, []string{"beta", "stable"}); err != nil {
			t.Fatalf("%v", err)
		}
		got, want = stdout.String(), generateOutput(labels)
		if got != want {
			t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
		}
	}
}

func checkReadme(t *testing.T, jirix *jiri.X, project, message string) {
	s := jirix.NewSeq()
	if _, err := s.Stat(project); err != nil {
		t.Fatalf("%v", err)
	}
	readmeFile := filepath.Join(project, "README")
	data, err := s.ReadFile(readmeFile)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if got, want := data, []byte(message); bytes.Compare(got, want) != 0 {
		t.Fatalf("unexpected content %v:\ngot\n%s\nwant\n%s\n", project, got, want)
	}
}

func localProjectName(i int) string {
	return "test-local-project-" + fmt.Sprintf("%d", i+1)
}

func remoteProjectName(i int) string {
	return "test-remote-project-" + fmt.Sprintf("%d", i+1)
}

func writeReadme(t *testing.T, jirix *jiri.X, projectDir, message string) {
	s := jirix.NewSeq()
	path, perm := filepath.Join(projectDir, "README"), os.FileMode(0644)
	if err := s.WriteFile(path, []byte(message), perm).Done(); err != nil {
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
	if err := gitutil.New(jirix.NewSeq()).CommitFile(path, "creating README"); err != nil {
		t.Fatalf("%v", err)
	}
}

func resetFlags() {
	snapshotDirFlag = ""
	pushRemoteFlag = false
}

func TestGetSnapshotDir(t *testing.T) {
	resetFlags()
	defer resetFlags()
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	// With all flags at default values, snapshot dir should be default.
	resetFlags()
	got, err := getSnapshotDir(fake.X)
	if err != nil {
		t.Fatalf("getSnapshotDir() failed: %v\n", err)
	}
	if want := filepath.Join(fake.X.Root, defaultSnapshotDir); got != want {
		t.Errorf("unexpected snapshot dir: got %v want %v", got, want)
	}

	// With dir flag set to absolute path, snapshot dir should be value of dir
	// flag.
	resetFlags()
	tempDir, err := fake.X.NewSeq().TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer fake.X.NewSeq().RemoveAll(tempDir).Done()
	snapshotDirFlag = tempDir
	got, err = getSnapshotDir(fake.X)
	if err != nil {
		t.Fatalf("getSnapshotDir() failed: %v\n", err)
	}
	if want := snapshotDirFlag; got != want {
		t.Errorf("unexpected snapshot dir: got %v want %v", got, want)
	}

	// With dir flag set to relative path, snapshot dir should absolute path
	// rooted at current working dir.
	resetFlags()
	snapshotDirFlag = "some/relative/path"
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() failed: %v", err)
	}
	got, err = getSnapshotDir(fake.X)
	if err != nil {
		t.Fatalf("getSnapshotDir() failed: %v\n", err)
	}
	if want := filepath.Join(cwd, snapshotDirFlag); got != want {
		t.Errorf("unexpected snapshot dir: got %v want %v", got, want)
	}
}

// TestCreate tests creating and checking out a snapshot.
func TestCreate(t *testing.T) {
	resetFlags()
	defer resetFlags()
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()
	s := fake.X.NewSeq()

	// Setup the initial remote and local projects.
	numProjects, remoteProjects := 2, []string{}
	for i := 0; i < numProjects; i++ {
		if err := fake.CreateRemoteProject(remoteProjectName(i)); err != nil {
			t.Fatalf("%v", err)
		}
		if err := fake.AddProject(project.Project{
			Name:   remoteProjectName(i),
			Path:   localProjectName(i),
			Remote: fake.Projects[remoteProjectName(i)],
		}); err != nil {
			t.Fatalf("%v", err)
		}
	}

	// Create initial commits in the remote projects and use UpdateUniverse()
	// to mirror them locally.
	for i := 0; i < numProjects; i++ {
		writeReadme(t, fake.X, fake.Projects[remoteProjectName(i)], "revision 1")
	}
	if err := project.UpdateUniverse(fake.X, true); err != nil {
		t.Fatalf("%v", err)
	}

	// Create a snapshot.
	var stdout bytes.Buffer
	fake.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &stdout})
	if err := runSnapshotCreate(fake.X, []string{"test-local"}); err != nil {
		t.Fatalf("%v", err)
	}

	// Remove the local project repositories.
	for i, _ := range remoteProjects {
		localProject := filepath.Join(fake.X.Root, localProjectName(i))
		if err := s.RemoveAll(localProject).Done(); err != nil {
			t.Fatalf("%v", err)
		}
	}

	// Check that invoking the UpdateUniverse() with the snapshot restores the
	// local repositories.
	snapshotDir := filepath.Join(fake.X.Root, defaultSnapshotDir)
	snapshotFile := filepath.Join(snapshotDir, "test-local")
	localX := fake.X.Clone(tool.ContextOpts{
		Manifest: &snapshotFile,
	})
	if err := project.UpdateUniverse(localX, true); err != nil {
		t.Fatalf("%v", err)
	}
	for i, _ := range remoteProjects {
		localProject := filepath.Join(fake.X.Root, localProjectName(i))
		checkReadme(t, fake.X, localProject, "revision 1")
	}
}

// TestCreatePushRemote checks that creating a snapshot with the -push-remote
// flag causes the snapshot to be committed and pushed upstream.
func TestCreatePushRemote(t *testing.T) {
	resetFlags()
	defer resetFlags()

	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	fake.EnableRemoteManifestPush()
	defer fake.DisableRemoteManifestPush()

	manifestDir := filepath.Join(fake.X.Root, ".manifest")
	snapshotDir := filepath.Join(manifestDir, "snapshot")
	label := "test"

	git := gitutil.New(fake.X.NewSeq(), gitutil.RootDirOpt(manifestDir))
	commitCount, err := git.CountCommits("master", "")
	if err != nil {
		t.Fatalf("git.CountCommits(\"master\", \"\") failed: %v", err)
	}

	// Create snapshot with -push-remote flag set to true.
	snapshotDirFlag = snapshotDir
	pushRemoteFlag = true
	if err := runSnapshotCreate(fake.X, []string{label}); err != nil {
		t.Fatalf("%v", err)
	}

	// Check that repo has one new commit.
	newCommitCount, err := git.CountCommits("master", "")
	if err != nil {
		t.Fatalf("git.CountCommits(\"master\", \"\") failed: %v", err)
	}
	if got, want := newCommitCount, commitCount+1; got != want {
		t.Errorf("unexpected commit count: got %v want %v", got, want)
	}

	// Check that new label is commited.
	labelFile := filepath.Join(snapshotDir, "labels", label)
	if !git.IsFileCommitted(labelFile) {
		t.Errorf("expected file %v to be committed but it was not", labelFile)
	}
}
