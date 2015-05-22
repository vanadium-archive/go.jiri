// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

package v23_only_test

import (
	"os"
	"testing"

	"v.io/x/ref/test"
	"v.io/x/ref/test/modules"
	"v.io/x/ref/test/v23tests"
)

func TestMain(m *testing.M) {
	test.Init()
	modules.DispatchAndExitIfChild()
	cleanup := v23tests.UseSharedBinDir()
	r := m.Run()
	cleanup()
	os.Exit(r)
}

func TestV23V23OnlyA(t *testing.T) {
	v23tests.RunTest(t, V23TestV23OnlyA)
}

func TestV23V23OnlyB(t *testing.T) {
	v23tests.RunTest(t, V23TestV23OnlyB)
}
