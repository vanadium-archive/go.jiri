package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"v.io/lib/cmdline"
	"v.io/x/devtools/lib/util"
)

func createLabelDir(t *testing.T, ctx *util.Context, snapshotDir, name string, snapshots []string) {
	labelDir, perm := filepath.Join(snapshotDir, "labels", name), os.FileMode(0700)
	if err := ctx.Run().MkdirAll(labelDir, perm); err != nil {
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
			if err := ctx.Run().Symlink(path, symlinkPath); err != nil {
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
	ctx := util.DefaultContext()

	// Setup a fake VANADIUM_ROOT.
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer ctx.Run().RemoveAll(tmpDir)
	oldRoot, err := util.VanadiumRoot()
	if err := os.Setenv("VANADIUM_ROOT", tmpDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("VANADIUM_ROOT", oldRoot)

	remoteManifestDir, err := util.RemoteManifestDir()
	if err != nil {
		t.Fatalf("%v", err)
	}
	localSnapshotDir, err := util.LocalSnapshotDir()
	if err != nil {
		t.Fatalf("%v", err)
	}

	// Create a test suite.
	tests := []config{
		config{
			remote: false,
			dir:    localSnapshotDir,
		},
		config{
			remote: true,
			dir:    remoteManifestDir,
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
			createLabelDir(t, ctx, test.dir, label.name, label.snapshots)
		}

		// Check that running "v23 snapshot list" with no arguments
		// returns the expected output.
		var stdout bytes.Buffer
		command := cmdline.Command{}
		command.Init(nil, &stdout, nil)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if err := runSnapshotList(&command, nil); err != nil {
			t.Fatalf("%v", err)
		}
		got, want := stdout.String(), generateOutput(labels)
		if got != want {
			t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
		}

		// Check that running "v23 snapshot list" with one argument
		// returns the expected output.
		stdout.Reset()
		if err := runSnapshotList(&command, []string{"stable"}); err != nil {
			t.Fatalf("%v", err)
		}
		got, want = stdout.String(), generateOutput(labels[1:])
		if got != want {
			t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
		}

		// Check that running "v23 snapshot list" with
		// multiple arguments returns the expected output.
		stdout.Reset()
		if err := runSnapshotList(&command, []string{"beta", "stable"}); err != nil {
			t.Fatalf("%v", err)
		}
		got, want = stdout.String(), generateOutput(labels)
		if got != want {
			t.Fatalf("unexpected output:\ngot\n%v\nwant\n%v\n", got, want)
		}
	}
}

func addRemote(t *testing.T, ctx *util.Context, localProject, name, remoteProject string) {
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

func checkReadme(t *testing.T, ctx *util.Context, project, message string) {
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

func createRemoteManifest(t *testing.T, ctx *util.Context, dir string, remotes []string) {
	manifestDir, perm := filepath.Join(dir, "v2"), os.FileMode(0755)
	if err := ctx.Run().MkdirAll(manifestDir, perm); err != nil {
		t.Fatalf("%v", err)
	}
	manifest := util.Manifest{}
	for i, remote := range remotes {
		project := util.Project{
			Name:     remote,
			Path:     localProjectName(i),
			Protocol: "git",
			Remote:   remote,
		}
		manifest.Projects = append(manifest.Projects, project)
	}
	commitManifest(t, ctx, &manifest, dir)
}

func commitManifest(t *testing.T, ctx *util.Context, manifest *util.Manifest, manifestDir string) {
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

func localProjectName(i int) string {
	return "test-local-project-" + fmt.Sprintf("%d", i+1)
}

func remoteProjectName(i int) string {
	return "test-remote-project-" + fmt.Sprintf("%d", i+1)
}

func setupNewProject(t *testing.T, ctx *util.Context, dir, name string) string {
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

func writeReadme(t *testing.T, ctx *util.Context, projectDir, message string) {
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

func TestCreate(t *testing.T) {
	// Setup an instance of vanadium universe, creating the remote
	// repositories for the manifest and projects under the
	// "remote" directory, which is ignored from the consideration
	// of LocalProjects().
	ctx := util.DefaultContext()
	rootDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		t.Fatalf("TempDir() failed: %v", err)
	}
	defer ctx.Run().RemoveAll(rootDir)
	localDir := filepath.Join(rootDir, "local")
	remoteDir := filepath.Join(rootDir, "remote")
	oldRoot := os.Getenv("VANADIUM_ROOT")
	if err := os.Setenv("VANADIUM_ROOT", localDir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("VANADIUM_ROOT", oldRoot)
	localManifest := setupNewProject(t, ctx, localDir, ".manifest")
	remoteManifest := setupNewProject(t, ctx, remoteDir, "test-remote-manifest")
	addRemote(t, ctx, localManifest, "origin", remoteManifest)
	numProjects, remoteProjects := 2, []string{}
	for i := 0; i < numProjects; i++ {
		remoteProject := setupNewProject(t, ctx, remoteDir, remoteProjectName(i))
		remoteProjects = append(remoteProjects, remoteProject)
	}
	createRemoteManifest(t, ctx, remoteManifest, remoteProjects)
	config := util.NewConfig(util.SnapshotLabelTestsOpt(map[string][]string{"remote-snapshot": []string{}}))
	createConfig(t, ctx, config)

	// Create initial commits in the remote projects and use
	// UpdateUniverse() to mirror them locally.
	for _, remoteProject := range remoteProjects {
		writeReadme(t, ctx, remoteProject, "revision 1")
	}
	if err := util.UpdateUniverse(ctx, "default", true); err != nil {
		t.Fatalf("%v", err)
	}

	// Change the branch of the remote manifest repository away
	// from the "master" branch, so that we can push changes to it
	// from the local manifest repository in the course of
	// creating a remote snapshot.
	if err := ctx.Run().Chdir(remoteManifest); err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Git().CreateAndCheckoutBranch("non-master"); err != nil {
		t.Fatalf("%v", err)

	}

	// Create a local snapshot.
	command := cmdline.Command{}
	command.Init(nil, nil, nil)
	remoteFlag = false
	if err := runSnapshotCreate(&command, []string{"local-snapshot"}); err != nil {
		t.Fatalf("%v", err)
	}

	// Remove the local project repositories.
	for i, _ := range remoteProjects {
		localProject := filepath.Join(localDir, localProjectName(i))
		if err := ctx.Run().RemoveAll(localProject); err != nil {
			t.Fatalf("%v", err)
		}
	}

	// Check that invoking the UpdateUniverse() with the local
	// snapshot restores the local repositories.
	snapshotDir, err := util.LocalSnapshotDir()
	if err != nil {
		t.Fatalf("%v", err)
	}
	snapshotFile := filepath.Join(snapshotDir, "local-snapshot")
	if err := util.UpdateUniverse(ctx, snapshotFile, true); err != nil {
		t.Fatalf("%v", err)
	}
	for i, _ := range remoteProjects {
		localProject := filepath.Join(localDir, localProjectName(i))
		checkReadme(t, ctx, localProject, "revision 1")
	}

	// Create a remote snapshot.
	remoteFlag = true
	if err := runSnapshotCreate(&command, []string{"remote-snapshot"}); err != nil {
		t.Fatalf("%v", err)
	}

	// Remove the local project repositories.
	for i, _ := range remoteProjects {
		localProject := filepath.Join(localDir, localProjectName(i))
		if err := ctx.Run().RemoveAll(localProject); err != nil {
			t.Fatalf("%v", err)
		}
	}

	// Check that invoking the UpdateUniverse() with the remote snapshot.
	if err := util.UpdateUniverse(ctx, "remote-snapshot", true); err != nil {
		t.Fatalf("%v", err)
	}
	for i, _ := range remoteProjects {
		localProject := filepath.Join(localDir, localProjectName(i))
		checkReadme(t, ctx, localProject, "revision 1")
	}
}
