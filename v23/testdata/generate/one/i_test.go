// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package one

import (
	"fmt"
	"os"
	"testing"

	_ "v.io/x/ref/runtime/factories/generic"
	"v.io/x/ref/test/modules"
)

// TestV23TestMain is not picked up as a V23 test, since it doesn't start with
// the V23Test prefix.
func TestV23TestMain(t *testing.T) {}

var modulesOneInt = modules.Register(func(env *modules.Env, args ...string) error {
	fmt.Fprintln(env.Stdout, "modulesOneInt")
	return nil
}, "modulesOneInt")

var modulesTwoInt = modules.Register(func(env *modules.Env, args ...string) error {
	fmt.Fprintln(env.Stdout, "modulesTwoInt")
	return nil
}, "modulesTwoInt")

func TestModulesOneAndTwo(t *testing.T) {
	sh, err := modules.NewShell(nil, nil, false, t)
	if err != nil {
		t.Fatal(err)
	}
	for _, prog := range []modules.Program{modulesOneInt, modulesTwoInt} {
		m, err := sh.Start(nil, prog)
		if err != nil {
			if m != nil {
				m.Shutdown(os.Stderr, os.Stderr)
			}
			t.Fatal(err)
		}
		switch prog {
		case modulesOneInt:
			m.Expect("modulesOneInt")
		case modulesTwoInt:
			m.Expect("modulesTwoInt")
		}
	}
}
