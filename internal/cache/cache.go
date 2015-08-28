// Copyright 2015 The Vanadium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cache

import (
	"os"
	"path/filepath"
	"strings"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
)

// StoreGoogleStorageFile reads the given file from the given Google Storage
// location and stores it in the given cache location. It returns the cached
// file path.
func StoreGoogleStorageFile(ctx *tool.Context, cacheRoot, bucketRoot, filename string) (_ string, e error) {
	cachedFile := filepath.Join(cacheRoot, filename)
	if _, err := ctx.Run().Stat(cachedFile); err != nil {
		if !os.IsNotExist(err) {
			return "", err
		}
		// To avoid interference between concurrent requests, download data to a
		// tmp dir, and move it to the final location.
		tmpDir, err := ctx.Run().TempDir(cacheRoot, "")
		if err != nil {
			return "", err
		}
		defer collect.Error(func() error { return ctx.Run().RemoveAll(tmpDir) }, &e)
		if err := ctx.Run().Command("gsutil", "-m", "-q", "cp", "-r", bucketRoot+"/"+filename, tmpDir); err != nil {
			return "", err
		}
		if err := ctx.Run().Rename(filepath.Join(tmpDir, filename), cachedFile); err != nil {
			// If the target directory already exists, it must have been created by
			// a concurrent request.
			if !strings.Contains(err.Error(), "directory not empty") {
				return "", err
			}
		}
	}
	return cachedFile, nil
}
