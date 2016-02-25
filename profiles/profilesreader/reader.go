// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package profilesreader provides support for reading and processing jiri profiles.
package profilesreader

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"v.io/jiri/jiri"
	"v.io/jiri/profiles"
	"v.io/jiri/project"
	"v.io/jiri/util"
	"v.io/x/lib/envvar"
)

// GoFlags lists all of the Go environment variables and will be sorted in the
// init function for this package.
var GoFlags = []string{
	"CC",
	"CC_FOR_TARGET",
	"CGO_ENABLED",
	"CXX_FOR_TARGET",
	"GO15VENDOREXPERIMENT",
	"GOARCH",
	"GOBIN",
	"GOEXE",
	"GOGCCFLAGS",
	"GOHOSTARCH",
	"GOHOSTOS",
	"GOOS",
	"GOPATH",
	"GORACE",
	"GOROOT",
	"GOTOOLDIR",
}

type ProfilesMode bool

func (pm *ProfilesMode) Set(s string) error {
	v, err := strconv.ParseBool(s)
	*pm = ProfilesMode(v)
	return err
}

func (pm *ProfilesMode) Get() interface{} { return bool(*pm) }

func (pm *ProfilesMode) String() string { return fmt.Sprintf("%v", *pm) }

func (pm *ProfilesMode) IsBoolFlag() bool { return true }

const (
	UseProfiles  ProfilesMode = false
	SkipProfiles ProfilesMode = true
)

func init() {
	sort.Strings(GoFlags)
}

// UnsetGoEnvVars unsets Go environment variables in the given environment.
func UnsetGoEnvVars(env *envvar.Vars) {
	for _, k := range GoFlags {
		env.Delete(k)
	}
}

// UnsetGoEnvMap unsets Go environment variables in the given environment.
func UnsetGoEnvMap(env map[string]string) {
	for _, k := range GoFlags {
		delete(env, k)
	}
}

// GoEnvironmentFromOS() returns the values of all Go environment variables
// as set via the OS; unset variables are omitted.
func GoEnvironmentFromOS() []string {
	os := envvar.SliceToMap(os.Environ())
	vars := make([]string, 0, len(GoFlags))
	for _, k := range GoFlags {
		v, present := os[k]
		if !present {
			continue
		}
		vars = append(vars, envvar.JoinKeyValue(k, v))
	}
	return vars
}

// Reader wraps the various sources of configuration and profile
// information to provide convenient methods for determing the environment
// variables to use for a given situation. It creates an initial copy of the OS
// environment that is mutated by its various methods.
type Reader struct {
	*envvar.Vars
	profilesMode bool
	path         string
	jirix        *jiri.X
	config       *util.Config
	projects     project.Projects
	pdb          *profiles.DB
}

// NewReader creates a new profiles reader. If path is of non-zero
// length then that path will be read as a profiles database, if not, the
// existing, if any, in-memory profiles information will be used. If SkipProfiles
// is specified for profilesMode, then no profiles are used.
func NewReader(jirix *jiri.X, profilesMode ProfilesMode, path string) (*Reader, error) {
	config, err := util.LoadConfig(jirix)
	if err != nil {
		return nil, err
	}
	projects, err := project.LocalProjects(jirix, project.FastScan)
	if err != nil {
		return nil, err
	}
	pdb := profiles.NewDB()
	if profilesMode == UseProfiles && len(path) > 0 {
		if err := pdb.Read(jirix, path); err != nil {
			return nil, err
		}
	}
	rd := &Reader{
		jirix:        jirix,
		path:         path,
		config:       config,
		projects:     projects,
		profilesMode: bool(profilesMode),
		pdb:          pdb,
	}
	rd.Vars = envvar.VarsFromOS()
	if profilesMode == SkipProfiles {
		return rd, nil
	}
	if len(os.Getenv("JIRI_PROFILE")) > 0 {
		return nil, fmt.Errorf(`old style profiles are no longer supported. Please
do not set JIRI_PROFILE.`)
	}
	return rd, nil
}

func (rd *Reader) SchemaVersion() profiles.Version {
	return rd.pdb.SchemaVersion()
}

func (rd *Reader) Path() string {
	return rd.path
}

func (rd *Reader) ProfileNames() []string {
	return rd.pdb.Names()
}

