// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package profiles

// Clear resets the current database and is intended for use from tests only.
func Clear() {
	db = newDB()
}
