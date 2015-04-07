// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

// A simple utility to display tests that are to be excluded on the
// host that this command is run on. It also displays the go
// environment variables and USER values in effect.
//
// You can run it as you would any other go main program that's
// contained in a single file within a related package:
//
// 1) if you obtained the code using 'go get':
// "go run $(go list -f {{.Dir}} v.io/x/devtools/internal/testutil)/excluded_tests.go"
// 2) if you are using the v23 tool and "V23_ROOT" setup.
// "v23 go run $(v23 go list -f {{.Dir}} v.io/x/devtools/internal/testutil)/excluded_tests.go"
package main

import (
	"fmt"
	"os"
	"runtime"

	"v.io/x/devtools/internal/testutil"
)

func main() {
	fmt.Printf("GOOS: %s\n", runtime.GOOS)
	fmt.Printf("GOARCH: %s\n", runtime.GOARCH)
	fmt.Printf("GOROOT: %s\n", runtime.GOROOT())
	fmt.Printf("USER: %q\n", os.Getenv("USER"))
	fmt.Println("Excluded tests:")
	excluded, err := testutil.ExcludedTests()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get exclusions: %s", err)
		os.Exit(1)
	}
	for _, t := range excluded {
		fmt.Printf("%#v\n", t)
	}
}
