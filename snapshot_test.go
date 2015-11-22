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

	"v.io/jiri/jiri"
	"v.io/jiri/jiritest"
	"v.io/jiri/project"
	"v.io/jiri/tool"
)

func createLabelDir(t *testing.T, jirix *jiri.X, snapshotDir, name string, snapshots []string) {
	labelDir, perm := filepath.Join(snapshotDir, "labels", name), os.FileMode(0700)
	if err := jirix.Run().MkdirAll(labelDir, perm); err != nil {
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
			if err := jirix.Run().Symlink(path, symlinkPath); err != nil {
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
	// Setup a fake JIRI_ROOT.
	root, err := jiritest.NewFakeJiriRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(); err != nil {
			t.Fatalf("%v", err)
		}
	}()

	remoteSnapshotDir := root.X.RemoteSnapshotDir()
	localSnapshotDir := root.X.LocalSnapshotDir()

	// Create a test suite.
	tests := []config{
		config{
			remote: false,
			dir:    localSnapshotDir,
		},
		config{
			remote: true,
			dir:    remoteSnapshotDir,
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
		remoteFlag = test.remote
		// Create the snapshots directory and populate it with the
		// data specified by the test suite.
		for _, label := range labels {
			createLabelDir(t, root.X, test.dir, label.name, label.snapshots)
		}

		// Check that running "jiri snapshot list" with no arguments
		// returns the expected output.
		var stdout bytes.Buffer
		root.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &stdout})
		if err := runSnapshotList(root.X, nil); err != nil {
			t.Fatalf("%v", err)
		}
		got, want := stdout.String(), generateOutput(labels)
		if got != want {
			t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
		}

		// Check that running "jiri snapshot list" with one argument
		// returns the expected output.
		stdout.Reset()
		if err := runSnapshotList(root.X, []string{"stable"}); err != nil {
			t.Fatalf("%v", err)
		}
		got, want = stdout.String(), generateOutput(labels[1:])
		if got != want {
			t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
		}

		// Check that running "jiri snapshot list" with
		// multiple arguments returns the expected output.
		stdout.Reset()
		if err := runSnapshotList(root.X, []string{"beta", "stable"}); err != nil {
			t.Fatalf("%v", err)
		}
		got, want = stdout.String(), generateOutput(labels)
		if got != want {
			t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
		}
	}
}

func checkReadme(t *testing.T, jirix *jiri.X, project, message string) {
	if _, err := jirix.Run().Stat(project); err != nil {
		t.Fatalf("%v", err)
	}
	readmeFile := filepath.Join(project, "README")
	data, err := jirix.Run().ReadFile(readmeFile)
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
	path, perm := filepath.Join(projectDir, "README"), os.FileMode(0644)
	if err := jirix.Run().WriteFile(path, []byte(message), perm); err != nil {
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
	if err := jirix.Git().CommitFile(path, "creating README"); err != nil {
		t.Fatalf("%v", err)
	}
}

func TestCreate(t *testing.T) {
	// Setup a fake JIRI_ROOT instance.
	root, err := jiritest.NewFakeJiriRoot()
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(); err != nil {
			t.Fatalf("%v", err)
		}
	}()

	// Setup the initial remote and local projects.
	numProjects, remoteProjects := 2, []string{}
	for i := 0; i < numProjects; i++ {
		if err := root.CreateRemoteProject(remoteProjectName(i)); err != nil {
			t.Fatalf("%v", err)
		}
		if err := root.AddProject(project.Project{
			Name:   remoteProjectName(i),
			Path:   localProjectName(i),
			Remote: root.Projects[remoteProjectName(i)],
		}); err != nil {
			t.Fatalf("%v", err)
		}
	}

	// Create initial commits in the remote projects and use
	// UpdateUniverse() to mirror them locally.
	for i := 0; i < numProjects; i++ {
		writeReadme(t, root.X, root.Projects[remoteProjectName(i)], "revision 1")
	}
	if err := project.UpdateUniverse(root.X, true); err != nil {
		t.Fatalf("%v", err)
	}

	// Create a local snapshot.
	var stdout bytes.Buffer
	root.X.Context = tool.NewContext(tool.ContextOpts{Stdout: &stdout})
	remoteFlag = false
	if err := runSnapshotCreate(root.X, []string{"test-local"}); err != nil {
		t.Fatalf("%v", err)
	}

	// Remove the local project repositories.
	for i, _ := range remoteProjects {
		localProject := filepath.Join(root.Dir, localProjectName(i))
		if err := root.X.Run().RemoveAll(localProject); err != nil {
			t.Fatalf("%v", err)
		}
	}

	// Check that invoking the UpdateUniverse() with the local
	// snapshot restores the local repositories.
	snapshotDir := root.X.LocalSnapshotDir()
	snapshotFile := filepath.Join(snapshotDir, "test-local")
	localX := root.X.Clone(tool.ContextOpts{
		Manifest: &snapshotFile,
	})
	if err := project.UpdateUniverse(localX, true); err != nil {
		t.Fatalf("%v", err)
	}
	for i, _ := range remoteProjects {
		localProject := filepath.Join(root.Dir, localProjectName(i))
		checkReadme(t, root.X, localProject, "revision 1")
	}

	// Create a remote snapshot.
	remoteFlag = true
	root.EnableRemoteManifestPush()
	if err := runSnapshotCreate(root.X, []string{"test-remote"}); err != nil {
		t.Fatalf("%v", err)
	}

	// Remove the local project repositories.
	for i, _ := range remoteProjects {
		localProject := filepath.Join(root.Dir, localProjectName(i))
		if err := root.X.Run().RemoveAll(localProject); err != nil {
			t.Fatalf("%v", err)
		}
	}

	// Check that invoking the UpdateUniverse() with the remote snapshot
	// restores the local repositories.
	manifest := "snapshot/test-remote"
	remoteX := root.X.Clone(tool.ContextOpts{
		Manifest: &manifest,
	})
	if err := project.UpdateUniverse(remoteX, true); err != nil {
		t.Fatalf("%v", err)
	}
	for i, _ := range remoteProjects {
		localProject := filepath.Join(root.Dir, localProjectName(i))
		checkReadme(t, root.X, localProject, "revision 1")
	}
}
