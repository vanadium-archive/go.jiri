// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"v.io/x/lib/envvar"
)

// Target represents specification for the environment that the profile is
// to be built for. Targets include a version string to allow for upgrades and
// for the simultaneous existence of incompatible versions.
//
// Target and Environment implement flag.Getter so that they may be used
// with the flag package. Two flags are required, one to specify the target
// in <arch>-<os>@<version> format and a second to specify environment
// variables either as comma separated values or as repeated arguments.
type Target struct {
	arch, opsys, version string
	// The environment as specified on the command line
	commandLineEnv Environment
	// The environment as modified by a profile implementation
	Env             Environment
	InstallationDir string // where this target is installed.
	UpdateTime      time.Time
	isSet           bool
}

// Arch returns the archiecture of this target.
func (pt *Target) Arch() string {
	return pt.arch
}

// OS returns the operating system of this target.
func (pt *Target) OS() string {
	return pt.opsys
}

// Version returns the version of this target.
func (pt *Target) Version() string {
	return pt.version
}

// SetVersion sets the version for the target.
func (pt *Target) SetVersion(v string) {
	pt.version = v
}

// CommandLineEnv returns the environment variables set on the
// command line for this target.
func (pt Target) CommandLineEnv() Environment {
	r := Environment{Vars: make([]string, len(pt.commandLineEnv.Vars))}
	copy(r.Vars, pt.commandLineEnv.Vars)
	return r
}

// UseCommandLineEnv copies the command line supplied environment variables
// into the mutable environment of the Target. It should be called as soon
// as all command line parsing has been completed and before the target is
// otherwise used.
func (pt *Target) UseCommandLineEnv() {
	pt.Env = pt.CommandLineEnv()
}

// TargetSpecificDirname returns a directory name that is specific
// to that target taking account the architecture, operating system and
// command line environment variables, if relevant, into account (e.g
// GOARM={5,6,7}).
func (pt *Target) TargetSpecificDirname() string {
	env := envvar.SliceToMap(pt.commandLineEnv.Vars)
	dir := pt.arch + "_" + pt.opsys
	if pt.arch == "arm" {
		if armv, present := env["GOARM"]; present {
			dir += "_armv" + armv
		}
	}
	return dir
}

type Environment struct {
	Vars []string `xml:"var"`
}

// NewTarget creates a new target using the supplied target and environment
// parameters specified in command line format.
func NewTarget(target string, env ...string) (Target, error) {
	t := &Target{}
	err := t.Set(target)
	for _, e := range env {
		t.commandLineEnv.Set(e)
	}
	return *t, err
}

// Match returns true if pt and pt2 meet the following criteria in the
// order they are listed:
// - if the Arch and OS fields are exactly the same
// - if pt has a non-zero length Version field, then it must be
//   the same as that in pt2
// Match is used by the various methods and functions in this package
// when looking up Targets unless otherwise specified.
func (pt Target) Match(pt2 *Target) bool {
	if pt.arch != pt2.arch || pt.opsys != pt2.opsys {
		return false
	}
	if (len(pt.version) > 0) && (pt.version != pt2.version) {
		return false
	}
	return true
}

// Less returns true if pt2 is considered less than pt. The ordering
// takes into account only the architecture, operating system and version of
// the target. The architecture and operating system are ordered
// lexicographically in ascending order, then the version is ordered but in
// descending lexicographic order except that the empty string is considered
// the 'highest' value.
// Thus, (targets in <arch>-<os>[@<version>] format), are all true:
// b-c < c-c
// b-c@3 < b-c@2
func (pt *Target) Less(pt2 *Target) bool {
	switch {
	case pt.arch != pt2.arch:
		return pt.arch < pt2.arch
	case pt.opsys != pt2.opsys:
		return pt.opsys < pt2.opsys
	case len(pt.version) == 0 && len(pt2.version) > 0:
		return true
	case len(pt.version) > 0 && len(pt2.version) == 0:
		return false
	case pt.version != pt2.version:
		return compareVersions(pt.version, pt2.version) > 0
	default:
		return false
	}
}

// CrossCompiling returns true if the target differs from that of the runtime.
func (pt Target) CrossCompiling() bool {
	arch, _ := goarch()
	return (pt.arch != arch) || (pt.opsys != runtime.GOOS)
}

// Usage returns the usage string for Target.
func (pt *Target) Usage() string {
	return "specifies a profile target in the following form: <arch>-<os>[@<version>]"
}

// Set implements flag.Value.
func (t *Target) Set(val string) error {
	index := strings.IndexByte(val, '@')
	if index > -1 {
		t.version = val[index+1:]
		val = val[:index]
	}
	parts := strings.Split(val, "-")
	if len(parts) != 2 || (len(parts[0]) == 0 || len(parts[1]) == 0) {
		return fmt.Errorf("%q doesn't look like <arch>-<os>[@<version>]", val)
	}
	t.arch = parts[0]
	t.opsys = parts[1]
	t.isSet = true
	return nil
}

// Get implements flag.Getter.
func (t Target) Get() interface{} {
	if !t.isSet {
		// Default value.
		arch, isSet := goarch()
		return Target{
			isSet:   isSet,
			arch:    arch,
			opsys:   runtime.GOOS,
			version: t.version,
			Env:     t.Env,
		}
	}
	return t
}

