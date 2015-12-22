// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profilesreader

import "testing"

func TestAppendJiriProfile(t *testing.T) {
	p := InitProfilesFromFlag("foo", DoNotAppendJiriProfile)
	if got, want := p, []string{"foo"}; len(got) != 1 || got[0] != "foo" {
		t.Errorf("got %v, want %v", got, want)
	}
	p = InitProfilesFromFlag("foo", AppendJiriProfile)
	if got, want := p, []string{"foo", "jiri"}; len(got) != 2 || got[0] != "foo" || got[1] != "jiri" {
		t.Errorf("got %v, want %v", got, want)
	}
}
