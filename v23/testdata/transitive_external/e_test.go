package transitive_external_test

import (
	"testing"

	"v.io/x/ref/lib/testutil/v23tests"
	_ "v.io/x/ref/profiles"

	"v.io/x/devtools/v23/testdata/transitive_external"
)

var cmd = "moduleInternalOnly"

func TestModulesExternal(t *testing.T) {
	transitive_external.Module(t)
}

func V23TestOneA(i *v23tests.T) {}
