// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles

import (
	"bytes"
	"fmt"
	"reflect"
	"sort"
)

// VersionInfo represents the supported and default versions offered
// by a profile and a map of versions to arbitrary metadata used internally
// by the profile implementation.
type VersionInfo struct {
	name           string
	data           map[string]interface{}
	ordered        []string
	defaultVersion string
}

// NewVersionInfo creates a new instance of VersionInfo from the supplied
// map of supported versions and their associated metadata. The keys for
// this map are the supported version strings. The supplied defaultVersion
// will be used whenever a specific version is not requested.
func NewVersionInfo(name string, supported map[string]interface{}, defaultVersion string) *VersionInfo {
	vi := &VersionInfo{}
	vi.name = name
	vi.data = make(map[string]interface{}, len(supported))
	vi.ordered = make([]string, 0, len(supported))
	for k, v := range supported {
		vi.data[k] = v
		vi.ordered = append(vi.ordered, k)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(vi.ordered)))
	vi.defaultVersion = defaultVersion
	return vi
}

// Lookup returns the metadata associated with the requested version.
func (vi *VersionInfo) Lookup(version string, to interface{}) error {
	if len(version) == 0 {
		version = vi.defaultVersion
	}
	from, present := vi.data[version]
	if !present {
		return fmt.Errorf("unsupported version: %q for %s", version, vi)
	}
	// The stored value may or may not be a pointer.
	fromV := reflect.Indirect(reflect.ValueOf(from))

	// Make sure that the type of the stored value is assignable to the
	// type of pointer passed in.
	fromVT := fromV.Type()
	if !fromVT.AssignableTo(reflect.TypeOf(to).Elem()) {
		return fmt.Errorf("mismatched types: %T not assignable to %T", from, to)
	}
	toV := reflect.ValueOf(to).Elem()
	toV.Set(fromV)
	return nil
}

// Select selects a version from the available ones that best matches
// the requested one. If requested is the emtpy string then the default
// version is returned, otherwise an exact match is required.
func (vi *VersionInfo) Select(requested string) (string, error) {
	if len(requested) == 0 {
		return vi.defaultVersion, nil
	}
	for _, version := range vi.ordered {
		if requested == version {
			return requested, nil
		}
	}
	return "", fmt.Errorf("unsupported version: %q for %s", requested, vi)
}

// String returns a string representation of the VersionInfo, in particular
// it lists all of the supported versions with an asterisk next to the
// default.
func (vi *VersionInfo) String() string {
	r := bytes.Buffer{}
	r.WriteString(vi.name + ":")
	for _, v := range vi.ordered {
		r.WriteString(" " + v)
		if v == vi.defaultVersion {
			r.WriteString("*")
		}
	}
	return r.String()
}

// Default returns the default version.
func (vi *VersionInfo) Default() string {
	return vi.defaultVersion
}

// Supported returns the set of supported versions.
func (vi *VersionInfo) Supported() []string {
	r := make([]string, len(vi.ordered))
	copy(r, vi.ordered)
	return r
}

// IsNewerThanDefault returns true if the supplied version is newer than the
// default.
func (vi *VersionInfo) IsNewerThanDefault(version string) bool {
	return vi.defaultVersion < version
}

// IsOlderThanDefault returns true if the supplied version is older than the
// default.
func (vi *VersionInfo) IsOlderThanDefault(version string) bool {
	return vi.defaultVersion > version
}
