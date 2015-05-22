// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modules_only_test

import (
	"fmt"
	"os"
	"testing"

	_ "v.io/x/ref/runtime/factories/generic"
	"v.io/x/ref/test/modules"
)

var modulesOnlyExt = modules.Register(func(env *modules.Env, args ...string) error {
	fmt.Fprintln(env.Stdout, "modulesOnlyExt")
	return nil
}, "modulesOnlyExt")

func TestModulesOnlyExt(t *testing.T) {
	sh, err := modules.NewShell(nil, nil, false, t)
	if err != nil {
		t.Fatal(err)
	}
	m, err := sh.Start(nil, modulesOnlyExt)
	if err != nil {
		if m != nil {
			m.Shutdown(os.Stderr, os.Stderr)
		}
		t.Fatal(err)
	}
	m.Expect("modulesOnlyExt")
}
