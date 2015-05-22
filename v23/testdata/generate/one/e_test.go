// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package one_test

import (
	"fmt"
	"os"
	"testing"

	"v.io/x/ref/test/modules"
	"v.io/x/ref/test/v23tests"
)

func V23TestOneA(i *v23tests.T) {}

func V23TestOneB(i *v23tests.T) {}

var modulesOneExt = modules.Register(func(env *modules.Env, args ...string) error {
	fmt.Fprintln(env.Stdout, "modulesOneExt")
	return nil
}, "modulesOneExt")

var modulesTwoExt = modules.Register(func(env *modules.Env, args ...string) error {
	fmt.Fprintln(env.Stdout, "modulesTwoExt")
	return nil
}, "modulesTwoExt")

func TestModulesOneExt(t *testing.T) {
	sh, err := modules.NewShell(nil, nil, false, t)
	if err != nil {
		t.Fatal(err)
	}
	for _, prog := range []modules.Program{modulesOneExt, modulesTwoExt} {
		m, err := sh.Start(nil, prog)
		if err != nil {
			if m != nil {
				m.Shutdown(os.Stderr, os.Stderr)
			}
			t.Fatal(err)
		}
		switch prog {
		case modulesOneExt:
			m.Expect("modulesOneExt")
		case modulesTwoExt:
			m.Expect("modulesTwoExt")
		}
	}
}