func goarch() (string, bool) {
	// GOARCH may be set to 386 for binaries compiled for amd64 - i.e.
	// the same binary can be run in these two modes, but the compiled
	// in value of runtime.GOARCH will only ever be the value that it
	// was compiled with.
	if a := os.Getenv("GOARCH"); len(a) > 0 {
		return a, true
	}
	return runtime.GOARCH, false
}

// DefaultTarget returns a default value for a Target. Use this function to
// initialize Targets that are expected to set from the command line via
// the flags package.
func DefaultTarget() Target {
	arch, isSet := goarch()
	return Target{
		isSet: isSet,
		arch:  arch,
		opsys: runtime.GOOS,
	}
}

// NativeTarget returns a value for Target for the host on which it is running.
// Use this function for Target values that are passed into other functions
// and libraries where a native target is specifically required.
func NativeTarget() Target {
	arch, _ := goarch()
	return Target{
		isSet: true,
		arch:  arch,
		opsys: runtime.GOOS,
	}
}

// IsSet returns true if this target has had its value set.
func (pt Target) IsSet() bool {
	return pt.isSet
}

// String implements flag.Getter.
func (pt Target) String() string {
	v := pt.Get().(Target)
	return fmt.Sprintf("%v-%v@%s", v.arch, v.opsys, v.version)
}

// Targets is a list of *Target's ordered by architecture,
// operating system and descending versions.
type Targets []*Target

// Implements sort.Len
func (tl Targets) Len() int {
	return len(tl)
}

// Implements sort.Less
func (tl Targets) Less(i, j int) bool {
	return tl[i].Less(tl[j])
}

// Implements sort.Swap
func (tl Targets) Swap(i, j int) {
	tl[i], tl[j] = tl[i], tl[j]
}

func (tl Targets) Sort() {
	sort.Sort(tl)
}

// DebugString returns a pretty-printed representation of pt.
func (pt Target) DebugString() string {
	v := pt.Get().(Target)
	return fmt.Sprintf("%v-%v@%s dir:%s --env=%s envvars:%v", v.arch, v.opsys, v.version, pt.InstallationDir, strings.Join(pt.commandLineEnv.Vars, ","), pt.Env.Vars)
}

// Set implements flag.Getter.
func (e *Environment) Get() interface{} {
	return *e
}

// Set implements flag.Value.
func (e *Environment) Set(val string) error {
	for _, v := range strings.Split(val, ",") {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 || (len(parts[0]) == 0) {
			return fmt.Errorf("%q doesn't look like var=[val]", v)
		}
		e.Vars = append(e.Vars, v)
	}
	return nil
}

// String implements flag.Getter.
func (e Environment) String() string {
	return strings.Join(e.Vars, ",")
}

// Usage returns the usage string for Environment.
func (e Environment) Usage() string {
	return "specify an environment variable in the form: <var>=[<val>],..."
}

// InsertTarget inserts the given target into Targets if it's not
// already there and returns a new slice.
func InsertTarget(targets Targets, target *Target) Targets {
	for i, t := range targets {
		if !t.Less(target) {
			targets = append(targets, nil)
			copy(targets[i+1:], targets[i:])
			targets[i] = target
			return targets
		}
	}
	return append(targets, target)
}

// RemoveTarget removes the given target from a slice of Target and returns
// a slice.
func RemoveTarget(targets Targets, target *Target) Targets {
	for i, t := range targets {
		if target.Match(t) {
			targets, targets[len(targets)-1] = append(targets[:i], targets[i+1:]...), nil
			return targets
		}
	}
	return targets
}

// FindTarget returns the first target that matches the requested target from
// the slice of Targets. If target has not been explicitly set and there is
// only a single target available in targets then that one target is considered
// as matching.
func FindTarget(targets Targets, target *Target) *Target {
	for _, t := range targets {
		if target.Match(t) {
			tmp := *t
			return &tmp
		}
	}
	return nil
}

// FindTargetWithDefault is like FindTarget except that if there is only one
// target in the slice and the requested target has not been explicitly set
// (IsSet is false) then that one target is returned by default.
func FindTargetWithDefault(targets Targets, target *Target) *Target {
	if len(targets) == 1 && !target.IsSet() {
		tmp := *targets[0]
		return &tmp
	}
	return FindTarget(targets, target)
}

// compareVersions compares version numbers.  It handles cases like:
// compareVersions("2", "11") => 1
// compareVersions("1.1", "1.2") => 1
// compareVersions("1.2", "1.2.1") => 1
// compareVersions("1.2.1.b", "1.2.1.c") => 1
func compareVersions(v1, v2 string) int {
	v1parts := strings.Split(v1, ".")
	v2parts := strings.Split(v2, ".")

	maxLen := len(v1parts)
	if len(v2parts) > maxLen {
		maxLen = len(v2parts)
	}

	for i := 0; i < maxLen; i++ {
		if i == len(v1parts) {
			// v2 has more parts than v1, so v2 > v1.
			return -1
		}
		if i == len(v2parts) {
			// v1 has more parts than v2, so v1 > v2.
			return 1
		}

		mustCompareStrings := false
		v1part, err := strconv.Atoi(v1parts[i])
		if err != nil {
			mustCompareStrings = true
		}
		v2part, err := strconv.Atoi(v2parts[i])
		if err != nil {
			mustCompareStrings = true
		}
		if mustCompareStrings {
			return strings.Compare(v1parts[i], v2parts[i])
		}

		if v1part > v2part {
			return 1
		}
		if v2part > v1part {
			return -1
		}
	}
	return 0
}
