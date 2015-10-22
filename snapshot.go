// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"v.io/jiri/collect"
	"v.io/jiri/gitutil"
	"v.io/jiri/project"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
)

var (
	remoteFlag     bool
	timeFormatFlag string
)

func init() {
	cmdSnapshot.Flags.BoolVar(&remoteFlag, "remote", false, "Manage remote snapshots.")
	cmdSnapshotCreate.Flags.StringVar(&timeFormatFlag, "time-format", time.RFC3339, "Time format for snapshot file name.")
}

var cmdSnapshot = &cmdline.Command{
	Name:  "snapshot",
	Short: "Manage project snapshots",
	Long: `
The "jiri snapshot" command can be used to manage project snapshots.
In particular, it can be used to create new snapshots and to list
existing snapshots.

The command-line flag "-remote" determines whether the command
pertains to "local" snapshots that are only stored locally or "remote"
snapshots the are revisioned in the manifest repository.
`,
	Children: []*cmdline.Command{cmdSnapshotCreate, cmdSnapshotList},
}

// cmdSnapshotCreate represents the "jiri snapshot create" command.
var cmdSnapshotCreate = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runSnapshotCreate),
	Name:   "create",
	Short:  "Create a new project snapshot",
	Long: `
The "jiri snapshot create <label>" command captures the current project
state in a manifest and, depending on the value of the -remote flag,
the command either stores the manifest in the local
$JIRI_ROOT/.snapshots directory, or in the manifest repository, pushing
the change to the remote repository and thus making it available
globally.

Internally, snapshots are organized as follows:

 <snapshot-dir>/
   labels/
     <label1>/
       <label1-snapshot1>
       <label1-snapshot2>
       ...
     <label2>/
       <label2-snapshot1>
       <label2-snapshot2>
       ...
     <label3>/
     ...
   <label1> # a symlink to the latest <label1-snapshot*>
   <label2> # a symlink to the latest <label2-snapshot*>
   ...

NOTE: Unlike the jiri tool commands, the above internal organization
is not an API. It is an implementation and can change without notice.
`,
	ArgsName: "<label>",
	ArgsLong: "<label> is the snapshot label.",
}

func runSnapshotCreate(env *cmdline.Env, args []string) error {
	if len(args) != 1 {
		return env.UsageErrorf("unexpected number of arguments")
	}
	label := args[0]
	ctx := tool.NewContextFromEnv(env)
	if err := checkSnapshotDir(ctx); err != nil {
		return err
	}
	snapshotDir, err := getSnapshotDir()
	if err != nil {
		return err
	}
	snapshotFile := filepath.Join(snapshotDir, "labels", label, time.Now().Format(timeFormatFlag))
	// Either atomically create a new snapshot that captures the project
	// state and push the changes to the remote repository (if
	// applicable), or fail with no effect.
	createFn := func() error {
		revision, err := ctx.Git().CurrentRevision()
		if err != nil {
			return err
		}
		if err := createSnapshot(ctx, snapshotDir, snapshotFile, label); err != nil {
			// Clean up on all errors.
			ctx.Git().Reset(revision)
			ctx.Git().RemoveUntrackedFiles()
			return err
		}
		return nil
	}

	// Execute the above function in the snapshot directory.
	p := project.Project{
		Path:     snapshotDir,
		Protocol: "git",
		Revision: "HEAD",
	}
	if err := project.ApplyToLocalMaster(ctx, project.Projects{p.Name: p}, createFn); err != nil {
		return err
	}
	return nil
}

// checkSnapshotDir makes sure that he local snapshot directory exists
// and is initialized properly.
func checkSnapshotDir(ctx *tool.Context) (e error) {
	snapshotDir, err := getSnapshotDir()
	if err != nil {
		return err
	}
	if _, err := ctx.Run().Stat(snapshotDir); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if remoteFlag {
			if err := ctx.Run().MkdirAll(snapshotDir, 0755); err != nil {
				return err
			}
			return nil
		}
		createFn := func() (err error) {
			if err := ctx.Run().MkdirAll(snapshotDir, 0755); err != nil {
				return err
			}
			if err := ctx.Git().Init(snapshotDir); err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
			if err := ctx.Run().Chdir(snapshotDir); err != nil {
				return err
			}
			if err := ctx.Git().Commit(); err != nil {
				return err
			}
			return nil
		}
		if err := createFn(); err != nil {
			ctx.Run().RemoveAll(snapshotDir)
			return err
		}
	}
	return nil
}

