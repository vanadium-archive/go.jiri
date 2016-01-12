// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package project

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/url"
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
	Hooks       []Hook       `xml:"hooks>hook"`
	Imports     []Import     `xml:"imports>import"`
	FileImports []FileImport `xml:"imports>fileimport"`
	Label       string       `xml:"label,attr,omitempty"`
	Projects    []Project    `xml:"projects>project"`
	Tools       []Tool       `xml:"tools>tool"`
	XMLName     struct{}     `xml:"manifest"`
}

// ManifestFromBytes returns a manifest parsed from data, with defaults filled
// in.
func ManifestFromBytes(data []byte) (*Manifest, error) {
	m := new(Manifest)
	if err := xml.Unmarshal(data, m); err != nil {
		return nil, err
	}
	if err := m.fillDefaults(); err != nil {
		return nil, err
	}
	return m, nil
}

// ManifestFromFile returns a manifest parsed from the contents of filename,
// with defaults filled in.
func ManifestFromFile(jirix *jiri.X, filename string) (*Manifest, error) {
	data, err := jirix.NewSeq().ReadFile(filename)
	if err != nil {
		return nil, err
	}
	m, err := ManifestFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("invalid manifest %s: %v", filename, err)
	}
	return m, nil
}

var (
	newlineBytes       = []byte("\n")
	emptyHooksBytes    = []byte("\n  <hooks></hooks>\n")
	emptyImportsBytes  = []byte("\n  <imports></imports>\n")
	emptyProjectsBytes = []byte("\n  <projects></projects>\n")
	emptyToolsBytes    = []byte("\n  <tools></tools>\n")

	endElemBytes       = []byte("/>\n")
	endHookBytes       = []byte("></hook>\n")
	endImportBytes     = []byte("></import>\n")
	endFileImportBytes = []byte("></fileimport>\n")
	endProjectBytes    = []byte("></project>\n")
	endToolBytes       = []byte("></tool>\n")

	endImportSoloBytes  = []byte("></import>")
	endProjectSoloBytes = []byte("></project>")
	endElemSoloBytes    = []byte("/>")
)

// deepCopy returns a deep copy of Manifest.
func (m *Manifest) deepCopy() *Manifest {
	x := new(Manifest)
	x.Label = m.Label
	// First make copies of all slices.
	x.Hooks = append([]Hook(nil), m.Hooks...)
	x.Imports = append([]Import(nil), m.Imports...)
	x.FileImports = append([]FileImport(nil), m.FileImports...)
	x.Projects = append([]Project(nil), m.Projects...)
	x.Tools = append([]Tool(nil), m.Tools...)
	// Now make copies of sub-slices.
	for index, hook := range x.Hooks {
		x.Hooks[index].Args = append([]HookArg(nil), hook.Args...)
	}
	return x
}

// ToBytes returns m as serialized bytes, with defaults unfilled.
func (m *Manifest) ToBytes() ([]byte, error) {
	m = m.deepCopy() // avoid changing manifest when unfilling defaults.
	if err := m.unfillDefaults(); err != nil {
		return nil, err
	}
	data, err := xml.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("manifest xml.Marshal failed: %v", err)
	}
	// It's hard (impossible?) to get xml.Marshal to elide some of the empty
	// elements, or produce short empty elements, so we post-process the data.
	data = bytes.Replace(data, emptyHooksBytes, newlineBytes, -1)
	data = bytes.Replace(data, emptyImportsBytes, newlineBytes, -1)
	data = bytes.Replace(data, emptyProjectsBytes, newlineBytes, -1)
	data = bytes.Replace(data, emptyToolsBytes, newlineBytes, -1)
	data = bytes.Replace(data, endHookBytes, endElemBytes, -1)
	data = bytes.Replace(data, endImportBytes, endElemBytes, -1)
	data = bytes.Replace(data, endFileImportBytes, endElemBytes, -1)
	data = bytes.Replace(data, endProjectBytes, endElemBytes, -1)
	data = bytes.Replace(data, endToolBytes, endElemBytes, -1)
	if !bytes.HasSuffix(data, newlineBytes) {
		data = append(data, '\n')
	}
	return data, nil
}

func safeWriteFile(jirix *jiri.X, filename string, data []byte) error {
	tmp := filename + ".tmp"
	return jirix.NewSeq().
		MkdirAll(filepath.Dir(filename), 0755).
		WriteFile(tmp, data, 0644).
		Rename(tmp, filename).
		Done()
}

// ToFile writes the manifest m to a file with the given filename, with defaults
// unfilled.
func (m *Manifest) ToFile(jirix *jiri.X, filename string) error {
	data, err := m.ToBytes()
	if err != nil {
		return err
	}
	return safeWriteFile(jirix, filename, data)
}

