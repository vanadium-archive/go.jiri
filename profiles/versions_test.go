// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles_test

import (
	"testing"

	"v.io/jiri/profiles"
)

func cmp(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
}

func TestVersionInfo(t *testing.T) {
	type spec struct {
		v string
	}
	vi := profiles.NewVersionInfo("test", map[string]interface{}{
		"3": "3x",
		"5": "5x",
		"4": "4x",
		"6": &spec{"6"},
	}, "3")

	if got, want := vi.Supported(), []string{"6", "5", "4", "3"}; !cmp(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := vi.Default(), "3"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	var data string
	if err := vi.Lookup("4", &data); err != nil {
		t.Fatal(err)
	}
	if got, want := data, "4x"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	var bad bool
	if err := vi.Lookup("4", &bad); err == nil || err.Error() != "mismatched types: string not assignable to *bool" {
		t.Errorf("missing or wrong error: %v", err)
	}

	var s spec
	if err := vi.Lookup("6", &s); err != nil {
		t.Fatal(err)
	}
	if got, want := s.v, "6"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	ver, err := vi.Select("")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ver, "3"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	ver, err = vi.Select("5")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := ver, "5"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	if ver, err := vi.Select("2"); ver != "" || err.Error() != "unsupported version: \"2\" for 6 5 4 3*" {
		t.Errorf("failed to detect unsupported version: %q", err)
	}
}
