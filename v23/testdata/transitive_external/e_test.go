// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package transitive_external_test

import (
	"testing"

	_ "v.io/x/ref/runtime/factories/generic"
	"v.io/x/ref/test/v23tests"

	"v.io/x/devtools/v23/testdata/transitive_external"
)

var cmd = "moduleInternalOnly"

func TestModulesExternal(t *testing.T) {
	transitive_external.Module(t)
}

func V23TestOneA(i *v23tests.T) {}
