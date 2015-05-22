// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal_transitive_external

import (
	"os"
	"testing"

	"v.io/x/devtools/v23/testdata/generate/transitive/lib"
	_ "v.io/x/ref/runtime/factories/generic"
	"v.io/x/ref/test/modules"
)

func Module(t *testing.T) {
	sh, err := modules.NewShell(nil, nil, false, t)
	if err != nil {
		t.Fatal(err)
	}
	m, err := sh.Start(nil, lib.Prog)
	if err != nil {
		if m != nil {
			m.Shutdown(os.Stderr, os.Stderr)
		}
		t.Fatal(err)
	}
	m.Expect(lib.ProgOut)
}

func TestInternal(t *testing.T) {}
