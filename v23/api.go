// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

var (
	gotoolsBinPathFlag string
	commentRE          = regexp.MustCompile("^($|[:space:]*#)")
)

func init() {
	cmdApi.Flags.StringVar(&gotoolsBinPathFlag, "gotools-bin", "", "The path to the gotools binary to use. If empty, gotools will be built if necessary.")
}

// cmdApi represents the "v23 api" command.
var cmdApi = &cmdline.Command{
	Name:  "api",
	Short: "Work with Vanadium's public API",
	Long: `
Use this command to ensure that no unintended changes are made to Vanadium's
public API.
`,
	Children: []*cmdline.Command{cmdApiCheck, cmdApiUpdate},
}

// cmdApiCheck represents the "v23 api check" command.
var cmdApiCheck = &cmdline.Command{
	Run:      runApiCheck,
	Name:     "check",
	Short:    "Check to see if any changes have been made to the public API.",
	Long:     "Check to see if any changes have been made to the public API.",
	ArgsName: "<projects>",
	ArgsLong: "<projects> is a list of Vanadium projects to check. If none are specified, all projects are checked.",
}

func readApiFileContents(path string, buf *bytes.Buffer) (e error) {
	file, err := os.Open(path)
	defer collect.Error(file.Close, &e)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadBytes('\n')
		if !commentRE.Match(line) {
			buf.Write(line)
		}
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
	}
	return
}

type packageChange struct {
	name        string
	apiFilePath string
	newApi      string
}

// buildGotools builds the gotools binary and returns the path to the built
// binary and the function to call to clean up the built binary (always
// non-nil). If the binary could not be built, the empty string and a non-nil
// error are returned.
//
// If the gotools_bin flag is specified, that path, a no-op cleanup and a
// nil error are returned.
func buildGotools(ctx *tool.Context) (string, func() error, error) {
	nopCleanup := func() error { return nil }
	if gotoolsBinPathFlag != "" {
		return gotoolsBinPathFlag, nopCleanup, nil
	}

	// Determine the location of the gotools source.
	projects, _, err := util.ReadManifest(ctx)
	if err != nil {
		return "", nopCleanup, err
	}

	project, ok := projects["third_party"]
	if !ok {
		return "", nopCleanup, fmt.Errorf(`project "third_party" not found`)
	}
	newGoPath := filepath.Join(project.Path, "go")

	// Build the gotools binary.
	tempDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return "", nopCleanup, err
	}
	cleanup := func() error { return ctx.Run().RemoveAll(tempDir) }

	gotoolsBin := filepath.Join(tempDir, "gotools")
	opts := ctx.Run().Opts()
	opts.Env["GOPATH"] = newGoPath
	if err := ctx.Run().CommandWithOpts(opts, "go", "build", "-o", gotoolsBin, "github.com/visualfc/gotools"); err != nil {
		return "", cleanup, err
	}

	return gotoolsBin, cleanup, nil
}

func getPackageChanges(ctx *tool.Context, command *cmdline.Command, args []string) (changes []packageChange, e error) {
	projects, _, err := util.ReadManifest(ctx)
	if err != nil {
		return nil, err
	}
	projectNames, err := parseArgs(args, projects)
	if err != nil {
		return nil, err
	}

	gotoolsBin, cleanup, err := buildGotools(ctx)
	if err != nil {
		return nil, err
	}
	defer collect.Error(cleanup, &e)

	for _, projectName := range projectNames {
		path := projects[projectName].Path
		branch, err := ctx.Git(tool.RootDirOpt(path)).CurrentBranchName()
		if err != nil {
			return nil, err
		}
		files, err := ctx.Git(tool.RootDirOpt(path)).ModifiedFiles("master", branch)
		if err != nil {
			return nil, err
		}
		// Extract the directories for these files.
		dirs := make(map[string]bool) // set
		for _, file := range files {
			if strings.HasSuffix(file, ".go") {
				dirs[filepath.Join(path, filepath.Dir(file))] = true
			}
		}
		if len(dirs) == 0 {
			continue
		}
		for dir := range dirs {
			// Read the existing public API file.
			apiFilePath := filepath.Join(dir, ".api")
			var apiFileContents bytes.Buffer
			err := readApiFileContents(apiFilePath, &apiFileContents)
			if err != nil {
				fmt.Fprintf(ctx.Stderr(), "WARNING: could not read public API from %s: %v\n", apiFilePath, err)
				fmt.Fprintf(ctx.Stderr(), "WARNING: skipping public API check for %s\n", dir)
				continue
			}
			var out bytes.Buffer
			opts := ctx.Run().Opts()
			opts.Stdout = &out
			if err := ctx.Run().CommandWithOpts(opts, gotoolsBin, "goapi", dir); err != nil {
				return nil, err
			}
			if out.String() != apiFileContents.String() {
				// The user has changed the public API.
				changes = append(changes, packageChange{name: dir, apiFilePath: apiFilePath, newApi: out.String()})
			}
		}
	}
	return
}

func runApiCheck(command *cmdline.Command, args []string) error {
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:    &colorFlag,
		DryRun:   &dryRunFlag,
		Manifest: &manifestFlag,
		Verbose:  &verboseFlag})
	changes, err := getPackageChanges(ctx, command, args)
	if err != nil {
		return err
	} else if len(changes) > 0 {
		fmt.Fprintf(ctx.Stdout(), "Detected changes in the following %d package(s):\n", len(changes))
		for _, change := range changes {
			fmt.Fprintf(ctx.Stdout(), "\t%s\n", change.name)
			opts := ctx.Run().Opts()
			opts.Stdin = strings.NewReader(change.newApi)
			if err := ctx.Run().CommandWithOpts(opts, "diff", "-u", change.apiFilePath, "-"); err != nil {
				// We expect diff to return 1 if changes are
				// detected
				if exiterr, ok := err.(*exec.ExitError); ok {
					if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
						if status != 1 {
							continue
						}
					}
				}
				// If we got here, diff returned a non-nil err
				// other than an ExitError with status code=1
				fmt.Fprintf(ctx.Stderr(), "WARNING: got an error while running diff: %v", err)
			}
		}
	}
	return nil
}

// cmdApiUpdate represents the "v23 api fix" command.
var cmdApiUpdate = &cmdline.Command{
	Run:      runApiFix,
	Name:     "fix",
	Short:    "Updates the .api files to reflect your changes to the public API.",
	Long:     "Updates the .api files to reflect your changes to the public API.",
	ArgsName: "<projects>",
	ArgsLong: "<projects> is a list of Vanadium projects to update. If none are specified, all project APIs are updated.",
}

func runApiFix(command *cmdline.Command, args []string) error {
	ctx := tool.NewContextFromCommand(command, tool.ContextOpts{
		Color:    &colorFlag,
		DryRun:   &dryRunFlag,
		Manifest: &manifestFlag,
		Verbose:  &verboseFlag})
	changes, err := getPackageChanges(ctx, command, args)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if err := ctx.Run().WriteFile(change.apiFilePath, []byte(change.newApi), 0644); err != nil {
			return fmt.Errorf("WriteFile(%s) failed: %v", change.apiFilePath, err)
		}
		fmt.Fprintf(ctx.Stdout(), "Updated %s.\n", change.apiFilePath)
	}
	return nil
}