func (rd *Reader) Profiles() []*profiles.Profile {
	return rd.pdb.Profiles()
}

func (rd *Reader) LookupProfile(name string) *profiles.Profile {
	installer, profile := profiles.SplitProfileName(name)
	return rd.pdb.LookupProfile(installer, profile)
}

func (rd *Reader) LookupProfileTarget(name string, target profiles.Target) *profiles.Target {
	installer, profile := profiles.SplitProfileName(name)
	return rd.pdb.LookupProfileTarget(installer, profile, target)
}

// MergeEnv merges the embedded environment with the environment
// variables provided by the vars parameter according to the policies parameter.
func (rd *Reader) MergeEnv(policies map[string]MergePolicy, vars ...[]string) {
	MergeEnv(policies, rd.Vars, vars...)
}

// EnvFromProfile obtains the environment variable settings from the specified
// profile and target. It returns nil if the target and/or profile could not
// be found.
func (rd *Reader) EnvFromProfile(name string, target profiles.Target) []string {
	installer, profile := profiles.SplitProfileName(name)
	return rd.pdb.EnvFromProfile(installer, profile, target)
}

// MergeEnvFromProfiles merges the embedded environment with the environment
// variables stored in the requested profiles. The profiles are those read from
// the manifest and in addition the 'jiri' profile may be used which refers to
// the environment variables maintained by the jiri tool itself. It will also
// expand all instances of ${JIRI_ROOT} in the returned environment.
func (rd *Reader) MergeEnvFromProfiles(policies map[string]MergePolicy, target profiles.Target, profileNames ...string) {
	envs := [][]string{}
	for _, name := range profileNames {
		var e []string
		if name == "jiri" {
			e = rd.JiriProfile()
		} else {
			installer, profile := profiles.SplitProfileName(name)
			e = rd.pdb.EnvFromProfile(installer, profile, target)
		}
		if e == nil {
			continue
		}
		envs = append(envs, e)
	}
	MergeEnv(policies, rd.Vars, envs...)
	jiri.ExpandEnv(rd.jirix, rd.Vars)
}

// SkippingProfiles returns true if no profiles are being used.
func (rd *Reader) SkippingProfiles() bool {
	return rd.profilesMode == bool(SkipProfiles)
}

// ValidateRequestProfilesAndTarget checks that the supplied slice of profiles
// names is supported (including the 'jiri' profile) and that each has
// the specified target installed taking account if running using profiles
// at all or if using old-style profiles.
func (rd *Reader) ValidateRequestedProfilesAndTarget(profileNames []string, target profiles.Target) error {
	if rd.SkippingProfiles() {
		return nil
	}
	for _, name := range profileNames {
		if name == "jiri" {
			continue
		}
		installer, profile := profiles.SplitProfileName(name)
		if rd.pdb.LookupProfileTarget(installer, profile, target) == nil {
			return fmt.Errorf("%q for %q is not available or not installed, use the \"list\" command to see the installed/available profiles.", target, name)
		}
	}
	return nil
}

// PrependToPath prepends its argument to the PATH environment variable.
func (rd *Reader) PrependToPATH(path string) {
	existing := rd.GetTokens("PATH", ":")
	rd.SetTokens("PATH", append([]string{path}, existing...), ":")
}

// JiriProfile returns a pseudo profile that is maintained by the Jiri
// tool itself, this currently consists of the GoPath and VDLPath variables.
// It will generally be used as the last profile in the set of profiles
// passed to MergeEnv.
func (rd *Reader) JiriProfile() []string {
	return []string{rd.GoPath(), rd.VDLPath()}
}

// GoPath computes and returns the GOPATH environment variable based on the
// current jiri configuration.
func (rd *Reader) GoPath() string {
	path := pathHelper(rd.jirix, rd.projects, rd.config.GoWorkspaces(), "")
	return "GOPATH=" + envvar.JoinTokens(path, ":")
}

// VDLPath computes and returns the VDLPATH environment variable based on the
// current jiri configuration.
func (rd *Reader) VDLPath() string {
	path := pathHelper(rd.jirix, rd.projects, rd.config.VDLWorkspaces(), "src")
	return "VDLPATH=" + envvar.JoinTokens(path, ":")
}