func createSnapshot(ctx *tool.Context, snapshotDir, snapshotFile, label string) error {
	// Create a snapshot that encodes the current state of master
	// branches for all local projects.
	if err := project.CreateSnapshot(ctx, snapshotFile); err != nil {
		return err
	}

	// Update the symlink for this snapshot label to point to the
	// latest snapshot.
	symlink := filepath.Join(snapshotDir, label)
	newSymlink := symlink + ".new"
	if err := ctx.Run().RemoveAll(newSymlink); err != nil {
		return err
	}
	relativeSnapshotPath := strings.TrimPrefix(snapshotFile, snapshotDir+string(os.PathSeparator))
	if err := ctx.Run().Symlink(relativeSnapshotPath, newSymlink); err != nil {
		return err
	}
	if err := ctx.Run().Rename(newSymlink, symlink); err != nil {
		return err
	}

	// Revision the changes.
	if err := revisionChanges(ctx, snapshotDir, snapshotFile, label); err != nil {
		return err
	}
	return nil
}

// getSnapshotDir returns the path to the snapshot directory,
// respecting the value of the "-remote" command-line flag.
func getSnapshotDir() (string, error) {
	if remoteFlag {
		snapshotDir, err := project.RemoteSnapshotDir()
		if err != nil {
			return "", err
		}
		return snapshotDir, nil
	}
	snapshotDir, err := project.LocalSnapshotDir()
	if err != nil {
		return "", err
	}
	return snapshotDir, nil
}

// revisionChanges commits changes identified by the given manifest
// file and label to the manifest repository and (if applicable)
// pushes these changes to the remote repository.
func revisionChanges(ctx *tool.Context, snapshotDir, snapshotFile, label string) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return ctx.Run().Chdir(cwd) }, &e)
	if err := ctx.Run().Chdir(snapshotDir); err != nil {
		return err
	}
	relativeSnapshotPath := strings.TrimPrefix(snapshotFile, snapshotDir+string(os.PathSeparator))
	if err := ctx.Git().Add(relativeSnapshotPath); err != nil {
		return err
	}
	if err := ctx.Git().Add(label); err != nil {
		return err
	}
	name := strings.TrimPrefix(snapshotFile, snapshotDir)
	if err := ctx.Git().CommitWithMessage(fmt.Sprintf("adding snapshot %q for label %q", name, label)); err != nil {
		return err
	}
	if remoteFlag {
		if err := ctx.Git().Push("origin", "master", gitutil.VerifyOpt(false)); err != nil {
			return err
		}
	}
	return nil
}

// cmdSnapshotList represents the "jiri snapshot list" command.
var cmdSnapshotList = &cmdline.Command{
	Runner: cmdline.RunnerFunc(runSnapshotList),
	Name:   "list",
	Short:  "List existing project snapshots",
	Long: `
The "snapshot list" command lists existing snapshots of the labels
specified as command-line arguments. If no arguments are provided, the
command lists snapshots for all known labels.
`,
	ArgsName: "<label ...>",
	ArgsLong: "<label ...> is a list of snapshot labels.",
}

func runSnapshotList(env *cmdline.Env, args []string) error {
	ctx := tool.NewContextFromEnv(env)
	if err := checkSnapshotDir(ctx); err != nil {
		return err
	}

	snapshotDir, err := getSnapshotDir()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		// Identify all known snapshot labels, using a
		// heuristic that looks for all symbolic links <foo>
		// in the snapshot directory that point to a file in
		// the "labels/<foo>" subdirectory of the snapshot
		// directory.
		fileInfoList, err := ioutil.ReadDir(snapshotDir)
		if err != nil {
			return fmt.Errorf("ReadDir(%v) failed: %v", snapshotDir, err)
		}
		for _, fileInfo := range fileInfoList {
			if fileInfo.Mode()&os.ModeSymlink != 0 {
				path := filepath.Join(snapshotDir, fileInfo.Name())
				dst, err := filepath.EvalSymlinks(path)
				if err != nil {
					return fmt.Errorf("EvalSymlinks(%v) failed: %v", path, err)
				}
				if strings.HasSuffix(filepath.Dir(dst), filepath.Join("labels", fileInfo.Name())) {
					args = append(args, fileInfo.Name())
				}
			}
		}
	}

	// Check that all labels exist.
	failed := false
	for _, label := range args {
		labelDir := filepath.Join(snapshotDir, "labels", label)
		if _, err := ctx.Run().Stat(labelDir); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			failed = true
			fmt.Fprintf(env.Stderr, "snapshot label %q not found", label)
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}

	// Print snapshots for all labels.
	sort.Strings(args)
	for _, label := range args {
		// Scan the snapshot directory "labels/<label>" printing
		// all snapshots.
		labelDir := filepath.Join(snapshotDir, "labels", label)
		fileInfoList, err := ioutil.ReadDir(labelDir)
		if err != nil {
			return fmt.Errorf("ReadDir(%v) failed: %v", labelDir, err)
		}
		fmt.Fprintf(env.Stdout, "snapshots of label %q:\n", label)
		for _, fileInfo := range fileInfoList {
			fmt.Fprintf(env.Stdout, "  %v\n", fileInfo.Name())
		}
	}
	return nil
}
