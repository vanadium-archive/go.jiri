// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"v.io/jiri/collect"
	"v.io/jiri/gitutil"
	"v.io/jiri/googlesource"
	"v.io/jiri/jiri"
	"v.io/jiri/runutil"
	"v.io/jiri/tool"
	"v.io/x/lib/cmdline"
	"v.io/x/lib/set"
)

var JiriProject = "release.go.jiri"
var JiriName = "jiri"
var JiriPackage = "v.io/jiri"

// CL represents a changelist.
type CL struct {
	// Author identifies the author of the changelist.
	Author string
	// Email identifies the author's email.
	Email string
	// Description holds the description of the changelist.
	Description string
}

// Manifest represents a setting used for updating the universe.
type Manifest struct {
	Hooks    []Hook    `xml:"hooks>hook"`
	Hosts    []Host    `xml:"hosts>host"`
	Imports  []Import  `xml:"imports>import"`
	Label    string    `xml:"label,attr"`
	Projects []Project `xml:"projects>project"`
	Tools    []Tool    `xml:"tools>tool"`
	XMLName  struct{}  `xml:"manifest"`
}

// Hooks maps hook names to their detailed description.
type Hooks map[string]Hook

// Hook represents a post-update project hook.
type Hook struct {
	// Exclude is flag used to exclude previously included hooks.
	Exclude bool `xml:"exclude,attr"`
	// Name is the hook name.
	Name string `xml:"name,attr"`
	// Project is the name of the project the hook is associated with.
	Project string `xml:"project,attr"`
	// Path is the path of the hook relative to its project's root.
	Path string `xml:"path,attr"`
	// Interpreter is an optional program used to interpret the hook (i.e. python). Unlike Path,
	// Interpreter is relative to the environment's PATH and not the project's root.
	Interpreter string `xml:"interpreter,attr"`
	// Arguments for the hook.
	Args []HookArg `xml:"arg"`
}

type HookArg struct {
	Arg string `xml:",chardata"`
}

// Hosts map host name to their detailed description.
type Hosts map[string]Host

// Host represents the locations of git and gerrit repository hosts.
type Host struct {
	// Name is the host name.
	Name string `xml:"name,attr"`
	// Location is the url of the host.
	Location string `xml:"location,attr"`
	// Git hooks to apply to repos from this host.
	GitHooks []GitHook `xml:"githooks>githook"`
}

// GitHook represents the name and source of git hooks.
type GitHook struct {
	// The hook name, as required by git (e.g. commit-msg, pre-rebase, etc.)
	Name string `xml:"name,attr"`
	// The filename of the hook implementation.  When editing the manifest,
	// specify this path as relative to the manifest dir.  In loadManifest,
	// this gets resolved to the absolute path.
	Path string `xml:"path,attr"`
}

// Imports maps manifest import names to their detailed description.
type Imports map[string]Import

// Import represnts a manifest import.
type Import struct {
	// Name is the name under which the manifest can be found the
	// manifest repository.
	Name string `xml:"name,attr"`
}

// ProjectKey is a unique string for a project.
type ProjectKey string

// ProjectKeys is a slice of ProjectKeys implementing the Sort interface.
type ProjectKeys []ProjectKey

func (pks ProjectKeys) Len() int           { return len(pks) }
func (pks ProjectKeys) Less(i, j int) bool { return string(pks[i]) < string(pks[j]) }
func (pks ProjectKeys) Swap(i, j int)      { pks[i], pks[j] = pks[j], pks[i] }

// Project represents a jiri project.
type Project struct {
	// Exclude is flag used to exclude previously included projects.
	Exclude bool `xml:"exclude,attr"`
	// Name is the project name.
	Name string `xml:"name,attr"`
	// Path is the path used to store the project locally. Project
	// manifest uses paths that are relative to the $JIRI_ROOT
	// environment variable. When a manifest is parsed (e.g. in
	// RemoteProjects), the program logic converts the relative
	// paths to an absolute paths, using the current value of the
	// $JIRI_ROOT environment variable as a prefix.
	Path string `xml:"path,attr"`
	// Protocol is the version control protocol used by the
	// project. If not set, "git" is used as the default.
	Protocol string `xml:"protocol,attr"`
	// Remote is the project remote.
	Remote string `xml:"remote,attr"`
	// RemoteBranch is the name of the remote branch to track.  It doesn't affect
	// the name of the local branch that jiri maintains, which is always "master".
	RemoteBranch string `xml:"remotebranch,attr"`
	// Revision is the revision the project should be advanced to
	// during "jiri update". If not set, "HEAD" is used as the
	// default.
	Revision string `xml:"revision,attr"`
}

// projectKeySeparator is a reserved string used in ProjectKeys.  It cannot
// occur in Project names or remotes.
const projectKeySeparator = "="

// Key returns a unique ProjectKey for the project.
func (p Project) Key() ProjectKey {
	return ProjectKey(p.Name + projectKeySeparator + p.Remote)
}

// Projects maps ProjectKeys to Projects.
type Projects map[ProjectKey]Project

// Find returns all projects in Projects with the given key or name.
func (ps Projects) Find(name string) Projects {
	projects := Projects{}
	for _, p := range ps {
		if name == p.Name {
			projects[p.Key()] = p
		}
	}
	return projects
}

// FindUnique returns the project in Projects with the given key or
// name, and returns an error if none or multiple matching projects are found.
func (ps Projects) FindUnique(name string) (Project, error) {
	var p Project
	projects := ps.Find(name)
	if len(projects) == 0 {
		return p, fmt.Errorf("no projects found with name %q", name)
	}
	if len(projects) > 1 {
		return p, fmt.Errorf("multiple projects found with name %q", name)
	}
	// Return the only project in projects.
	for _, project := range projects {
		p = project
	}
	return p, nil
}

// Tools maps jiri tool names, to their detailed description.
type Tools map[string]Tool

// Tool represents a jiri tool.
type Tool struct {
	// Exclude is flag used to exclude previously included projects.
	Exclude bool `xml:"exclude,attr"`
	// Data is a relative path to a directory for storing tool data
	// (e.g. tool configuration files). The purpose of this field is to
	// decouple the configuration of the data directory from the tool
	// itself so that the location of the data directory can change
	// without the need to change the tool.
	Data string `xml:"data,attr"`
	// Name is the name of the tool binary.
	Name string `xml:"name,attr"`
	// Package is the package path of the tool.
	Package string `xml:"package,attr"`
	// Project identifies the project that contains the tool. If not
	// set, "https://vanadium.googlesource.com/<JiriProject>" is
	// used as the default.
	Project string `xml:"project,attr"`
}

// ScanMode determines whether LocalProjects should scan the local filesystem
// for projects (FullScan), or optimistically assume that the local projects
// will match those in the manifest (FastScan).
type ScanMode bool

const (
	FastScan = ScanMode(false)
	FullScan = ScanMode(true)
)

type UnsupportedProtocolErr string

func (e UnsupportedProtocolErr) Error() string {
	return "unsupported protocol: " + string(e)
}

// Update represents an update of projects as a map from
// project names to a collections of commits.
type Update map[string][]CL

