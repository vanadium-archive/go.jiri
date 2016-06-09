// Copyright 2016 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gerrit

import (
	"reflect"
	"testing"
)

func checkMultiPartCLSet(t *testing.T, expectedTotal int, expectedCLsByPart map[int]Change, set *MultiPartCLSet) {
	if expectedTotal != set.expectedTotal {
		t.Fatalf("total: want %v, got %v", expectedTotal, set.expectedTotal)
	}
	if !reflect.DeepEqual(expectedCLsByPart, set.parts) {
		t.Fatalf("clsByPart: want %+v, got %+v", expectedCLsByPart, set.parts)
	}
}

func TestMultiPartCLSet(t *testing.T) {
	set := NewMultiPartCLSet()
	checkMultiPartCLSet(t, -1, map[int]Change{}, set)

	// Add a non-multipart cl.
	cl := GenCL(1000, 1, "release.go.core")
	if err := set.AddCL(cl); err == nil {
		t.Fatalf("expected AddCL(%v) to fail and it did not", cl)
	}
	checkMultiPartCLSet(t, -1, map[int]Change{}, set)

	// Add a multi part cl.
	cl.MultiPart = &MultiPartCLInfo{
		Topic: "test",
		Index: 1,
		Total: 2,
	}
	if err := set.AddCL(cl); err != nil {
		t.Fatalf("AddCL(%v) failed: %v", cl, err)
	}
	checkMultiPartCLSet(t, 2, map[int]Change{
		1: cl,
	}, set)

	// Test incomplete.
	if expected, got := false, set.Complete(); expected != got {
		t.Fatalf("want %v, got %v", expected, got)
	}

	// Add another multi part cl with the wrong "Total" number.
	cl2 := GenMultiPartCL(1050, 2, "release.js.core", "test", 2, 3)
	if err := set.AddCL(cl2); err == nil {
		t.Fatalf("expected AddCL(%v) to fail and it did not", cl)
	}
	checkMultiPartCLSet(t, 2, map[int]Change{
		1: cl,
	}, set)

	// Add another multi part cl with duplicated "Index" number.
	cl3 := GenMultiPartCL(1052, 2, "release.js.core", "Test", 1, 2)
	if err := set.AddCL(cl3); err == nil {
		t.Fatalf("expected AddCL(%v) to fail and it did not", cl)
	}
	checkMultiPartCLSet(t, 2, map[int]Change{
		1: cl,
	}, set)

	// Add another multi part cl with the wrong "Topic".
	cl4 := GenMultiPartCL(1062, 2, "release.js.core", "test123", 1, 2)
	if err := set.AddCL(cl4); err == nil {
		t.Fatalf("expected AddCL(%v) to fail and it did not", cl)
	}
	checkMultiPartCLSet(t, 2, map[int]Change{
		1: cl,
	}, set)

	// Add a valid multi part cl.
	cl5 := GenMultiPartCL(1072, 2, "release.js.core", "test", 2, 2)
	if err := set.AddCL(cl5); err != nil {
		t.Fatalf("AddCL(%v) failed: %v", cl, err)
	}
	checkMultiPartCLSet(t, 2, map[int]Change{
		1: cl,
		2: cl5,
	}, set)

	// Test complete.
	if expected, got := true, set.Complete(); expected != got {
		t.Fatalf("want %v, got %v", expected, got)
	}

	// Test cls.
	if expected, got := (CLList{cl, cl5}), set.CLs(); !reflect.DeepEqual(expected, got) {
		t.Fatalf("want %v, got %v", expected, got)
	}
}

