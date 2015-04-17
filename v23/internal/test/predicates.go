// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package test

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func is386() bool {
	return runtime.GOARCH == "386" || os.Getenv("GOARCH") == "386"
}

func isCI() bool {
	return os.Getenv("USER") == "veyron"
}

func isDarwin() bool {
	return runtime.GOOS == "darwin"
}

func isYosemite() bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	out, err := exec.Command("uname", "-a").Output()
	if err != nil {
		return true
	}
	return strings.Contains(string(out), "Version 14.")
}
