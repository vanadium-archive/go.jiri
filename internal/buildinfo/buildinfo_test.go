// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package buildinfo

import (
	"reflect"
	"testing"
	"time"

	"v.io/jiri/internal/project"
	"v.io/x/lib/metadata"
)

var (
	allTests = []struct {
		BuildInfo T
		MetaData  *metadata.T
	}{
		{
			BuildInfo: T{
				Manifest: project.Manifest{Label: "foo"},
				Platform: "platform",
				Pristine: true,
				Time:     time.Date(2015, time.May, 3, 3, 15, 0, 0, time.UTC),
				User:     "user",
			},
			MetaData: metadata.FromMap(map[string]string{
				"build.Manifest": `<manifest label="foo">
  <hooks></hooks>
  <imports></imports>
  <projects></projects>
  <tools></tools>
</manifest>`,
				"build.Platform": "platform",
				"build.Pristine": "true",
				"build.Time":     "2015-05-03T03:15:00Z",
				"build.User":     "user",
			}),
		},
		{
			BuildInfo: T{
				Manifest: project.Manifest{Label: "bar"},
				Platform: "amd64unknown-linux-unknown",
				Pristine: false,
				Time:     time.Unix(0, 0).UTC(),
				User:     "Vanadium Vamoose",
			},
			MetaData: metadata.FromMap(map[string]string{
				"build.Manifest": `<manifest label="bar">
  <hooks></hooks>
  <imports></imports>
  <projects></projects>
  <tools></tools>
</manifest>`,
				"build.Platform": "amd64unknown-linux-unknown",
				"build.Pristine": "false",
				"build.Time":     "1970-01-01T00:00:00Z",
				"build.User":     "Vanadium Vamoose",
			}),
		},
	}
)

func TestToMetaData(t *testing.T) {
	for _, test := range allTests {
		md, err := test.BuildInfo.ToMetaData()
		if err != nil {
			t.Errorf("%#v ToMetaData failed: %v", test.BuildInfo, err)
		}
		if got, want := md, test.MetaData; !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	}
}

func TestFromMetaData(t *testing.T) {
	for _, test := range allTests {
		bi, err := FromMetaData(test.MetaData)
		if err != nil {
			t.Errorf("%#v FromMetaData failed: %v", test.MetaData, err)
		}
		if got, want := bi, test.BuildInfo; !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	}
}
