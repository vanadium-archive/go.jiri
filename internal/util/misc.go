package util

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// ThirdPartyBinPath returns the path to the given third-party tool
// taking into account the host and the target Go architecture.
func ThirdPartyBinPath(root, name string) string {
	bin := filepath.Join(root, "third_party", "go", "bin", name)
	goArch := os.Getenv("GOARCH")
	// TODO(jingjin): we assume now we run all our tests on amd64 machines.
	if goArch != "" && goArch != "amd64" {
		bin = filepath.Join(root, "third_party", "go", "bin", fmt.Sprintf("%s_%s", runtime.GOOS, goArch), name)
	}
	return bin
}
