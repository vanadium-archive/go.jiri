// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package buildinfo implements encoding and decoding of build
// metadata injected into binaries via the jiri tool.
package buildinfo

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"time"

	"v.io/jiri/internal/project"
	"v.io/x/lib/metadata"
)

// T describes build related metadata.
type T struct {
	// Manifest records the project manifest that identifies the state of Vanadium
	// projects used for the build.
	Manifest project.Manifest
	// Platform records the target platform of the build.
	Platform string
	// Pristine records whether the build was executed using pristine master
	// branches of Vanadium projects (or not).
	Pristine bool
	// Time records the time of the build.
	Time time.Time
	// User records the name of user who executed the build.
	User string
}

// ToMetaData encodes build info t into metadata md.
func (t T) ToMetaData() (*metadata.T, error) {
	md := new(metadata.T)
	manifest, err := xml.MarshalIndent(t.Manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("MarshalIndent(%v) failed: %v", t.Manifest, err)
	}
	md.Insert("build.Manifest", string(manifest))
	md.Insert("build.Platform", t.Platform)
	md.Insert("build.Pristine", strconv.FormatBool(t.Pristine))
	md.Insert("build.Time", t.Time.UTC().Format(time.RFC3339))
	md.Insert("build.User", t.User)
	return md, nil
}

// FromMetaData decodes metadata md and returns the build info.
func FromMetaData(md *metadata.T) (T, error) {
	var t T
	var err error
	if manifest := md.Lookup("build.Manifest"); manifest != "" {
		if err := xml.Unmarshal([]byte(manifest), &t.Manifest); err != nil {
			return T{}, fmt.Errorf("Unmarshal(%v) failed: %v", manifest, err)
		}
	}
	t.Platform = md.Lookup("build.Platform")
	if pristine := md.Lookup("build.Pristine"); pristine != "" {
		if t.Pristine, err = strconv.ParseBool(pristine); err != nil {
			return T{}, fmt.Errorf("ParseBool(%q) failed: %v", pristine, err)
		}
	}
	if buildtime := md.Lookup("build.Time"); buildtime != "" {
		if t.Time, err = time.Parse(time.RFC3339, buildtime); err != nil {
			return T{}, fmt.Errorf("Parse(%q) failed: %v", buildtime, err)
		}
	}
	t.User = md.Lookup("build.User")
	return t, nil
}