// CreateSnapshot creates a manifest that encodes the current state of
// master branches of all projects and writes this snapshot out to the
// given file.
func CreateSnapshot(jirix *jiri.X, path string) error {
	jirix.TimerPush("create snapshot")
	defer jirix.TimerPop()

	manifest := Manifest{}

	// Add all local projects to manifest.
	localProjects, err := LocalProjects(jirix, FullScan)
	if err != nil {
		return err
	}
	for _, project := range localProjects {
		relPath, err := toRel(jirix, project.Path)
		if err != nil {
			return err
		}
		project.Path = relPath
		manifest.Projects = append(manifest.Projects, project)
	}

	// Add all hosts, tools, and hooks from the current manifest to the
	// snapshot manifest.
	hosts, _, tools, hooks, err := readManifest(jirix, true)
	if err != nil {
		return err
	}
	for _, tool := range tools {
		manifest.Tools = append(manifest.Tools, tool)
	}
	for _, host := range hosts {
		manifest.Hosts = append(manifest.Hosts, host)
	}
	for _, hook := range hooks {
		manifest.Hooks = append(manifest.Hooks, hook)
	}

	perm := os.FileMode(0755)
	if err := jirix.Run().MkdirAll(filepath.Dir(path), perm); err != nil {
		return err
	}
	data, err := xml.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent(%v) failed: %v", manifest, err)
	}
	perm = os.FileMode(0644)
	if err := jirix.Run().WriteFile(path, data, perm); err != nil {
		return err
	}
	return nil
}

const currentManifestFileName = ".current_manifest"

// CurrentManifest returns a manifest that identifies the result of
// the most recent "jiri update" invocation.
func CurrentManifest(jirix *jiri.X) (*Manifest, error) {
	currentManifestPath := toAbs(jirix, currentManifestFileName)
	bytes, err := jirix.Run().ReadFile(currentManifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(jirix.Stderr(), `WARNING: Could not find %s.
The contents of this file are stored as metadata in binaries the jiri
tool builds. To fix this problem, please run "jiri update".
`, currentManifestPath)
			return &Manifest{}, nil
		}
		return nil, err
	}
	var m Manifest
	if err := xml.Unmarshal(bytes, &m); err != nil {
		return nil, fmt.Errorf("Unmarshal(%v) failed: %v", string(bytes), err)
	}
	return &m, nil
}

// writeCurrentManifest writes the given manifest to a file that
// stores the result of the most recent "jiri update" invocation.
func writeCurrentManifest(jirix *jiri.X, manifest *Manifest) error {
	currentManifestPath := toAbs(jirix, currentManifestFileName)
	bytes, err := xml.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("MarshalIndent(%v) failed: %v", manifest, err)
	}
	if err := jirix.Run().WriteFile(currentManifestPath, bytes, os.FileMode(0644)); err != nil {
		return err
	}
	return nil
}

// CurrentProjectKey gets the key of the current project from the current
// directory by reading the jiri project metadata located in a directory at the
// root of the current repository.
func CurrentProjectKey(jirix *jiri.X) (ProjectKey, error) {
	topLevel, err := jirix.Git().TopLevel()
	if err != nil {
		return "", nil
	}
	metadataDir := filepath.Join(topLevel, jiri.ProjectMetaDir)
	if _, err := jirix.Run().Stat(metadataDir); err == nil {
		metadataFile := filepath.Join(metadataDir, jiri.ProjectMetaFile)
		bytes, err := jirix.Run().ReadFile(metadataFile)
		if err != nil {
			return "", err
		}
		var project Project
		if err := xml.Unmarshal(bytes, &project); err != nil {
			return "", fmt.Errorf("Unmarshal() failed: %v", err)
		}
		return project.Key(), nil
	}
	return "", nil
}

// setProjectRevisions sets the current project revision from the master for
// each project as found on the filesystem
func setProjectRevisions(jirix *jiri.X, projects Projects) (_ Projects, e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return jirix.Run().Chdir(cwd) }, &e)

	for name, project := range projects {
		switch project.Protocol {
		case "git":
			if err := jirix.Run().Chdir(project.Path); err != nil {
				return nil, err
			}
			revision, err := jirix.Git().CurrentRevisionOfBranch("master")
			if err != nil {
				return nil, err
			}
			project.Revision = revision
		default:
			return nil, UnsupportedProtocolErr(project.Protocol)
		}
		projects[name] = project
	}
	return projects, nil
}

// LocalProjects returns projects on the local filesystem.  If all projects in
// the manifest exist locally and scanMode is set to FastScan, then only the
// projects in the manifest that exist locally will be returned.  Otherwise, a
// full scan of the filesystem will take place, and all found projects will be
// returned.
func LocalProjects(jirix *jiri.X, scanMode ScanMode) (Projects, error) {
	jirix.TimerPush("local projects")
	defer jirix.TimerPop()

	if scanMode == FastScan {
		// Fast path:  Full scan was not requested, and all projects in
		// manifest exist on local filesystem.  We just use the projects
		// directly from the manifest.
		manifestProjects, _, err := ReadManifest(jirix)
		if err != nil {
			return nil, err
		}
		projectsExist, err := projectsExistLocally(jirix, manifestProjects)
		if err != nil {
			return nil, err
		}
		if projectsExist {
			return setProjectRevisions(jirix, manifestProjects)
		}
	}

	// Slow path: Either full scan was not requested, or projects exist in
	// manifest that were not found locally.  Do a recursive scan of all projects
	// under JIRI_ROOT.
	projects := Projects{}
	jirix.TimerPush("scan fs")
	err := findLocalProjects(jirix, jirix.Root, projects)
	jirix.TimerPop()
	if err != nil {
		return nil, err
	}
	return setProjectRevisions(jirix, projects)
}

// projectsExistLocally returns true iff all the given projects exist on the
// local filesystem.
// Note that this may return true even if there are projects on the local
// filesystem not included in the provided projects argument.
func projectsExistLocally(jirix *jiri.X, projects Projects) (bool, error) {
	jirix.TimerPush("match manifest")
	defer jirix.TimerPop()
	for _, p := range projects {
		isLocal, err := isLocalProject(jirix, p.Path)
		if err != nil {
			return false, err
		}
		if !isLocal {
			return false, nil
		}
	}
	return true, nil
}

