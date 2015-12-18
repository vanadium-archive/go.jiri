// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"

	"v.io/jiri/jiri"
	"v.io/jiri/project"
	"v.io/jiri/runutil"
	"v.io/x/lib/cmdline"
)

var (
	// Flags for configuring project attributes.
	flagImportName, flagImportPath, flagImportProtocol, flagImportRemoteBranch, flagImportRevision, flagImportRoot string
	// Flags for controlling the behavior of the command.
	flagImportMode importMode
	flagImportOut  string
)

func init() {
	cmdImport.Flags.StringVar(&flagImportName, "name", "", `The name of the remote manifest project, used to disambiguate manifest projects with the same remote.  Typically empty.`)
	cmdImport.Flags.StringVar(&flagImportPath, "path", "", `Path to store the manifest project locally.  Uses "manifest" if unspecified.`)
	cmdImport.Flags.StringVar(&flagImportProtocol, "protocol", "git", `The version control protocol used by the remote manifest project.`)
	cmdImport.Flags.StringVar(&flagImportRemoteBranch, "remotebranch", "master", `The branch of the remote manifest project to track.`)
	cmdImport.Flags.StringVar(&flagImportRevision, "revision", "HEAD", `The revision of the remote manifest project to reset to during "jiri update".`)
	cmdImport.Flags.StringVar(&flagImportRoot, "root", "", `Root to store the manifest project locally.`)

	cmdImport.Flags.Var(&flagImportMode, "mode", `
The import mode:
   append    - Create file if it doesn't exist, or append to existing file.
   overwrite - Write file regardless of whether it already exists.
`)
	cmdImport.Flags.StringVar(&flagImportOut, "out", "", `The output file.  Uses $JIRI_ROOT/.jiri_manifest if unspecified.  Uses stdout if set to "-".`)
}

type importMode int

const (
	importAppend importMode = iota
	importOverwrite
)

func (m *importMode) Set(s string) error {
	switch s {
	case "append":
		*m = importAppend
		return nil
	case "overwrite":
		*m = importOverwrite
		return nil
	}
	return fmt.Errorf("unknown import mode %q", s)
}

func (m importMode) String() string {
	switch m {
	case importAppend:
		return "append"
	case importOverwrite:
		return "overwrite"
	}
	return "UNKNOWN"
}

func (m importMode) Get() interface{} {
	return m
}

var cmdImport = &cmdline.Command{
	Runner: jiri.RunnerFunc(runImport),
	Name:   "import",
	Short:  "Adds imports to .jiri_manifest file",
	Long: `
Command "import" adds imports to the $JIRI_ROOT/.jiri_manifest file, which
specifies manifest information for the jiri tool.  The file is created if it
doesn't already exist, otherwise additional imports are added to the existing
file.  The arguments and flags configure the <import> element that is added to
the manifest.

Run "jiri help manifest" for details on manifests.
`,
	ArgsName: "<remote> <manifest>",
	ArgsLong: `
<remote> specifies the remote repository that contains your manifest project.

<manifest> specifies the manifest file to use from the manifest project.
`,
}

func runImport(jirix *jiri.X, args []string) error {
	if len(args) != 2 || args[0] == "" || args[1] == "" {
		return jirix.UsageErrorf("must specify non-empty <remote> and <manifest>")
	}
	// Initialize manifest.
	var manifest *project.Manifest
	if flagImportMode == importAppend {
		m, err := project.ManifestFromFile(jirix, jirix.JiriManifestFile())
		if err != nil && !runutil.IsNotExist(err) {
			return err
		}
		manifest = m
	}
	if manifest == nil {
		manifest = &project.Manifest{}
	}
	// Add remote import.
	manifest.Imports = append(manifest.Imports, project.Import{
		Manifest: args[1],
		Root:     flagImportRoot,
		Project: project.Project{
			Name:         flagImportName,
			Path:         flagImportPath,
			Protocol:     flagImportProtocol,
			Remote:       args[0],
			RemoteBranch: flagImportRemoteBranch,
			Revision:     flagImportRevision,
		},
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
