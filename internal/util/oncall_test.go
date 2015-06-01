// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"v.io/x/devtools/internal/tool"
)

func createOncallFile(t *testing.T, ctx *tool.Context) {
	content := `<?xml version="1.0" ?>
<rotation>
  <shift>
    <primary>spetrovic</primary>
    <secondary>suharshs</secondary>
    <startDate>Nov 5, 2014 12:00:00 PM</startDate>
  </shift>
  <shift>
    <primary>suharshs</primary>
    <secondary>tilaks</secondary>
    <startDate>Nov 12, 2014 12:00:00 PM</startDate>
  </shift>
  <shift>
    <primary>jsimsa</primary>
    <secondary>toddw</secondary>
    <startDate>Nov 19, 2014 12:00:00 PM</startDate>
  </shift>
</rotation>`
	oncallRotationsFile, err := OncallRotationPath(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	dir := filepath.Dir(oncallRotationsFile)
	dirMode := os.FileMode(0700)
	if err := ctx.Run().MkdirAll(dir, dirMode); err != nil {
		t.Fatalf("MkdirAll(%q, %v) failed: %v", dir, dirMode, err)
	}
	fileMode := os.FileMode(0644)
	if err := ioutil.WriteFile(oncallRotationsFile, []byte(content), fileMode); err != nil {
		t.Fatalf("WriteFile(%q, %q, %v) failed: %v", oncallRotationsFile, content, fileMode, err)
	}
}

func TestOncall(t *testing.T) {
	ctx := tool.NewDefaultContext()
	root, err := NewFakeV23Root(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() {
		if err := root.Cleanup(ctx); err != nil {
			t.Fatalf("%v", err)
		}
	}()
	oldRoot, err := V23Root()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.Setenv("V23_ROOT", root.Dir); err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Setenv("V23_ROOT", oldRoot)

	// Create a oncall.v1.xml file.
	createOncallFile(t, ctx)
	type testCase struct {
		targetTime     time.Time
		expectedOncall string
	}
	testCases := []testCase{
		testCase{
			targetTime:     time.Date(2013, time.November, 5, 12, 0, 0, 0, time.Local),
			expectedOncall: "",
		},
		testCase{
			targetTime:     time.Date(2014, time.November, 5, 12, 0, 0, 0, time.Local),
			expectedOncall: "spetrovic",
		},
		testCase{
			targetTime:     time.Date(2014, time.November, 5, 14, 0, 0, 0, time.Local),
			expectedOncall: "spetrovic",
		},
		testCase{
			targetTime:     time.Date(2014, time.November, 20, 14, 0, 0, 0, time.Local),
			expectedOncall: "jsimsa",
		},
	}
	for _, test := range testCases {
		got, err := Oncall(ctx, test.targetTime)
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if test.expectedOncall != got {
			t.Fatalf("want %v, got %v", test.expectedOncall, got)
		}
	}
}
