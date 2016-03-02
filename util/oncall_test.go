// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package util_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"v.io/jiri"
	"v.io/jiri/jiritest"
	"v.io/jiri/util"
)

func createOncallFile(t *testing.T, jirix *jiri.X) {
	content := `<?xml version="1.0" ?>
<rotation>
  <shift>
    <primary>spetrovic</primary>
    <secondary>suharshs</secondary>
    <startDate>Nov 5, 2014 12:00:00 PM</startDate>
  </shift>
  <shift>
    <primary>suharshs</primary>
    <secondary>jingjin</secondary>
    <startDate>Nov 12, 2014 12:00:00 PM</startDate>
  </shift>
  <shift>
    <primary>jsimsa</primary>
    <secondary>toddw</secondary>
    <startDate>Nov 19, 2014 12:00:00 PM</startDate>
  </shift>
</rotation>`
	oncallRotationsFile, err := util.OncallRotationPath(jirix)
	if err != nil {
		t.Fatalf("%v", err)
	}
	dir := filepath.Dir(oncallRotationsFile)
	dirMode := os.FileMode(0700)
	if err := jirix.NewSeq().MkdirAll(dir, dirMode).Done(); err != nil {
		t.Fatalf("MkdirAll(%q, %v) failed: %v", dir, dirMode, err)
	}
	fileMode := os.FileMode(0644)
	if err := ioutil.WriteFile(oncallRotationsFile, []byte(content), fileMode); err != nil {
		t.Fatalf("WriteFile(%q, %q, %v) failed: %v", oncallRotationsFile, content, fileMode, err)
	}
}

func TestOncall(t *testing.T) {
	fake, cleanup := jiritest.NewFakeJiriRoot(t)
	defer cleanup()

	// Create a oncall.v1.xml file.
	createOncallFile(t, fake.X)
	testCases := []struct {
		targetTime    time.Time
		expectedShift *util.OncallShift
	}{
		{
			time.Date(2013, time.November, 5, 12, 0, 0, 0, time.Local),
			nil,
		},
		{
			time.Date(2014, time.November, 5, 12, 0, 0, 0, time.Local),
			&util.OncallShift{
				Primary:   "spetrovic",
				Secondary: "suharshs",
				Date:      "Nov 5, 2014 12:00:00 PM",
			},
		},
		{
			time.Date(2014, time.November, 5, 14, 0, 0, 0, time.Local),
			&util.OncallShift{
				Primary:   "spetrovic",
				Secondary: "suharshs",
				Date:      "Nov 5, 2014 12:00:00 PM",
			},
		},
		{
			time.Date(2014, time.November, 20, 14, 0, 0, 0, time.Local),
			&util.OncallShift{
				Primary:   "jsimsa",
				Secondary: "toddw",
				Date:      "Nov 19, 2014 12:00:00 PM",
			},
		},
	}
	for _, test := range testCases {
		got, err := util.Oncall(fake.X, test.targetTime)
		if err != nil {
			t.Fatalf("want no errors, got: %v", err)
		}
		if !reflect.DeepEqual(test.expectedShift, got) {
			t.Fatalf("want %#v, got %#v", test.expectedShift, got)
		}
	}
}
