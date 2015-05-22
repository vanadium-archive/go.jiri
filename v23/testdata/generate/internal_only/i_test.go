// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package internal_only

import (
	"fmt"
	"os"
	"testing"

	_ "v.io/x/ref/runtime/factories/generic"
	"v.io/x/ref/test/modules"
)

var moduleInternalOnly = modules.Register(func(env *modules.Env, args ...string) error {
	fmt.Fprintln(env.Stdout, "moduleInternalOnly")
	return nil
}, "moduleInternalOnly")

func TestModulesInternalOnly(t *testing.T) {
	sh, err := modules.NewShell(nil, nil, false, t)
	if err != nil {
		t.Fatal(err)
	}
	m, err := sh.Start(nil, moduleInternalOnly)
	if err != nil {
		if m != nil {
			m.Shutdown(os.Stderr, os.Stderr)
		}
		t.Fatal(err)
	}
	m.Expect("moduleInternalOnly")
}
