// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY
package modules_and_v23_test

import "testing"

import "v.io/core/veyron/lib/modules"
import "v.io/core/veyron/lib/testutil/v23tests"

func init() {
	modules.RegisterChild("modulesModulesAndV23Ext", ``, modulesModulesAndV23Ext)
}

func TestV23ModulesAndV23A(t *testing.T) {
	v23tests.RunTest(t, V23TestModulesAndV23A)
}

func TestV23ModulesAndV23B(t *testing.T) {
	v23tests.RunTest(t, V23TestModulesAndV23B)
}
