// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file was auto-generated via go generate.
// DO NOT UPDATE MANUALLY

package empty

import (
	"os"
	"testing"

	"v.io/x/ref/test"
)

func TestMain(m *testing.M) {
	test.Init()
	os.Exit(m.Run())
}
