// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"

	"v.io/jiri/jiri"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/x/lib/cmdline"
)

var (
	// Flags for configuring project attributes for remote imports.
	flagImportName, flagImportProtocol, flagImportRemoteBranch, flagImportRoot string
	// Flags for controlling the behavior of the command.
	flagImportOverwrite bool
	flagImportOut       string
)

func init() {
	cmdImport.Flags.StringVar(&flagImportName, "name", "", `The name of the remote manifest project, used to disambiguate manifest projects with the same remote.  Typically empty.`)
	cmdImport.Flags.StringVar(&flagImportProtocol, "protocol", "git", `The version control protocol used by the remote manifest project.`)
	cmdImport.Flags.StringVar(&flagImportRemoteBranch, "remote-branch", "master", `The branch of the remote manifest project to track, without the leading "origin/".`)
	cmdImport.Flags.StringVar(&flagImportRoot, "root", "", `Root to store the manifest project locally.`)

	cmdImport.Flags.BoolVar(&flagImportOverwrite, "overwrite", false, `Write a new .jiri_manifest file with the given specification.  If it already exists, the existing content will be ignored and the file will be overwritten.`)
	cmdImport.Flags.StringVar(&flagImportOut, "out", "", `The output file.  Uses $JIRI_ROOT/.jiri_manifest if unspecified.  Uses stdout if set to "-".`)
}

var cmdImport = &cmdline.Command{
	Runner: jiri.RunnerFunc(runImport),
	Name:   "import",
	Short:  "Adds imports to .jiri_manifest file",
	Long: `
Command "import" adds imports to the $JIRI_ROOT/.jiri_manifest file, which
specifies manifest information for the jiri tool.  The file is created if it
doesn't already exist, otherwise additional imports are added to the existing
file.

An <import> element is added to the manifest representing a remote manifest
import.  The manifest file path is relative to the root directory of the remote
import repository.

Example:
  $ jiri import myfile https://foo.com/bar.git

Run "jiri help manifest" for details on manifests.
`,
	ArgsName: "<manifest> <remote>",
	ArgsLong: `
<manifest> specifies the manifest file to use.

<remote> specifies the remote manifest repository.
`,
}

func runImport(jirix *jiri.X, args []string) error {
	if len(args) != 2 {
		return jirix.UsageErrorf("wrong number of arguments")
	}
	// Initialize manifest.
	var manifest *project.Manifest
	if !flagImportOverwrite {
		m, err := project.ManifestFromFile(jirix, jirix.JiriManifestFile())
		if err != nil && !runutil.IsNotExist(err) {
			return err
		}
		manifest = m
	}
	if manifest == nil {
		manifest = &project.Manifest{}
	}
	// There's not much error checking when writing the .jiri_manifest file;
	// errors will be reported when "jiri update" is run.
	manifest.Imports = append(manifest.Imports, project.Import{
		Manifest:     args[0],
		Name:         flagImportName,
		Protocol:     flagImportProtocol,
		Remote:       args[1],
		RemoteBranch: flagImportRemoteBranch,
		Root:         flagImportRoot,
	})
	// Write output to stdout or file.
	outFile := flagImportOut
	if outFile == "" {
		outFile = jirix.JiriManifestFile()
	}
	if outFile == "-" {
		bytes, err := manifest.ToBytes()
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(bytes)
		return err
	}
	return manifest.ToFile(jirix, outFile)
}