// pathHelper is a utility function for determining paths for project workspaces.
func pathHelper(jirix *jiri.X, projects project.Projects, workspaces []string, suffix string) []string {
	path := []string{}
	for _, workspace := range workspaces {
		absWorkspace := filepath.Join(jirix.Root, workspace, suffix)
		// Only append an entry to the path if the workspace is rooted
		// under a jiri project that exists locally or vice versa.
		for _, project := range projects {
			// We check if <project.Path> is a prefix of <absWorkspace> to
			// account for Go workspaces nested under a single jiri project,
			// such as: $JIRI_ROOT/release/projects/chat/go.
			//
			// We check if <absWorkspace> is a prefix of <project.Path> to
			// account for Go workspaces that span multiple jiri projects,
			// such as: $JIRI_ROOT/release/go.
			if strings.HasPrefix(absWorkspace, project.Path) || strings.HasPrefix(project.Path, absWorkspace) {
				if _, err := jirix.NewSeq().Stat(filepath.Join(absWorkspace)); err == nil {
					path = append(path, absWorkspace)
					break
				}
			}
		}
	}
	return path
}

// The environment variables passed to a subprocess are the result
// of merging those in the processes environment and those from
// one or more profiles according to the policies defined below.
// There is a starting environment, nominally called 'base', and one
// or profile environments. The base environment will typically be that
// inherited by the running process from its invoking shell. A policy
// consists of an 'action' and an optional separator to use when concatenating
// variables.
type MergePolicy struct {
	Action    MergeAction
	Separator string
}

type MergeAction int

const (
	// Use the first value encountered
	First MergeAction = iota
	// Use the last value encountered.
	Last
	// Ignore the variable regardless of where it occurs.
	Ignore
	// Append the current value to the values already accumulated.
	Append
	// Prepend the current value to the values already accumulated.
	Prepend
	// Ignore the value in the base environment, but append in the profiles.
	IgnoreBaseAndAppend
	// Ignore the value in the base environment, but prepend in the profiles.
	IgnoreBaseAndPrepend
	// Ignore the value in the base environment, but use the first value from profiles.
	IgnoreBaseAndUseFirst
	// Ignore the value in the base environment, but use the last value from profiles.
	IgnoreBaseAndUseLast
	// Ignore the values in the profiles.
	IgnoreProfiles
)

var (
	// A MergePolicy with a Last action.
	UseLast = MergePolicy{Action: Last}
	// A MergePolicy with a First action.
	UseFirst = MergePolicy{Action: First}
	// A MergePolicy that ignores the variable, regardless of where it occurs.
	IgnoreVariable = MergePolicy{Action: Ignore}
	// A MergePolicy that appends using : as a separator.
	AppendPath = MergePolicy{Action: Append, Separator: ":"}
	// A MergePolicy that appends using " " as a separator.
	AppendFlag = MergePolicy{Action: Append, Separator: " "}
	// A MergePolicy that prepends using : as a separator.
	PrependPath = MergePolicy{Action: Prepend, Separator: ":"}
	// A MergePolicy that prepends using " " as a separator.
	PrependFlag = MergePolicy{Action: Prepend, Separator: " "}
	// A MergePolicy that will ignore base, but append across profiles using ':'
	IgnoreBaseAppendPath = MergePolicy{Action: IgnoreBaseAndAppend, Separator: ":"}
	// A MergePolicy that will ignore base, but append across profiles using ' '
	IgnoreBaseAppendFlag = MergePolicy{Action: IgnoreBaseAndAppend, Separator: " "}
	// A MergePolicy that will ignore base, but prepend across profiles using ':'
	IgnoreBasePrependPath = MergePolicy{Action: IgnoreBaseAndPrepend, Separator: ":"}
	// A MergePolicy that will ignore base, but prepend across profiles using ' '
	IgnoreBasePrependFlag = MergePolicy{Action: IgnoreBaseAndPrepend, Separator: " "}
	// A MergePolicy that will ignore base, but use the last value from profiles.
	IgnoreBaseUseFirst = MergePolicy{Action: IgnoreBaseAndUseFirst}
	// A MergePolicy that will ignore base, but use the last value from profiles.
	IgnoreBaseUseLast = MergePolicy{Action: IgnoreBaseAndUseLast}
	// A MergePolicy that will always use the value from base and ignore profiles.
	UseBaseIgnoreProfiles = MergePolicy{Action: IgnoreProfiles}
)

