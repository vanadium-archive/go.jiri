// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package goutil provides Go wrappers around the Go command-line
// tool.
package goutil

import (
	"bytes"
	"fmt"
	"strings"

	"v.io/x/devtools/internal/tool"
)

// List inputs a list of Go package expressions and returns a list of
// Go packages that can be found in the GOPATH and match any of the
// expressions. The implementation invokes 'go list' internally.
func List(ctx *tool.Context, pkgs []string) ([]string, error) {
	args := []string{"go", "list"}
	args = append(args, pkgs...)
	var out bytes.Buffer
	opts := ctx.Run().Opts()
	opts.Stdout = &out
	opts.Stderr = &out
	if err := ctx.Run().CommandWithOpts(opts, "v23", args...); err != nil {
		fmt.Fprintln(ctx.Stdout(), out.String())
		return nil, err
	}
	cleanOut := strings.TrimSpace(out.String())
	if cleanOut == "" {
		return nil, nil
	}
	return strings.Split(cleanOut, "\n"), nil
}
