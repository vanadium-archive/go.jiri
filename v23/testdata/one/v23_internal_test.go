// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY
package one

import "fmt"
import "testing"
import "os"

import "v.io/core/veyron/lib/modules"
import "v.io/core/veyron/lib/testutil"
import "v.io/core/veyron/lib/testutil/v23tests"

func init() {
	modules.RegisterChild("modulesOneInt", `modulesOneInt does the following...
Usage: <a> <b>...`, modulesOneInt)
	modules.RegisterChild("modulesTwoInt", `modulesTwoInt does the following...
<ab> <cd>`, modulesTwoInt)
}

func TestMain(m *testing.M) {
	testutil.Init()
	if modules.IsModulesProcess() {
		if err := modules.Dispatch(); err != nil {
			fmt.Fprintf(os.Stderr, "modules.Dispatch failed: %v\n", err)
			os.Exit(1)
		}
		return
	}
	cleanup := v23tests.UseSharedBinDir()
	r := m.Run()
	cleanup()
	os.Exit(r)
}
