// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"v.io/jiri/jiri"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/x/lib/cmdline"
)

// TODO(toddw): Remove the upgrade command after the transition to new-style
// manifests is complete.

var flagUpgradeRevert bool

func init() {
	cmdUpgrade.Flags.BoolVar(&flagUpgradeRevert, "revert", false, `Revert the upgrade by deleting the $JIRI_ROOT/.jiri_manifest file.`)
}

var cmdUpgrade = &cmdline.Command{
	Runner: jiri.RunnerFunc(runUpgrade),
	Name:   "upgrade",
	Short:  "Upgrade jiri to new-style manifests",
	Long: `
Upgrades jiri to use new-style manifests.

The old (deprecated) behavior only allowed a single manifest repository, located
in $JIRI_ROOT/.manifest.  The initial manifest file is located as follows:
  1) Use -manifest flag, if non-empty.  If it's empty...
  2) Use $JIRI_ROOT/.local_manifest file.  If it doesn't exist...
  3) Use $JIRI_ROOT/.manifest/v2/default.

The new behavior allows multiple manifest repositories, by allowing imports to
specify project attributes describing the remote repository.  The -manifest flag
is no longer allowed to be set; the initial manifest file is always located in
$JIRI_ROOT/.jiri_manifest.  The .local_manifest file is ignored.

During the transition phase, both old and new behaviors are supported.  The jiri
tool uses the existence of the $JIRI_ROOT/.jiri_manifest file as the signal; if
it exists we run the new behavior, otherwise we run the old behavior.

The new behavior includes a "jiri import" command, which writes or updates the
.jiri_manifest file.  The new bootstrap procedure runs "jiri import", and it is
intended as a regular command to add imports to your jiri environment.

This upgrade command eases the transition by writing an initial .jiri_manifest
file for you.  If you have an existing .local_manifest file, its contents will
be incorporated into the new .jiri_manifest file, and it will be renamed to
.local_manifest.BACKUP.  The -revert flag deletes the .jiri_manifest file, and
restores the .local_manifest file.
`,
	ArgsName: "<kind>",
	ArgsLong: `
<kind> specifies the kind of upgrade, one of "v23" or "fuchsia".
`,
}

func runUpgrade(jirix *jiri.X, args []string) error {
	localFile := filepath.Join(jirix.Root, ".local_manifest")
	backupFile := localFile + ".BACKUP"
	if flagUpgradeRevert {
		// Restore .local_manifest.BACKUP if it exists.
		switch _, err := jirix.NewSeq().Stat(backupFile); {
		case err != nil && !runutil.IsNotExist(err):
			return err
		case err == nil:
			if err := jirix.NewSeq().Rename(backupFile, localFile).Done(); err != nil {
				return fmt.Errorf("couldn't restore %v to %v: %v", backupFile, localFile, err)
			}
		}
		// Deleting the .jiri_manifest file reverts to the old behavior.
		return jirix.NewSeq().Remove(jirix.JiriManifestFile()).Done()
	}
	if len(args) != 1 {
		return jirix.UsageErrorf("must specify upgrade kind")
	}
	kind := args[0]
	var argRemote, argName, argManifest string
	switch kind {
	case "v23":
		argRemote = "https://vanadium.googlesource.com/manifest"
		argName, argManifest = "manifest", "public"
	case "fuchsia":
		argRemote = "https://fuchsia.googlesource.com/fnl-start"
		argName, argManifest = "fnl-start", "manifest/fuchsia"
	default:
		return jirix.UsageErrorf("unknown upgrade kind %q", kind)
	}
	// Initialize manifest from .local_manifest.
	hasLocalFile := true
	manifest, err := project.ManifestFromFile(jirix, localFile)
	if err != nil {
		if !runutil.IsNotExist(err) {
			return err
		}
		hasLocalFile = false
		manifest = &project.Manifest{}
	}
	oldImports := manifest.Imports
	manifest.Imports = nil
	for _, oldImport := range oldImports {
		if oldImport.Remote != "" {
			// This is a new-style remote import, carry it over directly.
			manifest.Imports = append(manifest.Imports, oldImport)
			continue
		}
		// This is an old-style import, convert it to the new style.
		oldName := oldImport.Name
		switch {
		case kind == "v23" && oldName == "default":
			oldName = "public"
		case kind == "fuchsia" && oldName == "default":
			oldName = "manifest/fuchsia"
		}
		manifest.Imports = append(manifest.Imports, project.Import{
			Manifest: oldName,
			Name:     argName,
			Remote:   argRemote,
		})
	}
	if len(manifest.Imports) == 0 {
		manifest.Imports = append(manifest.Imports, project.Import{
			Manifest: argManifest,
			Name:     argName,
			Remote:   argRemote,
		})
	}
	// Write output to .jiri_manifest file.
	outFile := jirix.JiriManifestFile()
	if _, err := os.Stat(outFile); err == nil {
		return fmt.Errorf("%v already exists", outFile)
	}
	if err := manifest.ToFile(jirix, outFile); err != nil {
		return err
	}
	// Backup .local_manifest file, if it exists.
	if hasLocalFile {
		if err := jirix.NewSeq().Rename(localFile, backupFile).Done(); err != nil {
			return fmt.Errorf("couldn't backup %v to %v: %v", localFile, backupFile, err)
		}
	}
	return nil
}
