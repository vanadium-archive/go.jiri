// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lib

import (
	"fmt"

	_ "v.io/x/ref/runtime/factories/generic"
	"v.io/x/ref/test/modules"
)

const ProgOut = "transitiveLibProg"

var Prog = modules.Register(func(env *modules.Env, args ...string) error {
	fmt.Fprintln(env.Stdout, ProgOut)
	return nil
}, ProgOut)
