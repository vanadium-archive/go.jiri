// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runutil

import "os"

// IsFNLHost returns true iff the host machine is running FNL
// TODO(bprosnitz) We should find a better way to detect that the machine is running FNL
// TODO(bprosnitz) This is needed in part because fnl is not currently a GOHOSTOS. This should
// probably be handled by having hosts that are separate from GOHOSTOSs similarly to how targets
// are defined.
func IsFNLHost() bool {
	return os.Getenv("FNL_SYSTEM") != ""
}