// PollProjects returns the set of changelists that exist remotely but not
// locally. Changes are grouped by projects and contain author identification
// and a description of their content.
func PollProjects(jirix *jiri.X, projectSet map[string]struct{}) (_ Update, e error) {
	jirix.TimerPush("poll projects")
	defer jirix.TimerPop()

	// Switch back to current working directory when we're done.
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	defer collect.Error(func() error { return jirix.Run().Chdir(cwd) }, &e)

	// Gather local & remote project data.
	localProjects, err := LocalProjects(jirix, FastScan)
	if err != nil {
		return nil, err
	}
	_, remoteProjects, _, _, err := readManifest(jirix, false)
	if err != nil {
		return nil, err
	}

	// Compute difference between local and remote.
	update := Update{}
	ops, err := computeOperations(localProjects, remoteProjects, false)
	if err != nil {
		return nil, err
	}

	for _, op := range ops {
		name := op.Project().Name

		// If given a project set, limit our results to those projects in the set.
		if len(projectSet) > 0 {
			if _, ok := projectSet[name]; !ok {
				continue
			}
		}

		// We only inspect this project if an update operation is required.
		cls := []CL{}
		if updateOp, ok := op.(updateOperation); ok {
			switch updateOp.project.Protocol {
			case "git":

				// Enter project directory - this assumes absolute paths.
				if err := jirix.Run().Chdir(updateOp.destination); err != nil {
					return nil, err
				}

				// Fetch the latest from origin.
				if err := jirix.Git().FetchRefspec("origin", updateOp.project.RemoteBranch); err != nil {
					return nil, err
				}

				// Collect commits visible from FETCH_HEAD that aren't visible from master.
				commitsText, err := jirix.Git().Log("FETCH_HEAD", "master", "%an%n%ae%n%B")
				if err != nil {
					return nil, err
				}

				// Format those commits and add them to the results.
				for _, commitText := range commitsText {
					if got, want := len(commitText), 3; got < want {
						return nil, fmt.Errorf("Unexpected length of %v: got %v, want at least %v", commitText, got, want)
					}
					cls = append(cls, CL{
						Author:      commitText[0],
						Email:       commitText[1],
						Description: strings.Join(commitText[2:], "\n"),
					})
				}
			default:
				return nil, UnsupportedProtocolErr(updateOp.project.Protocol)
			}
		}
		update[name] = cls
	}
	return update, nil
}

// ReadManifest retrieves and parses the manifest that determines what
// projects and tools are part of the jiri universe.
func ReadManifest(jirix *jiri.X) (Projects, Tools, error) {
	_, p, t, _, e := readManifest(jirix, false)
	return p, t, e
}

// getManifestRemote returns the remote url of the origin from the manifest
// repo.
// TODO(nlacasse,toddw): Once the manifest project is specified in the
// manifest, we should get the remote directly from the manifest, and not from
// the filesystem.
func getManifestRemote(jirix *jiri.X, manifestPath string) (string, error) {
	var remote string
	return remote, jirix.NewSeq().Pushd(manifestPath).Call(
		func() (e error) {
			remote, e = jirix.Git().RemoteUrl("origin")
			return
		}, "get manifest origin").Done()
}

// readManifest implements the ReadManifest logic and provides an
// optional flag that can be used to fetch the latest manifest updates
// from the manifest repository.
func readManifest(jirix *jiri.X, update bool) (Hosts, Projects, Tools, Hooks, error) {
	jirix.TimerPush("read manifest")
	defer jirix.TimerPop()
	if update {
		manifestPath := toAbs(jirix, ".manifest")
		manifestRemote, err := getManifestRemote(jirix, manifestPath)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		project := Project{
			Path:         manifestPath,
			Protocol:     "git",
			Remote:       manifestRemote,
			Revision:     "HEAD",
			RemoteBranch: "master",
		}
		if err := resetProject(jirix, project); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	path, err := jirix.ResolveManifestPath(jirix.Manifest())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	hosts, projects, tools, hooks, stack := Hosts{}, Projects{}, Tools{}, Hooks{}, map[string]struct{}{}
	if err := loadManifest(jirix, path, hosts, projects, tools, hooks, stack); err != nil {
		return nil, nil, nil, nil, err
	}
	return hosts, projects, tools, hooks, nil
}

// UpdateUniverse updates all local projects and tools to match the
// remote counterparts identified by the given manifest. Optionally,
// the 'gc' flag can be used to indicate that local projects that no
// longer exist remotely should be removed.
func UpdateUniverse(jirix *jiri.X, gc bool) (e error) {
	jirix.TimerPush("update universe")
	defer jirix.TimerPop()
	_, remoteProjects, remoteTools, remoteHooks, err := readManifest(jirix, true)
	if err != nil {
		return err
	}
	// 1. Update all local projects to match their remote counterparts.
	if err := updateProjects(jirix, remoteProjects, gc); err != nil {
		return err
	}
	// 2. Build all tools in a temporary directory.
	tmpDir, err := jirix.Run().TempDir("", "tmp-jiri-tools-build")
	if err != nil {
		return fmt.Errorf("TempDir() failed: %v", err)
	}
	defer collect.Error(func() error { return jirix.Run().RemoveAll(tmpDir) }, &e)
	if err := buildToolsFromMaster(jirix, remoteTools, tmpDir); err != nil {
		return err
	}
	// 3. Install the tools into $JIRI_ROOT/.jiri_root/bin.
	if err := InstallTools(jirix, tmpDir); err != nil {
		return err
	}
	// 4. Run all specified hooks
	return runHooks(jirix, remoteHooks)
}

// ApplyToLocalMaster applies an operation expressed as the given function to
// the local master branch of the given projects.
func ApplyToLocalMaster(jirix *jiri.X, projects Projects, fn func() error) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return jirix.Run().Chdir(cwd) }, &e)

	// Loop through all projects, checking out master and stashing any unstaged
	// changes.
	for _, project := range projects {
		p := project
		if err := jirix.Run().Chdir(p.Path); err != nil {
			return err
		}
		switch p.Protocol {
		case "git":
			branch, err := jirix.Git().CurrentBranchName()
			if err != nil {
				return err
			}
			stashed, err := jirix.Git().Stash()
			if err != nil {
				return err
			}
			if err := jirix.Git().CheckoutBranch("master"); err != nil {
				return err
			}
			// After running the function, return to this project's directory,
			// checkout the original branch, and stash pop if necessary.
			defer collect.Error(func() error {
				if err := jirix.Run().Chdir(p.Path); err != nil {
					return err
				}
				if err := jirix.Git().CheckoutBranch(branch); err != nil {
					return err
				}
				if stashed {
					return jirix.Git().StashPop()
				}
				return nil
			}, &e)
		default:
			return UnsupportedProtocolErr(p.Protocol)
		}
	}
	return fn()
}

