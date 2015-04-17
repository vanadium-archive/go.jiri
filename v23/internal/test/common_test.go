// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"regexp"
	"testing"
)

func TestOutputRegex(t *testing.T) {
	testCases := []struct {
		re    *regexp.Regexp
		name  string
		match bool
	}{
		{
			re:    reJSResult,
			name:  "abc_integration.out",
			match: true,
		},
		{
			re:    reJSResult,
			name:  "abc_integration.out2",
			match: false,
		},
		{
			re:    reJSResult,
			name:  "abc_inte.out",
			match: false,
		},
		{
			re:    reTestResult,
			name:  "tests_abc.xml",
			match: true,
		},
		{
			re:    reTestResult,
			name:  "status_abc.json",
			match: true,
		},
		{
			re:    reTestResult,
			name:  "tests_abc.json",
			match: false,
		},
		{
			re:    reTestResult,
			name:  "status_abc.xml",
			match: false,
		},
	}

	for _, test := range testCases {
		if got, want := test.re.MatchString(test.name), test.match; got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
