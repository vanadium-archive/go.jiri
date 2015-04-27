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
	"path/filepath"
	"regexp"
	"strings"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
	"v.io/x/lib/cmdline"
)

var (
	detailedOutputFlag bool
	gotoolsBinPathFlag string
	commentRE          = regexp.MustCompile("^($|[:space:]*#)")
)

func init() {
	cmdApiCheck.Flags.BoolVar(&detailedOutputFlag, "detailed", true, "If true, shows each API change in an expanded form. Otherwise, only a summary is shown.")
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
	ArgsLong: "<projects> is a list of Vanadium projects to check. If none are specified, all projects that require a public API check upon presubmit are checked.",
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
	name          string
	projectName   string
	apiFilePath   string
	oldApi        map[string]bool // set
	newApi        map[string]bool // set
	newApiContent []byte

	// If true, indicates that there was a problem reading the old API file.
	apiFileError error
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

func isFailedApiCheckFatal(projectName string, apiCheckRequiredProjects map[string]bool, apiFileError error) bool {
	if pathError, ok := apiFileError.(*os.PathError); ok {
		if pathError.Err == os.ErrNotExist {
			if _, ok := apiCheckRequiredProjects[projectName]; !ok {
				return false
			}
		}
	}

	return true
}

func shouldIgnoreFile(file string) bool {
	if !strings.HasSuffix(file, ".go") {
		return true
	}
	pathComponents := strings.Split(file, string(os.PathSeparator))
	for _, component := range pathComponents {
		if component == "testdata" || component == "internal" {
			return true
		}
	}
	return false
}

// parseProjectNames identifies the set of projects that the "v23 api
// ..." command should be applied to.
func parseProjectNames(args []string, projects map[string]util.Project, apiCheckRequiredProjects map[string]bool) ([]string, error) {
	names := args
	if len(names) == 0 {
		// Use all projects for which an API check is required.
		for name, _ := range apiCheckRequiredProjects {
			names = append(names, name)
		}
	} else {
		for _, name := range names {
			if _, ok := projects[name]; !ok {
				return nil, fmt.Errorf("project %q does not exist in the project manifest", name)
			}
		}
	}
	return names, nil
}

func splitLinesToSet(in io.Reader) map[string]bool {
	result := make(map[string]bool)
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		result[scanner.Text()] = true
	}
	return result
}

func packageName(path string) string {
	components := strings.Split(path, string(os.PathSeparator))
	for i, component := range components {
		if component == "src" {
			return strings.Join(components[i+1:], "/")
		}
	}
	return ""
}

func getPackageChanges(ctx *tool.Context, apiCheckRequiredProjects map[string]bool, args []string) (changes []packageChange, e error) {
	projects, _, err := util.ReadManifest(ctx)
	if err != nil {
		return nil, err
	}
	projectNames, err := parseProjectNames(args, projects, apiCheckRequiredProjects)
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
			if !shouldIgnoreFile(file) {
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
			apiFileError := readApiFileContents(apiFilePath, &apiFileContents)
			if apiFileError != nil {
				if !isFailedApiCheckFatal(projectName, apiCheckRequiredProjects, apiFileError) {
					// We couldn't read the API file, but
					// this project doesn't require one.
					// Just warn the user.
					fmt.Fprintf(ctx.Stderr(), "WARNING: could not read public API from %s: %v\n", apiFilePath, err)
					fmt.Fprintf(ctx.Stderr(), "WARNING: skipping public API check for %s\n", dir)
					continue
				}
			}
			var out bytes.Buffer
			opts := ctx.Run().Opts()
			opts.Stdout = &out
			if err := ctx.Run().CommandWithOpts(opts, "v23", "run", gotoolsBin, "goapi", dir); err != nil {
				return nil, err
			}
			pkgName := packageName(dir)
			if pkgName == "" {
				pkgName = dir
			}
			if apiFileError != nil || out.String() != apiFileContents.String() {
				// The user has changed the public API or we
				// couldn't read the public API in the first
				// place.
				changes = append(changes, packageChange{
					name:          pkgName,
					projectName:   projectName,
					apiFilePath:   apiFilePath,
					oldApi:        splitLinesToSet(&apiFileContents),
					newApi:        splitLinesToSet(&out),
					newApiContent: out.Bytes(),
					apiFileError:  apiFileError,
				})
			}
		}
	}
	return
}

func runApiCheck(command *cmdline.Command, args []string) error {
	return doApiCheck(command.Stdout(), command.Stderr(), args, detailedOutputFlag)
}

func printChangeSummary(out io.Writer, change packageChange, detailedOutput bool) {
	var removedEntries []string
	var addedEntries []string
	for entry, _ := range change.oldApi {
		if !change.newApi[entry] {
			removedEntries = append(removedEntries, entry)
		}
	}
	for entry, _ := range change.newApi {
		if !change.oldApi[entry] {
			addedEntries = append(addedEntries, entry)
		}
	}
	if detailedOutput {
		fmt.Fprintf(out, "Changes for package %s\n", change.name)
		if len(removedEntries) > 0 {
			fmt.Fprintf(out, "The following %d entries were removed:\n", len(removedEntries))
			for _, entry := range removedEntries {
				fmt.Fprintf(out, "\t%s\n", entry)
			}
		}
		if len(addedEntries) > 0 {
			fmt.Fprintf(out, "The following %d entries were added:\n", len(addedEntries))
			for _, entry := range addedEntries {
				fmt.Fprintf(out, "\t%s\n", entry)
			}
		}
	} else {
		fmt.Fprintf(out, "package %s: %d entries removed, %d entries added\n", change.name, len(removedEntries), len(addedEntries))
	}
}

func doApiCheck(stdout, stderr io.Writer, args []string, detailedOutput bool) error {
	ctx := tool.NewContext(tool.ContextOpts{
		Color:    &colorFlag,
		DryRun:   &dryRunFlag,
		Manifest: &manifestFlag,
		Verbose:  &verboseFlag,
		Stdout:   stdout,
		Stderr:   stderr,
	})
	config, err := util.LoadConfig(ctx)
	if err != nil {
		return err
	}
	changes, err := getPackageChanges(ctx, config.ApiCheckRequiredProjects(), args)
	if err != nil {
		return err
	} else if len(changes) > 0 {
		for _, change := range changes {
			if change.apiFileError != nil {
				fmt.Fprintf(stdout, "ERROR: package %s: could not read the package's .api file: %v\n", change.name, change.apiFileError)
				fmt.Fprintf(stdout, "ERROR: a readable .api file is required for all packages in project %s\n", change.projectName)
			} else {
				printChangeSummary(stdout, change, detailedOutput)
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
	config, err := util.LoadConfig(ctx)
	if err != nil {
		return err
	}
	changes, err := getPackageChanges(ctx, config.ApiCheckRequiredProjects(), args)
	if err != nil {
		return err
	}
	for _, change := range changes {
		if err := ctx.Run().WriteFile(change.apiFilePath, []byte(change.newApiContent), 0644); err != nil {
			return fmt.Errorf("WriteFile(%s) failed: %v", change.apiFilePath, err)
		}
		fmt.Fprintf(ctx.Stdout(), "Updated %s.\n", change.apiFilePath)
	}
	return nil
}
