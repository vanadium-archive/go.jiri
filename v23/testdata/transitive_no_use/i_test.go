package transitive_nouse

import (
	"testing"

	_ "v.io/x/ref/profiles"

	"v.io/x/devtools/v23/testdata/transitive/middle"
)

var cmd = "moduleInternalOnly"

func init() {
	middle.Init()
}

func TestWithoutModules(t *testing.T) {
}