// ProfileMergePolicies returns an instance of MergePolicies that containts
// appropriate default policies for use with MergeEnv from within
// profile implementations.
func ProfileMergePolicies() MergePolicies {
	values := MergePolicies{
		"PATH":         AppendPath,
		"CCFLAGS":      AppendFlag,
		"CXXFLAGS":     AppendFlag,
		"LDFLAGS":      AppendFlag,
		"CGO_CFLAGS":   AppendFlag,
		"CGO_CXXFLAGS": AppendFlag,
		"CGO_LDFLAGS":  AppendFlag,
		"GOPATH":       IgnoreBaseAppendPath,
		"GOARCH":       UseBaseIgnoreProfiles,
		"GOOS":         UseBaseIgnoreProfiles,
	}
	mp := MergePolicies{}
	for k, v := range values {
		mp[k] = v
	}
	return mp
}

// JiriMergePolicies returns an instance of MergePolicies that contains
// appropriate default policies for use with MergeEnv from jiri packages
// and subcommands such as those used to build go, java etc.
func JiriMergePolicies() MergePolicies {
	mp := ProfileMergePolicies()
	mp["GOPATH"] = PrependPath
	mp["VDLPATH"] = PrependPath
	mp["GOARCH"] = UseFirst
	mp["GOOS"] = UseFirst
	mp["GOROOT"] = IgnoreBaseUseLast
	return mp
}

// MergeEnv merges environment variables in base with those
// in vars according to the suppled policies.
func MergeEnv(policies map[string]MergePolicy, base *envvar.Vars, vars ...[]string) {
	// Remove any variables that have the IgnoreBase policy.
	for k, _ := range base.ToMap() {
		switch policies[k].Action {
		case Ignore, IgnoreBaseAndAppend, IgnoreBaseAndPrepend, IgnoreBaseAndUseFirst, IgnoreBaseAndUseLast:
			base.Delete(k)
		}
	}
	for _, ev := range vars {
		for _, tmp := range ev {
			k, v := envvar.SplitKeyValue(tmp)
			policy := policies[k]
			action := policy.Action
			switch policy.Action {
			case IgnoreBaseAndAppend:
				action = Append
			case IgnoreBaseAndPrepend:
				action = Prepend
			case IgnoreBaseAndUseLast:
				action = Last
			case IgnoreBaseAndUseFirst:
				action = First
			}
			switch action {
			case Ignore, IgnoreProfiles:
				continue
			case Append, Prepend:
				sep := policy.Separator
				ov := base.GetTokens(k, sep)
				nv := envvar.SplitTokens(v, sep)
				if action == Append {
					base.SetTokens(k, append(ov, nv...), sep)
				} else {
					base.SetTokens(k, append(nv, ov...), sep)
				}
			case First:
				if !base.Contains(k) {
					base.Set(k, v)
				}
			case Last:
				base.Set(k, v)
			}
		}
	}
}

// WithDefaultVersion returns a copy of the supplied target with its
// version set to the default (i.e. emtpy string).
func WithDefaultVersion(target profiles.Target) profiles.Target {
	t := &target
	t.SetVersion("")
	return target
}

type MergePolicies map[string]MergePolicy

func (mp *MergePolicy) String() string {
	switch mp.Action {
	case First:
		return "use first"
	case Last:
		return "use last"
	case Append:
		return "append using '" + mp.Separator + "'"
	case Prepend:
		return "prepend using '" + mp.Separator + "'"
	case IgnoreBaseAndAppend:
		return "ignore in environment/base, append using '" + mp.Separator + "'"
	case IgnoreBaseAndPrepend:
		return "ignore in environment/base, prepend using '" + mp.Separator + "'"
	case IgnoreBaseAndUseLast:
		return "ignore in environment/base, use last value from profiles"
	case IgnoreBaseAndUseFirst:
		return "ignore in environment/base, use first value from profiles"
	case IgnoreProfiles:
		return "ignore in profiles"
	}
	return "unrecognised action"
}