func (m *Manifest) fillDefaults() error {
	for index := range m.Imports {
		if err := m.Imports[index].fillDefaults(); err != nil {
			return err
		}
	}
	for index := range m.FileImports {
		if err := m.FileImports[index].validate(); err != nil {
			return err
		}
	}
	for index := range m.Projects {
		if err := m.Projects[index].fillDefaults(); err != nil {
			return err
		}
	}
	for index := range m.Tools {
		if err := m.Tools[index].fillDefaults(); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manifest) unfillDefaults() error {
	for index := range m.Imports {
		if err := m.Imports[index].unfillDefaults(); err != nil {
			return err
		}
	}
	for index := range m.FileImports {
		if err := m.FileImports[index].validate(); err != nil {
			return err
		}
	}
	for index := range m.Projects {
		if err := m.Projects[index].unfillDefaults(); err != nil {
			return err
		}
	}
	for index := range m.Tools {
		if err := m.Tools[index].unfillDefaults(); err != nil {
			return err
		}
	}
	return nil
}

// Hooks maps hook names to their detailed description.
type Hooks map[string]Hook

// Hook represents a post-update project hook.
type Hook struct {
	// Name is the hook name.
	Name string `xml:"name,attr,omitempty"`
	// Project is the name of the project the hook is associated with.
	Project string `xml:"project,attr,omitempty"`
	// Path is the path of the hook relative to its project's root.
	Path string `xml:"path,attr,omitempty"`
	// Interpreter is an optional program used to interpret the hook (i.e. python). Unlike Path,
	// Interpreter is relative to the environment's PATH and not the project's root.
	Interpreter string `xml:"interpreter,attr,omitempty"`
	// Arguments for the hook.
	Args    []HookArg `xml:"arg,omitempty"`
	XMLName struct{}  `xml:"hook"`
}

type HookArg struct {
	Arg     string   `xml:",chardata"`
	XMLName struct{} `xml:"arg"`
}

// Import represents a remote manifest import.
type Import struct {
	// Manifest file to use from the remote manifest project.
	Manifest string `xml:"manifest,attr,omitempty"`
	// Root path, prepended to the manifest project path, as well as all projects
	// specified in the manifest file.
	Root string `xml:"root,attr,omitempty"`
	// Project description of the manifest repository.
	Project
	XMLName struct{} `xml:"import"`
}

// ToFile writes the import i to a file with the given filename, with defaults
// unfilled.
func (i Import) ToFile(jirix *jiri.X, filename string) error {
	if err := i.unfillDefaults(); err != nil {
		return err
	}
	data, err := xml.Marshal(i)
	if err != nil {
		return fmt.Errorf("import xml.Marshal failed: %v", err)
	}
	// Same logic as Manifest.ToBytes, to make the output more compact.
	data = bytes.Replace(data, endImportSoloBytes, endElemSoloBytes, -1)
	return safeWriteFile(jirix, filename, data)
}

func (i *Import) fillDefaults() error {
	if i.Remote != "" {
		if i.Path == "" {
			i.Path = "manifest"
		}
		if err := i.Project.fillDefaults(); err != nil {
			return err
		}
	}
	return i.validate()
}

func (i *Import) unfillDefaults() error {
	if i.Remote != "" {
		if i.Path == "manifest" {
			i.Path = ""
		}
		if err := i.Project.unfillDefaults(); err != nil {
			return err
		}
	}
	return i.validate()
}

func (i *Import) validate() error {
	// After our transition is done, the "import" element will always denote
	// remote imports, and the "remote" and "manifest" attributes will be
	// required.  During the transition we allow old-style local imports, which
	// only set the "name" attribute.
	//
	// This is a bit tricky, since the "name" attribute is allowed in both old and
	// new styles, but have different semantics.  We distinguish between old and
	// new styles based on the existence of the "remote" attribute.
	oldStyle := *i
	oldStyle.Name = ""
	switch {
	case i.Name != "" && oldStyle == Import{}:
		// Only "name" is set, this is the old-style.
	case i.Remote != "" && i.Manifest != "":
		// At least "remote" and "manifest" are set, this is the new-style.
	default:
		return fmt.Errorf("bad import: neither old style (only name is set) or new style (at least remote and manifest are set): %+v", *i)
	}
	return nil
}

// remoteKey returns a key based on the remote and manifest, used for
// cycle-detection.  It's only valid for new-style remote imports; it's empty
// for the old-style local imports.
func (i *Import) remoteKey() string {
	if i.Remote == "" {
		return ""
	}
	// We don't join the remote and manifest with a slash, since that might not be
	// unique.  E.g.
	//   remote:   https://foo.com/a/b    remote:   https://foo.com/a
	//   manifest: c                      manifest: b/c
	// In both cases, the key would be https://foo.com/a/b/c.
	return i.Remote + " + " + i.Manifest
}

// FileImport represents a file-based import.
type FileImport struct {
	// Manifest file to import from.
	File    string   `xml:"file,attr,omitempty"`
	XMLName struct{} `xml:"fileimport"`
}

func (i *FileImport) validate() error {
	if i.File == "" {
		return fmt.Errorf("bad fileimport: must specify file: %+v", *i)
	}
	return nil
}

// ProjectKey is a unique string for a project.
type ProjectKey string

// MakeProjectKey returns the project key, given the project name and remote.
func MakeProjectKey(name, remote string) ProjectKey {
	return ProjectKey(name + projectKeySeparator + remote)
}

// projectKeySeparator is a reserved string used in ProjectKeys.  It cannot
// occur in Project names.
const projectKeySeparator = "="

// ProjectKeys is a slice of ProjectKeys implementing the Sort interface.
type ProjectKeys []ProjectKey

func (pks ProjectKeys) Len() int           { return len(pks) }
func (pks ProjectKeys) Less(i, j int) bool { return string(pks[i]) < string(pks[j]) }
func (pks ProjectKeys) Swap(i, j int)      { pks[i], pks[j] = pks[j], pks[i] }

// Project represents a jiri project.
type Project struct {
	// Name is the project name.
	Name string `xml:"name,attr,omitempty"`
	// Path is the path used to store the project locally. Project
	// manifest uses paths that are relative to the $JIRI_ROOT
	// environment variable. When a manifest is parsed (e.g. in
	// RemoteProjects), the program logic converts the relative
	// paths to an absolute paths, using the current value of the
	// $JIRI_ROOT environment variable as a prefix.
	Path string `xml:"path,attr,omitempty"`
	// Protocol is the version control protocol used by the
	// project. If not set, "git" is used as the default.
	Protocol string `xml:"protocol,attr,omitempty"`
	// Remote is the project remote.
	Remote string `xml:"remote,attr,omitempty"`
	// RemoteBranch is the name of the remote branch to track.  It doesn't affect
	// the name of the local branch that jiri maintains, which is always "master".
	RemoteBranch string `xml:"remotebranch,attr,omitempty"`
	// Revision is the revision the project should be advanced to
	// during "jiri update". If not set, "HEAD" is used as the
	// default.
	Revision string `xml:"revision,attr,omitempty"`
	// GerritHost is the gerrit host where project CLs will be sent.
	GerritHost string `xml:"gerrithost,attr,omitempty"`
	// GitHooks is a directory containing git hooks that will be installed for
	// this project.
	GitHooks string   `xml:"githooks,attr,omitempty"`
	XMLName  struct{} `xml:"project"`
}

var (
	startUpperProjectBytes = []byte("<Project")
	startLowerProjectBytes = []byte("<project")
	endUpperProjectBytes   = []byte("</Project>")
	endLowerProjectBytes   = []byte("</project>")
)

// ProjectFromFile returns a project parsed from the contents of filename,
// with defaults filled in.
func ProjectFromFile(jirix *jiri.X, filename string) (*Project, error) {
	data, err := jirix.NewSeq().ReadFile(filename)
	if err != nil {
		return nil, err
	}

	// Previous versions of the jiri tool had a bug where the project start and
	// end elements were in upper-case, since the XMLName field was missing.  That
	// bug is now fixed, but the xml.Unmarshal call is case-sensitive, and will
	// fail if it sees the upper-case version.  This hack rewrites the elements to
	// the lower-case version.
	//
	// TODO(toddw): Remove when the transition to new manifests is complete.
	data = bytes.Replace(data, startUpperProjectBytes, startLowerProjectBytes, -1)
	data = bytes.Replace(data, endUpperProjectBytes, endLowerProjectBytes, -1)

	p := new(Project)
	if err := xml.Unmarshal(data, p); err != nil {
		return nil, err
	}
	if err := p.fillDefaults(); err != nil {
		return nil, err
	}
	return p, nil
}

// ToFile writes the project p to a file with the given filename, with defaults
// unfilled.
func (p Project) ToFile(jirix *jiri.X, filename string) error {
	if err := p.unfillDefaults(); err != nil {
		return err
	}
	data, err := xml.Marshal(p)
	if err != nil {
		return fmt.Errorf("project xml.Marshal failed: %v", err)
	}
	// Same logic as Manifest.ToBytes, to make the output more compact.
	data = bytes.Replace(data, endProjectSoloBytes, endElemSoloBytes, -1)
	return safeWriteFile(jirix, filename, data)
}

// Key returns a unique ProjectKey for the project.
func (p Project) Key() ProjectKey {
	return MakeProjectKey(p.Name, p.Remote)
}

func (p *Project) fillDefaults() error {
	if p.Protocol == "" {
		p.Protocol = "git"
	}
	if p.RemoteBranch == "" {
		p.RemoteBranch = "master"
	}
	if p.Revision == "" {
		p.Revision = "HEAD"
	}
	return p.validate()
}

func (p *Project) unfillDefaults() error {
	if p.Protocol == "git" {
		p.Protocol = ""
	}
	if p.RemoteBranch == "master" {
		p.RemoteBranch = ""
	}
	if p.Revision == "HEAD" {
		p.Revision = ""
	}
	return p.validate()
}

func (p *Project) validate() error {
	if strings.Contains(p.Name, projectKeySeparator) {
		return fmt.Errorf("bad project: name cannot contain %q: %+v", projectKeySeparator, *p)
	}
	if p.Protocol != "" && p.Protocol != "git" {
		return fmt.Errorf("bad project: only git protocol is supported: %+v", *p)
	}
	return nil
}

// Projects maps ProjectKeys to Projects.
type Projects map[ProjectKey]Project

// Find returns all projects in Projects with the given key or name.
func (ps Projects) Find(keyOrName string) Projects {
	projects := Projects{}
	if p, ok := ps[ProjectKey(keyOrName)]; ok {
		projects[ProjectKey(keyOrName)] = p
	} else {
		for key, p := range ps {
			if keyOrName == p.Name {
				projects[key] = p
			}
		}
	}
	return projects
}

// FindUnique returns the project in Projects with the given key or name, and
// returns an error if none or multiple matching projects are found.
func (ps Projects) FindUnique(keyOrName string) (Project, error) {
	var p Project
	projects := ps.Find(keyOrName)
	if len(projects) == 0 {
		return p, fmt.Errorf("no projects found with key or name %q", keyOrName)
	}
	if len(projects) > 1 {
		return p, fmt.Errorf("multiple projects found with name %q", keyOrName)
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
	// Data is a relative path to a directory for storing tool data
	// (e.g. tool configuration files). The purpose of this field is to
	// decouple the configuration of the data directory from the tool
	// itself so that the location of the data directory can change
	// without the need to change the tool.
	Data string `xml:"data,attr,omitempty"`
	// Name is the name of the tool binary.
	Name string `xml:"name,attr,omitempty"`
	// Package is the package path of the tool.
	Package string `xml:"package,attr,omitempty"`
	// Project identifies the project that contains the tool. If not
	// set, "https://vanadium.googlesource.com/<JiriProject>" is
	// used as the default.
	Project string   `xml:"project,attr,omitempty"`
	XMLName struct{} `xml:"tool"`
}

func (t *Tool) fillDefaults() error {
	if t.Data == "" {
		t.Data = "data"
	}
	if t.Project == "" {
		t.Project = "https://vanadium.googlesource.com/" + JiriProject
	}
	return nil
}

func (t *Tool) unfillDefaults() error {
	if t.Data == "data" {
		t.Data = ""
	}
	// Don't unfill the jiri project setting, since that's not meant to be
	// optional.
	return nil
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
		relPath, err := filepath.Rel(jirix.Root, project.Path)
		if err != nil {
			return err
		}
		project.Path = relPath
		manifest.Projects = append(manifest.Projects, project)
	}

	// Add all tools and hooks from the current manifest to the
	// snapshot manifest.
	_, tools, hooks, err := readManifest(jirix)
	if err != nil {
		return err
	}
	for _, tool := range tools {
		manifest.Tools = append(manifest.Tools, tool)
	}
	for _, hook := range hooks {
		manifest.Hooks = append(manifest.Hooks, hook)
	}
	return manifest.ToFile(jirix, path)
}

// CurrentManifest returns a manifest that identifies the result of
// the most recent "jiri update" invocation.
func CurrentManifest(jirix *jiri.X) (*Manifest, error) {
	filename := filepath.Join(jirix.Root, ".current_manifest")
	m, err := ManifestFromFile(jirix, filename)
	if runutil.IsNotExist(err) {
		fmt.Fprintf(jirix.Stderr(), `WARNING: Could not find %s.
The contents of this file are stored as metadata in binaries the jiri
tool builds. To fix this problem, please run "jiri update".
`, filename)
		return &Manifest{}, nil
	}
	return m, err
}

// writeCurrentManifest writes the given manifest to a file that
// stores the result of the most recent "jiri update" invocation.
func writeCurrentManifest(jirix *jiri.X, manifest *Manifest) error {
	filename := filepath.Join(jirix.Root, ".current_manifest")
	return manifest.ToFile(jirix, filename)
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
	if _, err := jirix.NewSeq().Stat(metadataDir); err == nil {
		project, err := ProjectFromFile(jirix, filepath.Join(metadataDir, jiri.ProjectMetaFile))
		if err != nil {
			return "", err
		}
		return project.Key(), nil
	}
	return "", nil
}

// setProjectRevisions sets the current project revision from the master for
// each project as found on the filesystem
func setProjectRevisions(jirix *jiri.X, projects Projects) (_ Projects, e error) {
	for name, project := range projects {
		switch project.Protocol {
		case "git":
			revision, err := jirix.Git(tool.RootDirOpt(project.Path)).CurrentRevisionOfBranch("master")
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

	// Slow path: Either full scan was requested, or projects exist in manifest
	// that were not found locally.  Do a recursive scan of all projects under
	// JIRI_ROOT.
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
	defer collect.Error(func() error { return jirix.NewSeq().Chdir(cwd).Done() }, &e)

	// Gather local & remote project data.
	localProjects, err := LocalProjects(jirix, FastScan)
	if err != nil {
		return nil, err
	}
	remoteProjects, _, _, err := readManifest(jirix)
	if err != nil {
		return nil, err
	}

	// Compute difference between local and remote.
	update := Update{}
	ops := computeOperations(localProjects, remoteProjects, false, "")
	s := jirix.NewSeq()
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
				if err := s.Chdir(updateOp.destination).Done(); err != nil {
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
	p, t, _, e := readManifest(jirix)
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

func readManifest(jirix *jiri.X) (Projects, Tools, Hooks, error) {
	jirix.TimerPush("read manifest")
	defer jirix.TimerPop()
	file, err := jirix.ResolveManifestPath(jirix.Manifest())
	if err != nil {
		return nil, nil, nil, err
	}
	var imp importer
	projects, tools, hooks := Projects{}, Tools{}, Hooks{}
	if err := imp.Load(jirix, jirix.Root, file, "", projects, tools, hooks); err != nil {
		return nil, nil, nil, err
	}
	return projects, tools, hooks, nil
}

func updateManifestProjects(jirix *jiri.X) error {
	jirix.TimerPush("update manifest")
	defer jirix.TimerPop()
	if jirix.UsingOldManifests() {
		return updateManifestProjectsDeprecated(jirix)
	}
	// Update the repositories corresponding to all remote imports.
	//
	// TODO(toddw): Cache local projects in jirix, so that we don't need to
	// perform multiple full scans.
	localProjects, err := LocalProjects(jirix, FullScan)
	if err != nil {
		return err
	}
	file, err := jirix.ResolveManifestPath(jirix.Manifest())
	if err != nil {
		return err
	}
	var imp importer
	return imp.Update(jirix, jirix.Root, file, "", localProjects)
}

func updateManifestProjectsDeprecated(jirix *jiri.X) error {
	manifestPath := filepath.Join(jirix.Root, ".manifest")
	manifestRemote, err := getManifestRemote(jirix, manifestPath)
	if err != nil {
		return err
	}
	project := Project{
		Path:         manifestPath,
		Protocol:     "git",
		Remote:       manifestRemote,
		Revision:     "HEAD",
		RemoteBranch: "master",
	}
	return resetProject(jirix, project)
}

// UpdateUniverse updates all local projects and tools to match the
// remote counterparts identified by the given manifest. Optionally,
// the 'gc' flag can be used to indicate that local projects that no
// longer exist remotely should be removed.
func UpdateUniverse(jirix *jiri.X, gc bool) (e error) {
	jirix.TimerPush("update universe")
	defer jirix.TimerPop()
	// 0. Update all manifest projects to match their remote counterparts, and
	// read the manifest file.
	if err := updateManifestProjects(jirix); err != nil {
		return err
	}
	remoteProjects, remoteTools, remoteHooks, err := readManifest(jirix)
	if err != nil {
		return err
	}
	s := jirix.NewSeq()
	// 1. Update all local projects to match their remote counterparts.
	if err := updateProjects(jirix, remoteProjects, gc); err != nil {
		return err
	}
	// 2. Build all tools in a temporary directory.
	tmpDir, err := s.TempDir("", "tmp-jiri-tools-build")
	if err != nil {
		return fmt.Errorf("TempDir() failed: %v", err)
	}
	defer collect.Error(func() error { return s.RemoveAll(tmpDir).Done() }, &e)
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
	defer collect.Error(func() error { return jirix.NewSeq().Chdir(cwd).Done() }, &e)

	s := jirix.NewSeq()

	// Loop through all projects, checking out master and stashing any unstaged
	// changes.
	for _, project := range projects {
		p := project
		if err := s.Chdir(p.Path).Done(); err != nil {
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
				if err := s.Chdir(p.Path).Done(); err != nil {
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
	s := jirix.NewSeq()
	var stderr bytes.Buffer

	// We unset GOARCH and GOOS because jiri update should always build for the
	// native architecture and OS.  Also, as of go1.5, setting GOBIN is not
	// compatible with GOARCH or GOOS.
	env := map[string]string{
		"GOARCH": "",
		"GOOS":   "",
		"GOBIN":  outputDir,
		"GOPATH": strings.Join(workspaces, string(filepath.ListSeparator)),
	}
	args := append([]string{"install"}, toolPkgs...)
	if err := s.Env(env).Capture(ioutil.Discard, &stderr).Last("go", args...); err != nil {
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
	if err := jirix.NewSeq().Verbose(true).Call(updateFn, "build tools: %v", strings.Join(toolNames, " ")).Done(); err != nil {
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
	defer collect.Error(func() error { return jirix.NewSeq().Chdir(wd).Done() }, &e)
	s := jirix.NewSeq()
	for _, project := range projects {
		localProjectDir := project.Path
		if err := s.Chdir(localProjectDir).Done(); err != nil {
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
	// Existence of a metadata directory is how we know we've found a
	// Jiri-maintained project.
	metadataDir := filepath.Join(path, jiri.ProjectMetaDir)
	if _, err := jirix.NewSeq().Stat(metadataDir); err != nil {
		if runutil.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ProjectAtPath returns a Project struct corresponding to the project at the
// path in the filesystem.
func ProjectAtPath(jirix *jiri.X, path string) (Project, error) {
	metadataFile := filepath.Join(path, jiri.ProjectMetaDir, jiri.ProjectMetaFile)
	project, err := ProjectFromFile(jirix, metadataFile)
	if err != nil {
		return Project{}, err
	}
	project.Path = filepath.Join(jirix.Root, project.Path)
	return *project, nil
}

// findLocalProjects scans the filesystem for all projects.  Note that project
// directories can be nested recursively.
func findLocalProjects(jirix *jiri.X, path string, projects Projects) error {
	isLocal, err := isLocalProject(jirix, path)
	if err != nil {
		return err
	}
	if isLocal {
		project, err := ProjectAtPath(jirix, path)
		if err != nil {
			return err
		}
		if path != project.Path {
			return fmt.Errorf("project %v has path %v but was found in %v", project.Name, project.Path, path)
		}
		if p, ok := projects[project.Key()]; ok {
			return fmt.Errorf("name conflict: both %v and %v contain project with key %v", p.Path, project.Path, project.Key())
		}
		projects[project.Key()] = project
	}

	// Recurse into all the sub directories.
	fileInfos, err := jirix.NewSeq().ReadDir(path)
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
	s := jirix.NewSeq()
	for _, fi := range fis {
		installFn := func() error {
			src := filepath.Join(dir, fi.Name())
			dst := filepath.Join(binDir, fi.Name())
			return jirix.NewSeq().Rename(src, dst).Done()
		}
		if err := s.Verbose(true).Call(installFn, "install tool %q", fi.Name()).Done(); err != nil {
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
	s := jirix.NewSeq()
	oldDir, newDir := filepath.Join(jirix.Root, "devtools", "bin"), jirix.BinDir()
	switch info, err := s.Lstat(oldDir); {
	case runutil.IsNotExist(err):
		// Drop down to create the symlink below.
	case err != nil:
		return fmt.Errorf("Failed to stat old bin dir: %v", err)
	case info.Mode()&os.ModeSymlink != 0:
		link, err := s.Readlink(oldDir)
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
		switch _, err := s.Stat(backupDir); {
		case runutil.IsNotExist(err):
			if err := s.Rename(oldDir, backupDir).Done(); err != nil {
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
	if err := s.MkdirAll(filepath.Dir(oldDir), 0755).Symlink(newDir, oldDir).Done(); err != nil {
		return fmt.Errorf("Failed to symlink to new bin dir %v from %v: %v", newDir, oldDir, err)
	}
	return nil
}

// runHooks runs the specified hooks
func runHooks(jirix *jiri.X, hooks Hooks) error {
	jirix.TimerPush("run hooks")
	defer jirix.TimerPop()
	s := jirix.NewSeq()
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
		if err := s.Last(command, args...); err != nil {
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

// importer handles importing manifest files.  There are two uses: Load reads
// full manifests into memory, while Update updates remote manifest projects.
type importer struct {
	cycleStack []cycleInfo
}

type cycleInfo struct {
	file, key string
}

// importNoCycles checks for cycles in imports.  There are two types of cycles:
//   file - Cycle in the paths of manifest files in the local filesystem.
//   key  - Cycle in the remote manifests specified by remote imports.
//
// Example of file cycles.  File A imports file B, and vice versa.
//     file=manifest/A              file=manifest/B
//     <manifest>                   <manifest>
//       <fileimport file="B"/>       <fileimport file="A"/>
//     </manifest>                  </manifest>
//
// Example of key cycles.  The key consists of "remote/manifest", e.g.
//   https://vanadium.googlesource.com/manifest/v2/default
// In the example, key x/A imports y/B, and vice versa.
//     key=x/A                                 key=y/B
//     <manifest>                              <manifest>
//       <import remote="y" manifest="B"/>       <import remote="x" manifest="A"/>
//     </manifest>                             </manifest>
//
// The above examples are simple, but the general strategy is demonstrated.  We
// keep a single stack for both files and keys, and push onto each stack before
// running the recursive load or update function, and pop the stack when the
// function is done.  If we see a duplicate on the stack at any point, we know
// there's a cycle.  Note that we know the file for both local fileimports as
// well as remote imports, but we only know the key for remote imports; the key
// for local fileimports is empty.
//
// A more complex case would involve a combination of local fileimports and
// remote imports, using the "root" attribute to change paths on the local
// filesystem.  In this case the key will eventually expose the cycle.
func (imp *importer) importNoCycles(file, key string, fn func() error) error {
	info := cycleInfo{file, key}
	for _, c := range imp.cycleStack {
		if file == c.file {
			return fmt.Errorf("import cycle detected in local manifest files: %q", append(imp.cycleStack, info))
		}
		if key != "" && key == c.key {
			return fmt.Errorf("import cycle detected in remote manifest imports: %q", append(imp.cycleStack, info))
		}
	}
	imp.cycleStack = append(imp.cycleStack, info)
	if err := fn(); err != nil {
		return err
	}
	imp.cycleStack = imp.cycleStack[:len(imp.cycleStack)-1]
	return nil
}

func (imp *importer) Load(jirix *jiri.X, root, file, key string, projects Projects, tools Tools, hooks Hooks) error {
	return imp.importNoCycles(file, key, func() error {
		return imp.load(jirix, root, file, projects, tools, hooks)
	})
}

func (imp *importer) load(jirix *jiri.X, root, file string, projects Projects, tools Tools, hooks Hooks) error {
	m, err := ManifestFromFile(jirix, file)
	if err != nil {
		return err
	}
	// Process all imports.
	for _, _import := range m.Imports {
		newRoot, newFile := root, ""
		if _import.Remote != "" {
			// New-style remote import
			newRoot = filepath.Join(root, _import.Root)
			newFile = filepath.Join(newRoot, _import.Path, _import.Manifest)
		} else {
			// Old-style name-based local import.
			//
			// TODO(toddw): Remove this logic when the manifest transition is done.
			if newFile, err = jirix.ResolveManifestPath(_import.Name); err != nil {
				return err
			}
		}
		if err := imp.Load(jirix, newRoot, newFile, _import.remoteKey(), projects, tools, hooks); err != nil {
			return err
		}
	}
	// Process all file imports.
	for _, fileImport := range m.FileImports {
		newFile := filepath.Join(filepath.Dir(file), fileImport.File)
		if err := imp.Load(jirix, root, newFile, "", projects, tools, hooks); err != nil {
			return err
		}
	}
	// Process all projects.
	for _, project := range m.Projects {
		project.Path = filepath.Join(root, project.Path)
		projects[project.Key()] = project
	}
	// Process all tools.
	for _, tool := range m.Tools {
		tools[tool.Name] = tool
	}
	// Process all hooks.
	for _, hook := range m.Hooks {
		project, err := projects.FindUnique(hook.Project)
		if err != nil {
			return fmt.Errorf("error while finding project %q for hook %q: %v", hook.Project, hook.Name, err)
		}
		hook.Path = filepath.Join(project.Path, hook.Path)
		hooks[hook.Name] = hook
	}
	return nil
}

func (imp *importer) Update(jirix *jiri.X, root, file, key string, localProjects Projects) error {
	return imp.importNoCycles(file, key, func() error {
		return imp.update(jirix, root, file, localProjects)
	})
}

func (imp *importer) update(jirix *jiri.X, root, file string, localProjects Projects) error {
	m, err := ManifestFromFile(jirix, file)
	if err != nil {
		return err
	}
	// Process all remote imports.  This logic treats the remote import as a
	// regular project, and runs our regular create/move/update logic on it.  We
	// never handle deletes here; those are handled in updateProjects.
	for _, remote := range m.Imports {
		if remote.Remote == "" {
			// Old-style local imports handled in loop below.
			continue
		}
		newRoot := filepath.Join(root, remote.Root)
		remote.Path = filepath.Join(newRoot, remote.Path)
		newFile := filepath.Join(remote.Path, remote.Manifest)
		var localProject *Project
		if p, ok := localProjects[remote.Project.Key()]; ok {
			localProject = &p
		}
		// Since &remote.Project is never nil, we'll never produce a delete op.
		op := computeOp(localProject, &remote.Project, false, newRoot)
		if err := op.Test(jirix, newFsUpdates()); err != nil {
			return err
		}
		updateFn := func() error { return op.Run(jirix, nil) }
		if err := jirix.NewSeq().Verbose(true).Call(updateFn, "%v", op).Done(); err != nil {
			fmt.Fprintf(jirix.Stderr(), "%v\n", err)
			return err
		}
		localProjects[remote.Project.Key()] = remote.Project
		if err := imp.Update(jirix, newRoot, newFile, remote.remoteKey(), localProjects); err != nil {
			return err
		}
	}
	// Process all old-style local imports.
	for _, local := range m.Imports {
		if local.Remote != "" {
			// New-style remote imports handled in loop above.
			continue
		}
		newFile, err := jirix.ResolveManifestPath(local.Name)
		if err != nil {
			return err
		}
		if err := imp.Update(jirix, root, newFile, "", localProjects); err != nil {
			return err
		}
	}
	// Process all file imports.
	for _, fileImport := range m.FileImports {
		newFile := filepath.Join(filepath.Dir(file), fileImport.File)
		if err := imp.Update(jirix, root, newFile, "", localProjects); err != nil {
			return err
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
	defer collect.Error(func() error { return jirix.NewSeq().Chdir(cwd).Done() }, &e)
	s := jirix.NewSeq()
	if err := s.Chdir(project.Path).Done(); err != nil {
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
			s.Verbose(true).Output([]string{line1, line2})
		}
		return nil
	default:
		return UnsupportedProtocolErr(project.Protocol)
	}
}

// groupByGoogleSourceHosts returns a map of googlesource host to a Projects
// map where all project remotes come from that host.
func groupByGoogleSourceHosts(ps Projects) map[string]Projects {
	m := make(map[string]Projects)
	for _, p := range ps {
		if !googlesource.IsGoogleSourceRemote(p.Remote) {
			continue
		}
		u, err := url.Parse(p.Remote)
		if err != nil {
			continue
		}
		host := u.Scheme + "://" + u.Host
		if _, ok := m[host]; !ok {
			m[host] = Projects{}
		}
		m[host][p.Key()] = p
	}
	return m
}

// getRemoteHeadRevisions attempts to get the repo statuses from remote for
// projects at HEAD so we can detect when a local project is already
// up-to-date.
func getRemoteHeadRevisions(jirix *jiri.X, remoteProjects Projects) {
	projectsAtHead := Projects{}
	for _, rp := range remoteProjects {
		if rp.Revision == "HEAD" {
			projectsAtHead[rp.Key()] = rp
		}
	}
	gsHostsMap := groupByGoogleSourceHosts(projectsAtHead)
	for host, projects := range gsHostsMap {
		branchesMap := make(map[string]bool)
		for _, p := range projects {
			branchesMap[p.RemoteBranch] = true
		}
		branches := set.StringBool.ToSlice(branchesMap)
		repoStatuses, err := googlesource.GetRepoStatuses(jirix, host, branches)
		if err != nil {
			// Log the error but don't fail.
			fmt.Fprintf(jirix.Stderr(), "Error fetching repo statuses from remote: %v\n", err)
			continue
		}
		for _, p := range projects {
			status, ok := repoStatuses[p.Name]
			if !ok {
				continue
			}
			rev, ok := status.Branches[p.RemoteBranch]
			if !ok || rev == "" {
				continue
			}
			rp := remoteProjects[p.Key()]
			rp.Revision = rev
			remoteProjects[p.Key()] = rp
		}
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
	ops := computeOperations(localProjects, remoteProjects, gc, "")
	updates := newFsUpdates()
	for _, op := range ops {
		if err := op.Test(jirix, updates); err != nil {
			return err
		}
	}
	failed := false
	manifest := &Manifest{Label: jirix.Manifest()}
	s := jirix.NewSeq()
	for _, op := range ops {
		updateFn := func() error { return op.Run(jirix, manifest) }
		// Always log the output of updateFn, irrespective of
		// the value of the verbose flag.
		if err := s.Verbose(true).Call(updateFn, "%v", op).Done(); err != nil {
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
	defer collect.Error(func() error { return jirix.NewSeq().Chdir(cwd).Done() }, &e)

	s := jirix.NewSeq()
	if err := s.MkdirAll(metadataDir, os.FileMode(0755)).
		Chdir(metadataDir).Done(); err != nil {
		return err
	}

	// Replace absolute project paths with relative paths to make it
	// possible to move the $JIRI_ROOT directory locally.
	relPath, err := filepath.Rel(jirix.Root, project.Path)
	if err != nil {
		return err
	}
	project.Path = relPath
	metadataFile := filepath.Join(metadataDir, jiri.ProjectMetaFile)
	return project.ToFile(jirix, metadataFile)
}

// addProjectToManifest records the information about the given
// project in the given manifest. The function is used to create a
// manifest that records the current state of jiri projects, which
// can be used to restore this state at some later point.
//
// NOTE: The function assumes that the the given project is on a
// master branch.
func addProjectToManifest(jirix *jiri.X, manifest *Manifest, project Project) error {
	if manifest == nil {
		return nil
	}
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
	relPath, err := filepath.Rel(jirix.Root, project.Path)
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
	root string
}

func (op createOperation) Run(jirix *jiri.X, manifest *Manifest) (e error) {
	s := jirix.NewSeq()

	path, perm := filepath.Dir(op.destination), os.FileMode(0755)
	tmpDirPrefix := strings.Replace(op.Project().Name, "/", ".", -1) + "-"

	// Create a temporary directory for the initial setup of the
	// project to prevent an untimely termination from leaving the
	// $JIRI_ROOT directory in an inconsistent state.
	tmpDir, err := s.MkdirAll(path, perm).TempDir(path, tmpDirPrefix)
	if err != nil {
		return err
	}
	defer collect.Error(func() error { return jirix.NewSeq().RemoveAll(tmpDir).Done() }, &e)
	switch op.project.Protocol {
	case "git":
		if err := jirix.Git().Clone(op.project.Remote, tmpDir); err != nil {
			return err
		}

		// Apply git hooks.  We're creating this repo, so there's no danger of
		// overriding existing hooks.  Customizing your git hooks with jiri is a bad
		// idea anyway, since jiri won't know to not delete the project when you
		// switch between manifests or do a cleanup.
		gitHooksDstDir := filepath.Join(tmpDir, ".git", "hooks")
		if op.project.GitHooks != "" {
			gitHooksSrcDir := filepath.Join(jirix.Root, op.root, op.project.GitHooks)
			// Copy the specified GitHooks directory into the project's git
			// hook directory.  We walk the file system, creating directories
			// and copying files as we encounter them.
			copyFn := func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				relPath, err := filepath.Rel(gitHooksSrcDir, path)
				if err != nil {
					return err
				}
				dst := filepath.Join(gitHooksDstDir, relPath)
				if info.IsDir() {
					return s.MkdirAll(dst, perm).Done()
				}
				src, err := s.ReadFile(path)
				if err != nil {
					return err
				}
				return s.WriteFile(dst, src, perm).Done()
			}
			if err := filepath.Walk(gitHooksSrcDir, copyFn); err != nil {
				return err
			}
		}

		// Apply exclusion for /.jiri/. We're creating the repo so we can safely
		// write to .git/info/exclude
		excludeString := "/.jiri/\n"
		excludeDir := filepath.Join(tmpDir, ".git", "info")
		excludeFile := filepath.Join(excludeDir, "exclude")
		if err := s.MkdirAll(excludeDir, os.FileMode(0750)).
			WriteFile(excludeFile, []byte(excludeString), perm).Done(); err != nil {
			return err
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		defer collect.Error(func() error { return jirix.NewSeq().Chdir(cwd).Done() }, &e)
		if err := s.Chdir(tmpDir).Done(); err != nil {
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
	if err := s.Chmod(tmpDir, os.FileMode(0755)).
		Rename(tmpDir, op.destination).Done(); err != nil {
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
	if _, err := jirix.NewSeq().Stat(op.destination); err != nil {
		if !runutil.IsNotExist(err) {
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
	s := jirix.NewSeq()
	if op.gc {
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
			s.Verbose(true).Output(lines)
			return nil
		}
		return s.RemoveAll(op.source).Done()
	}
	lines := []string{
		fmt.Sprintf("NOTE: project %v was not found in the project manifest", op.project.Name),
		"it was not automatically removed to avoid deleting uncommitted work",
		fmt.Sprintf(`if you no longer need it, invoke "rm -rf %v"`, op.source),
		`or invoke "jiri update -gc" to remove all such local projects`,
	}
	s.Verbose(true).Output(lines)
	return nil
}

func (op deleteOperation) String() string {
	return fmt.Sprintf("delete project %q from %q", op.project.Name, op.source)
}

func (op deleteOperation) Test(jirix *jiri.X, updates *fsUpdates) error {
	if _, err := jirix.NewSeq().Stat(op.source); err != nil {
		if runutil.IsNotExist(err) {
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
	s := jirix.NewSeq()
	path, perm := filepath.Dir(op.destination), os.FileMode(0755)
	if err := s.MkdirAll(path, perm).Rename(op.source, op.destination).Done(); err != nil {
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
	s := jirix.NewSeq()
	if _, err := s.Stat(op.source); err != nil {
		if runutil.IsNotExist(err) {
			return fmt.Errorf("cannot move %q to %q as the source does not exist", op.source, op.destination)
		}
		return err
	}
	if _, err := s.Stat(op.destination); err != nil {
		if !runutil.IsNotExist(err) {
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
func computeOperations(localProjects, remoteProjects Projects, gc bool, root string) operations {
	result := operations{}
	allProjects := map[ProjectKey]bool{}
	for _, p := range localProjects {
		allProjects[p.Key()] = true
	}
	for _, p := range remoteProjects {
		allProjects[p.Key()] = true
	}
	for key, _ := range allProjects {
		var local, remote *Project
		if project, ok := localProjects[key]; ok {
			local = &project
		}
		if project, ok := remoteProjects[key]; ok {
			remote = &project
		}
		result = append(result, computeOp(local, remote, gc, root))
	}
	sort.Sort(result)
	return result
}

func computeOp(local, remote *Project, gc bool, root string) operation {
	switch {
	case local != nil && remote != nil:
		if local.Path != remote.Path {
			// moveOperation also does an update, so we don't need to check the
			// revision here.
			return moveOperation{commonOperation{
				destination: remote.Path,
				project:     *remote,
				source:      local.Path,
			}}
		}
		if local.Revision != remote.Revision {
			return updateOperation{commonOperation{
				destination: remote.Path,
				project:     *remote,
				source:      local.Path,
			}}
		}
		return nullOperation{commonOperation{
			destination: remote.Path,
			project:     *remote,
			source:      local.Path,
		}}
	case local != nil && remote == nil:
		return deleteOperation{commonOperation{
			destination: "",
			project:     *local,
			source:      local.Path,
		}, gc}
	case local == nil && remote != nil:
		return createOperation{commonOperation{
			destination: remote.Path,
			project:     *remote,
			source:      "",
		}, root}
	default:
		panic("jiri: computeOp called with nil local and remote")
	}
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
