// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package tool contains abstractions for working with developer
// tools. In particular:
//
// 1) It contains global variables that can be used to store
// attributes of a tool. Automated builds can set these values to
// something meaningful as follows:
//
// go build -ldflags "-X v.io/x/devtools/internal/tool.<key> <value>" v.io/x/devtools/<tool>
//
// 2) It provides the Context type, which encapsulates the state and
// abstractions commonly accessed throughout the lifetime of a tool
// invocation.
package tool