func TestNewOpenCLs(t *testing.T) {
	nonMultiPartCLs := CLList{
		GenCL(1010, 1, "release.go.core"),
		GenCL(1020, 2, "release.go.tools"),
		GenCL(1030, 3, "release.js.core"),

		GenMultiPartCL(1000, 1, "release.js.core", "T1", 1, 2),
		GenMultiPartCL(1001, 1, "release.go.core", "T1", 2, 2),
		GenMultiPartCL(1002, 2, "release.go.core", "T2", 2, 2),
		GenMultiPartCL(1001, 2, "release.go.core", "T1", 2, 2),
	}
	multiPartCLs := CLList{
		// Multi part CLs.
		// The first two form a complete set for topic T1.
		// The third one looks like the second one, but has a different topic.
		// The last one has a larger patchset than the second one.
		GenMultiPartCL(1000, 1, "release.js.core", "T1", 1, 2),
		GenMultiPartCL(1001, 1, "release.go.core", "T1", 2, 2),
		GenMultiPartCL(1002, 2, "release.go.core", "T2", 2, 2),
		GenMultiPartCL(1001, 2, "release.go.core", "T1", 2, 2),
	}

	type testCase struct {
		prevCLsMap CLRefMap
		curCLs     CLList
		expected   []CLList
	}
	testCases := []testCase{
		////////////////////////////////
		// Tests for non-multipart CLs.

		// Both prevCLsMap and curCLs are empty.
		testCase{
			prevCLsMap: CLRefMap{},
			curCLs:     CLList{},
			expected:   []CLList{},
		},
		// prevCLsMap is empty, curCLs is not.
		testCase{
			prevCLsMap: CLRefMap{},
			curCLs:     CLList{nonMultiPartCLs[0], nonMultiPartCLs[1]},
			expected:   []CLList{CLList{nonMultiPartCLs[0]}, CLList{nonMultiPartCLs[1]}},
		},
		// prevCLsMap is not empty, curCLs is.
		testCase{
			prevCLsMap: CLRefMap{nonMultiPartCLs[0].Reference(): nonMultiPartCLs[0]},
			curCLs:     CLList{},
			expected:   []CLList{},
		},
		// prevCLsMap and curCLs are not empty, and they have overlapping refs.
		testCase{
			prevCLsMap: CLRefMap{
				nonMultiPartCLs[0].Reference(): nonMultiPartCLs[0],
				nonMultiPartCLs[1].Reference(): nonMultiPartCLs[1],
			},
			curCLs:   CLList{nonMultiPartCLs[1], nonMultiPartCLs[2]},
			expected: []CLList{CLList{nonMultiPartCLs[2]}},
		},
		// prevCLsMap and curCLs are not empty, and they have NO overlapping refs.
		testCase{
			prevCLsMap: CLRefMap{nonMultiPartCLs[0].Reference(): nonMultiPartCLs[0]},
			curCLs:     CLList{nonMultiPartCLs[1]},
			expected:   []CLList{CLList{nonMultiPartCLs[1]}},
		},

		////////////////////////////////
		// Tests for multi part CLs.

		// len(curCLs) > len(prevCLsMap).
		// And the CLs in curCLs have different topics.
		testCase{
			prevCLsMap: CLRefMap{multiPartCLs[0].Reference(): multiPartCLs[0]},
			curCLs:     CLList{multiPartCLs[0], multiPartCLs[2]},
			expected:   []CLList{},
		},
		// len(curCLs) > len(prevCLsMap).
		// And the CLs in curCLs form a complete multi part cls set.
		testCase{
			prevCLsMap: CLRefMap{multiPartCLs[0].Reference(): multiPartCLs[0]},
			curCLs:     CLList{multiPartCLs[0], multiPartCLs[1]},
			expected:   []CLList{CLList{multiPartCLs[0], multiPartCLs[1]}},
		},
		// len(curCLs) == len(prevCLsMap).
		// And cl[6] has a larger patchset than multiPartCLs[4] with identical cl number.
		testCase{
			prevCLsMap: CLRefMap{
				multiPartCLs[0].Reference(): multiPartCLs[0],
				multiPartCLs[1].Reference(): multiPartCLs[1],
			},
			curCLs:   CLList{multiPartCLs[0], multiPartCLs[3]},
			expected: []CLList{CLList{multiPartCLs[0], multiPartCLs[3]}},
		},

		////////////////////////////////
		// Tests for mixed.
		testCase{
			prevCLsMap: CLRefMap{
				multiPartCLs[0].Reference(): multiPartCLs[0],
				multiPartCLs[1].Reference(): multiPartCLs[1],
			},
			curCLs: CLList{nonMultiPartCLs[0], multiPartCLs[0], multiPartCLs[3]},
			expected: []CLList{
				CLList{nonMultiPartCLs[0]},
				CLList{multiPartCLs[0], multiPartCLs[3]},
			},
		},
	}

	for index, test := range testCases {
		got, errs := NewOpenCLs(test.prevCLsMap, test.curCLs)
		if !reflect.DeepEqual(test.expected, got) {
			t.Fatalf("case %d: want: %v, got: %v", index, test.expected, got)
		}
		if len(errs) != 0 {
			t.Fatalf("case %d: multi-part errors: ", index, errs)
		}
	}
}
