// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package jiri

import (
	"path/filepath"
	"testing"
)

// TestRelPath checks that RelPath methods return the correct values, and in
// particular that Abs correctly roots the path at the given X's Root.
func TestRelPath(t *testing.T) {
	root1 := "/path/to/jiri-root"
	x := &X{
		Root: root1,
	}

	rp1 := NewRelPath("foo")
	if got, want := string(rp1), "foo"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := rp1.Abs(x), filepath.Join(root1, "foo"); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := rp1.Symbolic(), "${"+RootEnv+"}"+string(filepath.Separator)+"foo"; got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	rp2 := rp1.Join("a", "b")
	if got, want := string(rp2), filepath.Join("foo", "a", "b"); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := rp2.Abs(x), filepath.Join(root1, "foo", "a", "b"); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := rp2.Symbolic(), "${"+RootEnv+"}"+string(filepath.Separator)+filepath.Join("foo", "a", "b"); got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	// Check Abs with different x.Root.
	root2 := "/different/path"
	x.Root = root2
	if got, want := rp1.Abs(x), filepath.Join(root2, "foo"); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := rp2.Abs(x), filepath.Join(root2, "foo", "a", "b"); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}
