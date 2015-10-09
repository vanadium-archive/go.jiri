// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

// Target represents specification for the environment that the profile is
// to be built for. Target may be named (via the Tag field), see the Match
// method for a definition of how Targets are compated.
// Targets include a version string to allow for upgrades and for
// the simultaneous existence of incompatible versions.
//
// Target and Environment implement flag.Getter so that they may be used
// with the flag package. Two flags are required, one to specify the target
// in <tag>=<arch>-<os> format and a second to specify environment variables
// either as comma separated values or as repeated arguments.
type Target struct {
	Tag, Arch, OS   string
	Env             Environment
	InstallationDir string // where this target is installed.
	Version         string
	UpdateTime      time.Time
	isSet           bool
}

type Environment struct {
	Vars []string `xml:"var"`
}

func NewTarget(target string) (Target, error) {
	t := &Target{}
	err := t.Set(target)
	return *t, err
}

// Match returns true if pt and pt2 meet the following criteria in the
// order they are listed:
// - if both targets have a non-zero length Tag field that is
//   identical
// - if the Arch and OS fields are exactly the same
// - if pt has a non-zero length Version field, then it must be
//   the same as that in pt2
// Match is used by the various methods and functions in this package
// when looking up Targets unless otherwise specified.
func (pt Target) Match(pt2 *Target) bool {
	if len(pt.Tag) > 0 && len(pt2.Tag) > 0 {
		return pt.Tag == pt2.Tag
	}
	if pt.Arch != pt2.Arch || pt.OS != pt2.OS {
		return false
	}
	if (len(pt.Version) > 0) && (pt.Version != pt2.Version) {
		return false
	}
	return true
}

// CrossCompiling returns true if the target differs from that of
// the runtime.
func (pt Target) CrossCompiling() bool {
	arch, _ := goarch()
	return (pt.Arch != arch) || (pt.OS != runtime.GOOS)
}

// Usage returns the usage string for Target.
func (pt *Target) Usage() string {
	return "specifies a profile target in the following form: [<tag>=]<arch>-<os>"
}

// Set implements flag.Value.
func (t *Target) Set(val string) error {
	index := strings.IndexByte(val, '=')
	tag := ""
	if index > -1 {
		tag = val[0:index]
		val = val[index+1:]
	} else {
		if strings.IndexByte(val, '-') < 0 {
			t.Tag = val
			t.Arch = ""
			t.OS = ""
			t.isSet = true
			return nil
		}
	}
	parts := strings.Split(val, "-")
	if len(parts) != 2 || (len(parts[0]) == 0 || len(parts[1]) == 0) {
		return fmt.Errorf("%q doesn't look like [tag=]<arch>-<os>", val)
	}
	t.Tag = tag
	t.Arch = parts[0]
	t.OS = parts[1]
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
			Tag:     "",
			Arch:    arch,
			OS:      runtime.GOOS,
			Version: t.Version,
			Env:     t.Env,
		}
	}
	return t
}

func goarch() (string, bool) {
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
		Tag:   "",
		Arch:  arch,
		OS:    runtime.GOOS,
	}
}

// NativeTarget returns a value for Target for the host on which it is running.
// Use this function for Target values that are passed into other functions
// and libraries where a native target is specifically required.
func NativeTarget() Target {
	arch, _ := goarch()
	return Target{
		isSet: true,
		Tag:   "",
		Arch:  arch,
		OS:    runtime.GOOS,
	}
}

// IsSet returns true if this target has had its value set.
func (pt Target) IsSet() bool {
	return pt.isSet
}

// String implements flag.Getter.
func (pt Target) String() string {
	v := pt.Get().(Target)
	return fmt.Sprintf("%v=%v-%v@%s dir:%s env:%s", v.Tag, v.Arch, v.OS, v.Version, pt.InstallationDir, pt.Env.Vars)
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
func (e *Environment) String() string {
	return strings.Join(e.Vars, ",")
}

// Usage returns the usage string for Environment.
func (e *Environment) Usage() string {
	return "specifcy an environment variable in the form: <var>=[<val>],..."
}

// AddTarget adds the given target to a slice of Target if it's not
// already there and returns a new slice.
func AddTarget(targets []*Target, target *Target) []*Target {
	for _, t := range targets {
		if target.Match(t) {
			return targets
		}
	}
	return append(targets, target)
}

// RemoveTarget removes the given target from a slice of Target and returns
// a slice.
func RemoveTarget(targets []*Target, target *Target) []*Target {
	for i, t := range targets {
		if target.Match(t) {
			return append(targets[:i], targets[i+1:]...)
		}
	}
	return targets
}

// FindTarget returns the first target that matches the requested target from
// the slice of Targets; note that the requested target need only include a
// tag name. It returns nil if the requested target does not exist. If there
// is only one target in the slice and the requested target has not been
// explicitly set (IsSet is false) then that one target is returned by default.
func FindTarget(targets []*Target, target *Target) *Target {
	if len(targets) == 1 && !target.IsSet() {
		tmp := *targets[0]
		return &tmp
	}
	for _, t := range targets {
		if target.Match(t) {
			tmp := *t
			return &tmp
		}
	}
	return nil
}

// FindTargetByTag searches targets to see if any have the same
// tag as the target parameter, and if so, return that target.
func FindTargetByTag(targets []*Target, target *Target) *Target {
	if len(target.Tag) == 0 {
		return nil
	}
	for _, t := range targets {
		if target.Tag == t.Tag {
			return t
		}
	}
	return nil
}
