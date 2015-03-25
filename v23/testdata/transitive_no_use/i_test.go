// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transitive_nouse

import (
	"testing"

	_ "v.io/x/ref/profiles"

	"v.io/x/devtools/v23/testdata/transitive/middle"
)

var cmd = "moduleInternalOnly"

func init() {
	middle.Init()
}

func TestWithoutModules(t *testing.T) {
}
