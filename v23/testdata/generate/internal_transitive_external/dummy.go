// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Tests the case where modules are used by a package an external test but an
// internal test still has non-modules tests. The appropriate TestMain should
// be generated in the internal package.
package internal_transitive_external
