// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modules_only_test

import (
	"fmt"
	"io"
	"os"
	"testing"

	_ "v.io/x/ref/runtime/factories/generic"
	"v.io/x/ref/test/modules"
)

var cmd = "moduleModulesOnlyExt"

// Oh..
func moduleModulesOnlyExt(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error {
	fmt.Fprintln(stdout, cmd)
	return nil
}

func TestModulesOnlyExt(t *testing.T) {
	sh, err := modules.NewShell(nil, nil, false, t)
	if err != nil {
		t.Fatal(err)
	}
	m, err := sh.Start(cmd, nil)
	if err != nil {
		if m != nil {
			m.Shutdown(os.Stderr, os.Stderr)
		}
		t.Fatal(err)
	}
	m.Expect(cmd)
}