// BuildTools builds the given tools and places the resulting binaries into the
// given directory.
func BuildTools(jirix *jiri.X, tools Tools, outputDir string) error {
	jirix.TimerPush("build tools")
	defer jirix.TimerPop()
	if len(tools) == 0 {
		// Nothing to do here...
		return nil
	}
	projects, err := LocalProjects(jirix, FastScan)
	if err != nil {
		return err
	}
	toolPkgs := []string{}
	workspaceSet := map[string]bool{}
	for _, tool := range tools {
		toolPkgs = append(toolPkgs, tool.Package)
		toolProject, err := projects.FindUnique(tool.Project)
		if err != nil {
			return err
		}
		// Identify the Go workspace the tool is in. To this end we use a
		// heuristic that identifies the maximal suffix of the project path
		// that corresponds to a prefix of the package name.
		workspace := ""
		for i := 0; i < len(toolProject.Path); i++ {
			if toolProject.Path[i] == filepath.Separator {
				if strings.HasPrefix("src/"+tool.Package, filepath.ToSlash(toolProject.Path[i+1:])) {
					workspace = toolProject.Path[:i]
					break
				}
			}
		}
		if workspace == "" {
			return fmt.Errorf("could not identify go workspace for tool %v", tool.Name)
		}
		workspaceSet[workspace] = true
	}
	workspaces := []string{}
	for workspace := range workspaceSet {
		workspaces = append(workspaces, workspace)
	}
	if envGoPath := os.Getenv("GOPATH"); envGoPath != "" {
		workspaces = append(workspaces, strings.Split(envGoPath, string(filepath.ListSeparator))...)
	}
	var stderr bytes.Buffer
	opts := jirix.Run().Opts()
	// We unset GOARCH and GOOS because jiri update should always build for the
	// native architecture and OS.  Also, as of go1.5, setting GOBIN is not
	// compatible with GOARCH or GOOS.
	opts.Env = map[string]string{
		"GOARCH": "",
		"GOOS":   "",
		"GOBIN":  outputDir,
		"GOPATH": strings.Join(workspaces, string(filepath.ListSeparator)),
	}
	opts.Stdout = ioutil.Discard
	opts.Stderr = &stderr
	args := append([]string{"install"}, toolPkgs...)
	if err := jirix.Run().CommandWithOpts(opts, "go", args...); err != nil {
		return fmt.Errorf("tool build failed\n%v", stderr.String())
	}
	return nil
}

// buildToolsFromMaster builds and installs all jiri tools using the version
// available in the local master branch of the tools repository. Notably, this
// function does not perform any version control operation on the master
// branch.
func buildToolsFromMaster(jirix *jiri.X, tools Tools, outputDir string) error {
	localProjects, err := LocalProjects(jirix, FastScan)
	if err != nil {
		return err
	}
	failed := false

	toolsToBuild, toolProjects := Tools{}, Projects{}
	toolNames := []string{} // Used for logging purposes.
	for _, tool := range tools {
		// Skip tools with no package specified. Besides increasing
		// robustness, this step also allows us to create jiri root
		// fakes without having to provide an implementation for the "jiri"
		// tool, which every manifest needs to specify.
		if tool.Package == "" {
			continue
		}
		project, err := localProjects.FindUnique(tool.Project)
		if err != nil {
			return err
		}
		toolProjects[project.Key()] = project
		toolsToBuild[tool.Name] = tool
		toolNames = append(toolNames, tool.Name)
	}

	updateFn := func() error {
		return ApplyToLocalMaster(jirix, toolProjects, func() error {
			return BuildTools(jirix, toolsToBuild, outputDir)
		})
	}

	// Always log the output of updateFn, irrespective of
	// the value of the verbose flag.
	opts := runutil.Opts{Verbose: true}
	if err := jirix.Run().FunctionWithOpts(opts, updateFn, "build tools: %v", strings.Join(toolNames, " ")); err != nil {
		fmt.Fprintf(jirix.Stderr(), "%v\n", err)
		failed = true
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}

// CleanupProjects restores the given jiri projects back to their master
// branches and gets rid of all the local changes. If "cleanupBranches" is
// true, it will also delete all the non-master branches.
func CleanupProjects(jirix *jiri.X, projects Projects, cleanupBranches bool) (e error) {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("Getwd() failed: %v", err)
	}
	defer collect.Error(func() error { return jirix.Run().Chdir(wd) }, &e)
	for _, project := range projects {
		localProjectDir := project.Path
		if err := jirix.Run().Chdir(localProjectDir); err != nil {
			return err
		}
		if err := resetLocalProject(jirix, cleanupBranches, project.RemoteBranch); err != nil {
			return err
		}
	}
	return nil
}

// resetLocalProject checks out the master branch, cleans up untracked files
// and uncommitted changes, and optionally deletes all the other branches.
func resetLocalProject(jirix *jiri.X, cleanupBranches bool, remoteBranch string) error {
	// Check out master and clean up changes.
	curBranchName, err := jirix.Git().CurrentBranchName()
	if err != nil {
		return err
	}
	if curBranchName != "master" {
		if err := jirix.Git().CheckoutBranch("master", gitutil.ForceOpt(true)); err != nil {
			return err
		}
	}
	if err := jirix.Git().RemoveUntrackedFiles(); err != nil {
		return err
	}
	// Discard any uncommitted changes.
	if remoteBranch == "" {
		remoteBranch = "master"
	}
	if err := jirix.Git().Reset("origin/" + remoteBranch); err != nil {
		return err
	}

	// Delete all the other branches.
	// At this point we should be at the master branch.
	branches, _, err := jirix.Git().GetBranches()
	if err != nil {
		return err
	}
	for _, branch := range branches {
		if branch == "master" {
			continue
		}
		if cleanupBranches {
			if err := jirix.Git().DeleteBranch(branch, gitutil.ForceOpt(true)); err != nil {
				return nil
			}
		}
	}

	return nil
}

