// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package foo_test

import (
	"testing"

	"v.io/x/devtools/v23/internal/test/testdata/foo"
)

func Test1(t *testing.T) {
	if foo.Foo() != "hello" {
		t.Fatalf("that's rude")
	}
}
