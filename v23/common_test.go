package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

func createConfig(t *testing.T, ctx *tool.Context, config *util.Config) {
	configPath, err := util.ConfigPath(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Run().MkdirAll(filepath.Dir(configPath), os.FileMode(0755)); err != nil {
		t.Fatalf("%v", err)
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := ctx.Run().WriteFile(configPath, data, os.FileMode(0644)); err != nil {
		t.Fatalf("%v", err)
	}
}