// isLocalProject returns true if there is a project at the given path.
func isLocalProject(jirix *jiri.X, path string) (bool, error) {
	absPath := toAbs(jirix, path)
	// Existence of a metadata directory is how we know we've found a
	// Jiri-maintained project.
	metadataDir := filepath.Join(absPath, jiri.ProjectMetaDir)
	if _, err := jirix.Run().Stat(metadataDir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// projectAtPath returns a Project struct corresponding to the project at the
// path in the filesystem.
func projectAtPath(jirix *jiri.X, path string) (Project, error) {
	var project Project
	absPath := toAbs(jirix, path)
	metadataFile := filepath.Join(absPath, jiri.ProjectMetaDir, jiri.ProjectMetaFile)
	bytes, err := jirix.Run().ReadFile(metadataFile)
	if err != nil {
		return project, err
	}
	if err := xml.Unmarshal(bytes, &project); err != nil {
		return project, fmt.Errorf("Unmarshal() failed: %v\n%s", err, string(bytes))
	}
	project.Path = toAbs(jirix, project.Path)
	return project, nil
}

// findLocalProjects scans the filesystem for all projects.  Note that project
// directories can be nested recursively.
func findLocalProjects(jirix *jiri.X, path string, projects Projects) error {
	absPath := toAbs(jirix, path)
	isLocal, err := isLocalProject(jirix, absPath)
	if err != nil {
		return err
	}
	if isLocal {
		project, err := projectAtPath(jirix, absPath)
		if err != nil {
			return err
		}
		if absPath != project.Path {
			return fmt.Errorf("project %v has path %v but was found in %v", project.Name, project.Path, absPath)
		}
		if p, ok := projects[project.Key()]; ok {
			return fmt.Errorf("name conflict: both %v and %v contain project with key %v", p.Path, project.Path, project.Key())
		}
		projects[project.Key()] = project
	}

	// Recurse into all the sub directories.
	fileInfos, err := jirix.Run().ReadDir(path)
	if err != nil {
		return err
	}
	for _, fileInfo := range fileInfos {
		if fileInfo.IsDir() && !strings.HasPrefix(fileInfo.Name(), ".") {
			if err := findLocalProjects(jirix, filepath.Join(path, fileInfo.Name()), projects); err != nil {
				return err
			}
		}
	}
	return nil
}

// InstallTools installs the tools from the given directory into
// $JIRI_ROOT/.jiri_root/bin.
func InstallTools(jirix *jiri.X, dir string) error {
	jirix.TimerPush("install tools")
	defer jirix.TimerPop()
	if jirix.DryRun() {
		// In "dry run" mode, no binaries are built.
		return nil
	}
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("ReadDir(%v) failed: %v", dir, err)
	}
	binDir := jirix.BinDir()
	if err := jirix.NewSeq().MkdirAll(binDir, 0755).Done(); err != nil {
		return fmt.Errorf("MkdirAll(%v) failed: %v", binDir, err)
	}
	failed := false
	for _, fi := range fis {
		installFn := func() error {
			src := filepath.Join(dir, fi.Name())
			dst := filepath.Join(binDir, fi.Name())
			return jirix.Run().Rename(src, dst)
		}
		opts := runutil.Opts{Verbose: true}
		if err := jirix.Run().FunctionWithOpts(opts, installFn, "install tool %q", fi.Name()); err != nil {
			fmt.Fprintf(jirix.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	return nil
}

// TransitionBinDir handles the transition from the old location
// $JIRI_ROOT/devtools/bin to the new $JIRI_ROOT/.jiri_root/bin.  In
// InstallTools above we've already installed the tools to the new location.
//
// For now we want $JIRI_ROOT/devtools/bin symlinked to the new location, so
// that users won't perceive a difference in behavior.  In addition, we want to
// save the old binaries to $JIRI_ROOT/.jiri_root/bin.BACKUP the first time this
// is run.  That way if we screwed something up, the user can recover their old
// binaries.
//
// TODO(toddw): Remove this logic after the transition to .jiri_root is done.
func TransitionBinDir(jirix *jiri.X) error {
	oldDir, newDir := filepath.Join(jirix.Root, "devtools", "bin"), jirix.BinDir()
	switch info, err := jirix.Run().Lstat(oldDir); {
	case os.IsNotExist(err):
		// Drop down to create the symlink below.
	case err != nil:
		return fmt.Errorf("Failed to stat old bin dir: %v", err)
	case info.Mode()&os.ModeSymlink != 0:
		link, err := jirix.NewSeq().Readlink(oldDir)
		if err != nil {
			return fmt.Errorf("Failed to read link from old bin dir: %v", err)
		}
		if filepath.Clean(link) == newDir {
			// The old dir is already correctly symlinked to the new dir.
			return nil
		}
		fallthrough
	default:
		// The old dir exists, and either it's not a symlink, or it's a symlink that
		// doesn't point to the new dir.  Move the old dir to the backup location.
		backupDir := newDir + ".BACKUP"
		switch _, err := jirix.Run().Stat(backupDir); {
		case os.IsNotExist(err):
			if err := jirix.NewSeq().Rename(oldDir, backupDir).Done(); err != nil {
				return fmt.Errorf("Failed to backup old bin dir %v to %v: %v", oldDir, backupDir, err)
			}
			// Drop down to create the symlink below.
		case err != nil:
			return fmt.Errorf("Failed to stat backup bin dir: %v", err)
		default:
			return fmt.Errorf("Backup bin dir %v already exists", backupDir)
		}
	}
	// Create the symlink.
	if err := jirix.NewSeq().MkdirAll(filepath.Dir(oldDir), 0755).Symlink(newDir, oldDir).Done(); err != nil {
		return fmt.Errorf("Failed to symlink to new bin dir %v from %v: %v", newDir, oldDir, err)
	}
	return nil
}

// runHooks runs the specified hooks
func runHooks(jirix *jiri.X, hooks Hooks) error {
	jirix.TimerPush("run hooks")
	defer jirix.TimerPop()
	for _, hook := range hooks {
		command := hook.Path
		args := []string{}
		if hook.Interpreter != "" {
			command = hook.Interpreter
			args = append(args, hook.Path)
		}
		for _, arg := range hook.Args {
			args = append(args, arg.Arg)
		}
		if err := jirix.Run().Command(command, args...); err != nil {
			return fmt.Errorf("Hook %v failed: %v command: %v args: %v", hook.Name, err, command, args)
		}
	}
	return nil
}

// resetProject advances the local master branch of the given
// project, which is expected to exist locally at project.Path.
func resetProject(jirix *jiri.X, project Project) error {
	fn := func() error {
		switch project.Protocol {
		case "git":
			if project.Remote == "" {
				return fmt.Errorf("project %v does not have a remote", project.Name)
			}
			if err := jirix.Git().SetRemoteUrl("origin", project.Remote); err != nil {
				return err
			}
			if err := jirix.Git().Fetch("origin"); err != nil {
				return err
			}

			// Having a specific revision trumps everything else - once fetched,
			// always reset to that revision.
			if project.Revision != "" && project.Revision != "HEAD" {
				return jirix.Git().Reset(project.Revision)
			}

			// If no revision, reset to the configured remote branch, or master
			// if no remote branch.
			remoteBranch := project.RemoteBranch
			if remoteBranch == "" {
				remoteBranch = "master"
			}
			return jirix.Git().Reset("origin/" + remoteBranch)
		default:
			return UnsupportedProtocolErr(project.Protocol)
		}
	}
	return ApplyToLocalMaster(jirix, Projects{project.Key(): project}, fn)
}

// loadManifest loads the given manifest, processing all of its
// imports, projects and tools settings.
func loadManifest(jirix *jiri.X, path string, hosts Hosts, projects Projects, tools Tools, hooks Hooks, stack map[string]struct{}) error {
	data, err := jirix.Run().ReadFile(path)
	if err != nil {
		return err
	}
	m := &Manifest{}
	if err := xml.Unmarshal(data, m); err != nil {
		return fmt.Errorf("Unmarshal(%v) failed: %v", string(data), err)
	}
	// Process all imports.
	for _, manifest := range m.Imports {
		if _, ok := stack[manifest.Name]; ok {
			return fmt.Errorf("import cycle encountered")
		}
		path, err := jirix.ResolveManifestPath(manifest.Name)
		if err != nil {
			return err
		}
		stack[manifest.Name] = struct{}{}
		if err := loadManifest(jirix, path, hosts, projects, tools, hooks, stack); err != nil {
			return err
		}
		delete(stack, manifest.Name)
	}
	// Process all projects.
	for _, project := range m.Projects {
		if strings.Contains(project.Name, projectKeySeparator) {
			return fmt.Errorf("project name cannot contain %q: %q", projectKeySeparator, project.Name)
		}
		if project.Exclude {
			// Exclude the project in case it was
			// previously included.
			delete(projects, project.Key())
			continue
		}
		// Replace the relative path with an absolute one.
		project.Path = toAbs(jirix, project.Path)
		// Use git as the default protocol.
		if project.Protocol == "" {
			project.Protocol = "git"
		}
		// Use HEAD and tip as the default revision for git
		// and mercurial respectively.
		if project.Revision == "" {
			switch project.Protocol {
			case "git":
				project.Revision = "HEAD"
			default:
				return UnsupportedProtocolErr(project.Protocol)
			}
		}
		// Default to "master" branch if none is provided.
		if project.RemoteBranch == "" {
			project.RemoteBranch = "master"
		}
		projects[project.Key()] = project
	}
	// Process all tools.
	for _, tool := range m.Tools {
		if tool.Exclude {
			// Exclude the tool in case it was previously
			// included.
			delete(tools, tool.Name)
			continue
		}
		// Use <JiriProject> as the default project.
		if tool.Project == "" {
			tool.Project = "https://vanadium.googlesource.com/" + JiriProject
		}
		// Use "data" as the default data.
		if tool.Data == "" {
			tool.Data = "data"
		}
		tools[tool.Name] = tool
	}
	// Process all hooks.
	for _, hook := range m.Hooks {
		if hook.Exclude {
			// Exclude the hook in case it was previously
			// included.
			delete(hooks, hook.Name)
			continue
		}
		project, err := projects.FindUnique(hook.Project)
		if err != nil {
			return fmt.Errorf("error while finding project %q for hook %q: %v", hook.Project, hook.Name, err)
		}
		// Replace project-relative path with absolute path.
		hook.Path = filepath.Join(project.Path, hook.Path)
		hooks[hook.Name] = hook
	}
	// Process all hosts.
	for _, host := range m.Hosts {
		hosts[host.Name] = host

		// Sanity check that we only have githooks for git hosts.
		if host.Name != "git" {
			if len(host.GitHooks) > 0 {
				return fmt.Errorf("githook provided for a non-Git host: %s", host.Location)
			}
		}
	}

	return nil
}

// reportNonMaster checks if the given project is on master branch and
// if not, reports this fact along with information on how to update it.
func reportNonMaster(jirix *jiri.X, project Project) (e error) {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return jirix.Run().Chdir(cwd) }, &e)
	if err := jirix.Run().Chdir(project.Path); err != nil {
		return err
	}
	switch project.Protocol {
	case "git":
		current, err := jirix.Git().CurrentBranchName()
		if err != nil {
			return err
		}
		if current != "master" {
			line1 := fmt.Sprintf(`NOTE: "jiri update" only updates the "master" branch and the current branch is %q`, current)
			line2 := fmt.Sprintf(`to update the %q branch once the master branch is updated, run "git merge master"`, current)
			opts := runutil.Opts{Verbose: true}
			jirix.Run().OutputWithOpts(opts, []string{line1, line2})
		}
		return nil
	default:
		return UnsupportedProtocolErr(project.Protocol)
	}
}

// getRemoteHeadRevisions attempts to get the repo statuses from remote for HEAD
// projects so we can detect when a local project is already up-to-date.
func getRemoteHeadRevisions(jirix *jiri.X, remoteProjects Projects) {
	someAtHead := false
	for _, rp := range remoteProjects {
		if rp.Revision == "HEAD" {
			someAtHead = true
			break
		}
	}
	if !someAtHead {
		return
	}
	gitHost, gitHostErr := GitHost(jirix)
	if gitHostErr != nil || !googlesource.IsGoogleSourceHost(gitHost) {
		return
	}
	repoStatuses, err := googlesource.GetRepoStatuses(jirix.Context, gitHost)
	if err != nil {
		// Log the error but don't fail.
		fmt.Fprintf(jirix.Stderr(), "Error fetching repo statuses from remote: %v\n", err)
		return
	}
	for name, rp := range remoteProjects {
		if rp.Revision != "HEAD" {
			continue
		}
		status, ok := repoStatuses[rp.Name]
		if !ok {
			continue
		}
		masterRev, ok := status.Branches["master"]
		if !ok || masterRev == "" {
			continue
		}
		rp.Revision = masterRev
		remoteProjects[name] = rp
	}
}

func updateProjects(jirix *jiri.X, remoteProjects Projects, gc bool) error {
	jirix.TimerPush("update projects")
	defer jirix.TimerPop()

	scanMode := FastScan
	if gc {
		scanMode = FullScan
	}
	localProjects, err := LocalProjects(jirix, scanMode)
	if err != nil {
		return err
	}
	getRemoteHeadRevisions(jirix, remoteProjects)
	ops, err := computeOperations(localProjects, remoteProjects, gc)
	if err != nil {
		return err
	}

	updates := newFsUpdates()
	for _, op := range ops {
		if err := op.Test(jirix, updates); err != nil {
			return err
		}
	}
	failed := false
	manifest := &Manifest{Label: jirix.Manifest()}
	for _, op := range ops {
		updateFn := func() error { return op.Run(jirix, manifest) }
		// Always log the output of updateFn, irrespective of
		// the value of the verbose flag.
		opts := runutil.Opts{Verbose: true}
		if err := jirix.Run().FunctionWithOpts(opts, updateFn, "%v", op); err != nil {
			fmt.Fprintf(jirix.Stderr(), "%v\n", err)
			failed = true
		}
	}
	if failed {
		return cmdline.ErrExitCode(2)
	}
	if err := writeCurrentManifest(jirix, manifest); err != nil {
		return err
	}
	return nil
}

// writeMetadata stores the given project metadata in the directory
// identified by the given path.
func writeMetadata(jirix *jiri.X, project Project, dir string) (e error) {
	metadataDir := filepath.Join(dir, jiri.ProjectMetaDir)
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return jirix.Run().Chdir(cwd) }, &e)
	if err := jirix.Run().MkdirAll(metadataDir, os.FileMode(0755)); err != nil {
		return err
	}
	if err := jirix.Run().Chdir(metadataDir); err != nil {
		return err
	}
	// Replace absolute project paths with relative paths to make it
	// possible to move the $JIRI_ROOT directory locally.
	relPath, err := toRel(jirix, project.Path)
	if err != nil {
		return err
	}
	project.Path = relPath
	bytes, err := xml.Marshal(project)
	if err != nil {
		return fmt.Errorf("Marhsal() failed: %v", err)
	}
	metadataFile := filepath.Join(metadataDir, jiri.ProjectMetaFile)
	tmpMetadataFile := metadataFile + ".tmp"
	if err := jirix.Run().WriteFile(tmpMetadataFile, bytes, os.FileMode(0644)); err != nil {
		return err
	}
	if err := jirix.Run().Rename(tmpMetadataFile, metadataFile); err != nil {
		return err
	}
	return nil
}

