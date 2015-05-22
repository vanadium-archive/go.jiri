// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modules_and_v23_test

import (
	"fmt"
	"os"
	"testing"

	"v.io/x/ref/test/modules"
	"v.io/x/ref/test/v23tests"
)

func V23TestModulesAndV23A(i *v23tests.T) {}

func V23TestModulesAndV23B(i *v23tests.T) {}

var modulesAndV23Ext = modules.Register(func(env *modules.Env, args ...string) error {
	fmt.Fprintln(env.Stdout, "modulesAndV23Ext")
	return nil
}, "modulesAndV23Ext")

func TestModulesAndV23Ext(t *testing.T) {
	sh, err := modules.NewShell(nil, nil, false, t)
	if err != nil {
		t.Fatal(err)
	}
	m, err := sh.Start(nil, modulesAndV23Ext)
	if err != nil {
		if m != nil {
			m.Shutdown(os.Stderr, os.Stderr)
		}
		t.Fatal(err)
	}
	m.Expect("modulesAndV23Ext")
}