func (mp MergePolicies) Usage() string {
	return `<var>:<var>|<var>:|+<var>|<var>+|=<var>|<var>=
<var> - use the first value of <var> encountered, this is the default action.
<var>* - use the last value of <var> encountered.
-<var> - ignore the variable, regardless of where it occurs.
:<var> - append instances of <var> using : as a separator.
<var>: - prepend instances of <var> using : as a separator.
+<var> - append instances of <var> using space as a separator.
<var>+ - prepend instances of <var> using space as a separator.
^:<var> - ignore <var> from the base/inherited environment but append in profiles as per :<var>.
^<var>: - ignore <var> from the base/inherited environment but prepend in profiles as per <var>:.
^+<var> - ignore <var> from the base/inherited environment but append in profiles as per +<var>.
^<var>+ - ignore <var> from the base/inherited environment but prepend in profiles as per <var>+.
^<var> - ignore <var> from the base/inherited environment but use the first value encountered in profiles.
^<var>* - ignore <var> from the base/inherited environment but use the last value encountered in profiles.
<var>^ - ignore <var> from profiles.`
}

func separator(s string) string {
	switch s {
	case ":":
		return ":"
	default:
		return "+"
	}
}

// String implements flag.Value. It generates a string that can be used
// to recreate the MergePolicies value and that can be passed as a parameter
// to another process.
func (mp MergePolicies) String() string {
	buf := bytes.Buffer{}
	// Ensure a stable order.
	keys := make([]string, 0, len(mp))
	for k, _ := range mp {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := mp[k]
		var s string
		switch v.Action {
		case First:
			s = k
		case Last:
			s = k + "*"
		case Append:
			s = separator(v.Separator) + k
		case Prepend:
			s = k + separator(v.Separator)
		case IgnoreBaseAndAppend:
			s = "^" + separator(v.Separator) + k
		case IgnoreBaseAndPrepend:
			s = "^" + k + separator(v.Separator)
		case IgnoreBaseAndUseLast:
			s = "^" + k + "*"
		case IgnoreBaseAndUseFirst:
			s = "^" + k
		case IgnoreProfiles:
			s = k + "^"
		}
		buf.WriteString(s)
		buf.WriteString(",")
	}
	return strings.TrimSuffix(buf.String(), ",")
}

func (mp MergePolicies) DebugString() string {
	buf := bytes.Buffer{}
	for k, v := range mp {
		buf.WriteString(k + ": " + v.String() + ", ")
	}
	return strings.TrimSuffix(buf.String(), ", ")
}

// Get implements flag.Getter
func (mp MergePolicies) Get() interface{} {
	r := make(MergePolicies, len(mp))
	for k, v := range mp {
		r[k] = v
	}
	return r
}

func parseIgnoreBase(val string) (MergePolicy, string) {
	if len(val) == 0 {
		return IgnoreBaseUseLast, val
	}
	// [:+]<var>
	switch val[0] {
	case ':':
		return IgnoreBaseAppendPath, val[1:]
	case '+':
		return IgnoreBaseAppendFlag, val[1:]
	}
	// <var>[:+]
	last := len(val) - 1
	switch val[last] {
	case ':':
		return IgnoreBasePrependPath, val[:last]
	case '+':
		return IgnoreBasePrependFlag, val[:last]
	case '*':
		return IgnoreBaseUseLast, val[:last]
	}
	return IgnoreBaseUseFirst, val
}

// Set implements flag.Value
func (mp MergePolicies) Set(values string) error {
	if len(values) == 0 {
		return fmt.Errorf("no value!")
	}
	for _, val := range strings.Split(values, ",") {
		// [:+^-]<var>
		switch val[0] {
		case '^':
			a, s := parseIgnoreBase(val[1:])
			mp[s] = a
			continue
		case '-':
			mp[val[1:]] = IgnoreVariable
			continue
		case ':':
			mp[val[1:]] = AppendPath
			continue
		case '+':
			mp[val[1:]] = AppendFlag
			continue
		}
		// <var>[:+^]
		last := len(val) - 1
		switch val[last] {
		case ':':
			mp[val[:last]] = PrependPath
		case '+':
			mp[val[:last]] = PrependFlag
		case '*':
			mp[val[:last]] = UseLast
		case '^':
			mp[val[:last]] = UseBaseIgnoreProfiles
		default:
			mp[val] = UseFirst
		}
	}
	return nil
}