// addProjectToManifest records the information about the given
// project in the given manifest. The function is used to create a
// manifest that records the current state of jiri projects, which
// can be used to restore this state at some later point.
//
// NOTE: The function assumes that the the given project is on a
// master branch.
func addProjectToManifest(jirix *jiri.X, manifest *Manifest, project Project) error {
	// If the project uses relative revision, replace it with an absolute one.
	switch project.Protocol {
	case "git":
		if project.Revision == "HEAD" {
			revision, err := jirix.Git(tool.RootDirOpt(project.Path)).CurrentRevision()
			if err != nil {
				return err
			}
			project.Revision = revision
		}
	default:
		return UnsupportedProtocolErr(project.Protocol)
	}
	relPath, err := toRel(jirix, project.Path)
	if err != nil {
		return err
	}
	project.Path = relPath
	manifest.Projects = append(manifest.Projects, project)
	return nil
}

// fsUpdates is used to track filesystem updates made by operations.
// TODO(nlacasse): Currently we only use fsUpdates to track deletions so that
// jiri can delete and create a project in the same directory in one update.
// There are lots of other cases that should be covered though, like detecting
// when two projects would be created in the same directory.
type fsUpdates struct {
	deletedDirs map[string]bool
}

func newFsUpdates() *fsUpdates {
	return &fsUpdates{
		deletedDirs: map[string]bool{},
	}
}

