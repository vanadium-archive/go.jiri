// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modules_and_v23

import (
	"fmt"
	"os"
	"testing"

	_ "v.io/x/ref/runtime/factories/generic"
	"v.io/x/ref/test/modules"
)

var modulesAndV23Int = modules.Register(func(env *modules.Env, args ...string) error {
	fmt.Fprintln(env.Stdout, "modulesAndV23Int")
	return nil
}, "modulesAndV23Int")

func TestModulesAndV23Int(t *testing.T) {
	sh, err := modules.NewShell(nil, nil, false, t)
	if err != nil {
		t.Fatal(err)
	}
	m, err := sh.Start(nil, modulesAndV23Int)
	if err != nil {
		if m != nil {
			m.Shutdown(os.Stderr, os.Stderr)
		}
		t.Fatal(err)
	}
	m.Expect("modulesAndV23Int")
}
