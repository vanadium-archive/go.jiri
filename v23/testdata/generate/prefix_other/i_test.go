// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package prefix_other

import (
	"fmt"
	"os"
	"testing"

	"v.io/x/ref/test/modules"
)

var cmd = modules.Register(func(env *modules.Env, args ...string) error {
	fmt.Fprintln(env.Stdout, "cmd")
	return nil
}, "cmd")

func TestInternalFilename(t *testing.T) {
	sh, err := modules.NewShell(nil, nil, false, t)
	if err != nil {
		t.Fatal(err)
	}
	m, err := sh.Start(nil, cmd)
	if err != nil {
		if m != nil {
			m.Shutdown(os.Stderr, os.Stderr)
		}
		t.Fatal(err)
	}
	m.Expect("cmd")
}