func (u *fsUpdates) deleteDir(dir string) {
	dir = filepath.Clean(dir)
	u.deletedDirs[dir] = true
}

func (u *fsUpdates) isDeleted(dir string) bool {
	_, ok := u.deletedDirs[filepath.Clean(dir)]
	return ok
}

type operation interface {
	// Project identifies the project this operation pertains to.
	Project() Project
	// Run executes the operation.
	Run(jirix *jiri.X, manifest *Manifest) error
	// String returns a string representation of the operation.
	String() string
	// Test checks whether the operation would fail.
	Test(jirix *jiri.X, updates *fsUpdates) error
}

// commonOperation represents a project operation.
type commonOperation struct {
	// project holds information about the project such as its
	// name, local path, and the protocol it uses for version
	// control.
	project Project
	// destination is the new project path.
	destination string
	// source is the current project path.
	source string
}

func (op commonOperation) Project() Project {
	return op.project
}

// createOperation represents the creation of a project.
type createOperation struct {
	commonOperation
}

func (op createOperation) Run(jirix *jiri.X, manifest *Manifest) (e error) {
	hosts, _, _, _, err := readManifest(jirix, false)
	if err != nil {
		return err
	}

	path, perm := filepath.Dir(op.destination), os.FileMode(0755)
	if err := jirix.Run().MkdirAll(path, perm); err != nil {
		return err
	}
	// Create a temporary directory for the initial setup of the
	// project to prevent an untimely termination from leaving the
	// $JIRI_ROOT directory in an inconsistent state.
	tmpDirPrefix := strings.Replace(op.Project().Name, "/", ".", -1) + "-"
	tmpDir, err := jirix.Run().TempDir(path, tmpDirPrefix)
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return jirix.Run().RemoveAll(tmpDir) }, &e)
	switch op.project.Protocol {
	case "git":
		if err := jirix.Git().Clone(op.project.Remote, tmpDir); err != nil {
			return err
		}

		// Apply git hooks.  We're creating this repo, so there's no danger of
		// overriding existing hooks.  Customizing your git hooks with jiri is a bad
		// idea anyway, since jiri won't know to not delete the project when you
		// switch between manifests or do a cleanup.
		host, found := hosts["git"]
		if found && strings.HasPrefix(op.project.Remote, host.Location) {
			gitHookDir := filepath.Join(tmpDir, ".git", "hooks")
			for _, githook := range host.GitHooks {
				mdir := jirix.ManifestDir()
				src, err := jirix.Run().ReadFile(filepath.Join(mdir, githook.Path))
				if err != nil {
					return err
				}
				dst := filepath.Join(gitHookDir, githook.Name)
				if err := jirix.Run().WriteFile(dst, src, perm); err != nil {
					return err
				}
			}
		}

		// Apply exclusion for /.jiri/. We're creating the repo so we can safely
		// write to .git/info/exclude
		excludeString := "/.jiri/\n"
		excludeDir := filepath.Join(tmpDir, ".git", "info")
		if err := jirix.Run().MkdirAll(excludeDir, os.FileMode(0750)); err != nil {
			return err
		}
		excludeFile := filepath.Join(excludeDir, "exclude")
		if err := jirix.Run().WriteFile(excludeFile, []byte(excludeString), perm); err != nil {
			return err
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return jirix.Run().Chdir(cwd) }, &e)
		if err := jirix.Run().Chdir(tmpDir); err != nil {
			return err
		}
		if err := jirix.Git().Reset(op.project.Revision); err != nil {
			return err
		}
	default:
		return UnsupportedProtocolErr(op.project.Protocol)
	}
	if err := writeMetadata(jirix, op.project, tmpDir); err != nil {
		return err
	}
	if err := jirix.Run().Chmod(tmpDir, os.FileMode(0755)); err != nil {
		return err
	}
	if err := jirix.Run().Rename(tmpDir, op.destination); err != nil {
		return err
	}
	if err := resetProject(jirix, op.project); err != nil {
		return err
	}
	return addProjectToManifest(jirix, manifest, op.project)
}

func (op createOperation) String() string {
	return fmt.Sprintf("create project %q in %q and advance it to %q", op.project.Name, op.destination, fmtRevision(op.project.Revision))
}

func (op createOperation) Test(jirix *jiri.X, updates *fsUpdates) error {
	// Check the local file system.
	if _, err := jirix.Run().Stat(op.destination); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else if !updates.isDeleted(op.destination) {
		return fmt.Errorf("cannot create %q as it already exists", op.destination)
	}
	return nil
}

// deleteOperation represents the deletion of a project.
type deleteOperation struct {
	commonOperation
	// gc determines whether the operation should be executed or
	// whether it should only print a notification.
	gc bool
}

func (op deleteOperation) Run(jirix *jiri.X, _ *Manifest) error {
	if op.gc {
		// Never delete the <JiriProject>.
		if op.project.Name == JiriProject {
			lines := []string{
				fmt.Sprintf("NOTE: project %v was not found in the project manifest", op.project.Name),
				"however this project is required for correct operation of the jiri",
				"development tools and will thus not be deleted",
			}
			opts := runutil.Opts{Verbose: true}
			jirix.Run().OutputWithOpts(opts, lines)
			return nil
		}
		// Never delete projects with non-master branches, uncommitted
		// work, or untracked content.
		git := jirix.Git(tool.RootDirOpt(op.project.Path))
		branches, _, err := git.GetBranches()
		if err != nil {
			return err
		}
		uncommitted, err := git.HasUncommittedChanges()
		if err != nil {
			return err
		}
		untracked, err := git.HasUntrackedFiles()
		if err != nil {
			return err
		}
		if len(branches) != 1 || uncommitted || untracked {
			lines := []string{
				fmt.Sprintf("NOTE: project %v was not found in the project manifest", op.project.Name),
				"however this project either contains non-master branches, uncommitted",
				"work, or untracked files and will thus not be deleted",
			}
			opts := runutil.Opts{Verbose: true}
			jirix.Run().OutputWithOpts(opts, lines)
			return nil
		}
		return jirix.Run().RemoveAll(op.source)
	}
	lines := []string{
		fmt.Sprintf("NOTE: project %v was not found in the project manifest", op.project.Name),
		"it was not automatically removed to avoid deleting uncommitted work",
		fmt.Sprintf(`if you no longer need it, invoke "rm -rf %v"`, op.source),
		`or invoke "jiri update -gc" to remove all such local projects`,
	}
	opts := runutil.Opts{Verbose: true}
	jirix.Run().OutputWithOpts(opts, lines)
	return nil
}

func (op deleteOperation) String() string {
	return fmt.Sprintf("delete project %q from %q", op.project.Name, op.source)
}

func (op deleteOperation) Test(jirix *jiri.X, updates *fsUpdates) error {
	if _, err := jirix.Run().Stat(op.source); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cannot delete %q as it does not exist", op.source)
		}
		return err
	}
	updates.deleteDir(op.source)
	return nil
}

// moveOperation represents the relocation of a project.
type moveOperation struct {
	commonOperation
}

func (op moveOperation) Run(jirix *jiri.X, manifest *Manifest) error {
	path, perm := filepath.Dir(op.destination), os.FileMode(0755)
	if err := jirix.Run().MkdirAll(path, perm); err != nil {
		return err
	}
	if err := jirix.Run().Rename(op.source, op.destination); err != nil {
		return err
	}
	if err := reportNonMaster(jirix, op.project); err != nil {
		return err
	}
	if err := resetProject(jirix, op.project); err != nil {
		return err
	}
	if err := writeMetadata(jirix, op.project, op.project.Path); err != nil {
		return err
	}
	return addProjectToManifest(jirix, manifest, op.project)
}

func (op moveOperation) String() string {
	return fmt.Sprintf("move project %q located in %q to %q and advance it to %q", op.project.Name, op.source, op.destination, fmtRevision(op.project.Revision))
}

func (op moveOperation) Test(jirix *jiri.X, updates *fsUpdates) error {
	if _, err := jirix.Run().Stat(op.source); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("cannot move %q to %q as the source does not exist", op.source, op.destination)
		}
		return err
	}
	if _, err := jirix.Run().Stat(op.destination); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		return fmt.Errorf("cannot move %q to %q as the destination already exists", op.source, op.destination)
	}
	updates.deleteDir(op.source)
	return nil
}

// updateOperation represents the update of a project.
type updateOperation struct {
	commonOperation
}

func (op updateOperation) Run(jirix *jiri.X, manifest *Manifest) error {
	if err := reportNonMaster(jirix, op.project); err != nil {
		return err
	}
	if err := resetProject(jirix, op.project); err != nil {
		return err
	}
	if err := writeMetadata(jirix, op.project, op.project.Path); err != nil {
		return err
	}
	return addProjectToManifest(jirix, manifest, op.project)
}

func (op updateOperation) String() string {
	return fmt.Sprintf("advance project %q located in %q to %q", op.project.Name, op.source, fmtRevision(op.project.Revision))
}

func (op updateOperation) Test(jirix *jiri.X, _ *fsUpdates) error {
	return nil
}

// nullOperation represents a noop.  It is used for logging and adding project
// information to the current manifest.
type nullOperation struct {
	commonOperation
}

func (op nullOperation) Run(jirix *jiri.X, manifest *Manifest) error {
	if err := writeMetadata(jirix, op.project, op.project.Path); err != nil {
		return err
	}
	return addProjectToManifest(jirix, manifest, op.project)
}

func (op nullOperation) String() string {
	return fmt.Sprintf("project %q located in %q at revision %q is up-to-date", op.project.Name, op.source, fmtRevision(op.project.Revision))
}

func (op nullOperation) Test(jirix *jiri.X, _ *fsUpdates) error {
	return nil
}

// operations is a sortable collection of operations
type operations []operation

// Len returns the length of the collection.
func (ops operations) Len() int {
	return len(ops)
}

// Less defines the order of operations. Operations are ordered first
// by their type and then by their project path.
//
// The order in which operation types are defined determines the order
// in which operations are performed. For correctness and also to
// minimize the chance of a conflict, the delete operations should
// happen before move operations, which should happen before create
// operations. If two create operations make nested directories, the
// outermost should be created first.
func (ops operations) Less(i, j int) bool {
	vals := make([]int, 2)
	for idx, op := range []operation{ops[i], ops[j]} {
		switch op.(type) {
		case deleteOperation:
			vals[idx] = 0
		case moveOperation:
			vals[idx] = 1
		case createOperation:
			vals[idx] = 2
		case updateOperation:
			vals[idx] = 3
		case nullOperation:
			vals[idx] = 4
		}
	}
	if vals[0] != vals[1] {
		return vals[0] < vals[1]
	}
	return ops[i].Project().Path < ops[j].Project().Path
}

// Swap swaps two elements of the collection.
func (ops operations) Swap(i, j int) {
	ops[i], ops[j] = ops[j], ops[i]
}

// computeOperations inputs a set of projects to update and the set of
// current and new projects (as defined by contents of the local file
// system and manifest file respectively) and outputs a collection of
// operations that describe the actions needed to update the target
// projects.
func computeOperations(localProjects, remoteProjects Projects, gc bool) (operations, error) {
	result := operations{}
	allProjects := map[ProjectKey]struct{}{}
	for _, p := range localProjects {
		allProjects[p.Key()] = struct{}{}
	}
	for _, p := range remoteProjects {
		allProjects[p.Key()] = struct{}{}
	}
	for key, _ := range allProjects {
		if localProject, ok := localProjects[key]; ok {
			if remoteProject, ok := remoteProjects[key]; ok {
				if localProject.Path != remoteProject.Path {
					// moveOperation also does an update, so we don't need to
					// check the revision here.
					result = append(result, moveOperation{commonOperation{
						destination: remoteProject.Path,
						project:     remoteProject,
						source:      localProject.Path,
					}})
				} else {
					if localProject.Revision != remoteProject.Revision {
						result = append(result, updateOperation{commonOperation{
							destination: remoteProject.Path,
							project:     remoteProject,
							source:      localProject.Path,
						}})
					} else {
						result = append(result, nullOperation{commonOperation{
							destination: remoteProject.Path,
							project:     remoteProject,
							source:      localProject.Path,
						}})
					}
				}
			} else {
				result = append(result, deleteOperation{commonOperation{
					destination: "",
					project:     localProject,
					source:      localProject.Path,
				}, gc})
			}
		} else if remoteProject, ok := remoteProjects[key]; ok {
			result = append(result, createOperation{commonOperation{
				destination: remoteProject.Path,
				project:     remoteProject,
				source:      "",
			}})
		} else {
			return nil, fmt.Errorf("project with key %v does not exist", key)
		}
	}
	sort.Sort(result)
	return result, nil
}

// ParseNames identifies the set of projects that a jiri command should
// be applied to.
func ParseNames(jirix *jiri.X, args []string, defaultProjects map[string]struct{}) (Projects, error) {
	manifestProjects, _, err := ReadManifest(jirix)
	if err != nil {
		return nil, err
	}
	result := Projects{}
	if len(args) == 0 {
		// Use the default set of projects.
		args = set.String.ToSlice(defaultProjects)
	}
	for _, name := range args {
		projects := manifestProjects.Find(name)
		if len(projects) == 0 {
			// Issue a warning if the target project does not exist in the
			// project manifest.
			fmt.Fprintf(jirix.Stderr(), "project %q does not exist in the project manifest", name)
		}
		for _, project := range projects {
			result[project.Key()] = project
		}
	}
	return result, nil
}

// fmtRevision returns the first 8 chars of a revision hash.
func fmtRevision(r string) string {
	l := 8
	if len(r) < l {
		return r
	}
	return r[:l]
}
