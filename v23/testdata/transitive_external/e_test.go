package transitive_external_test

import (
	"testing"

	_ "v.io/x/ref/profiles"
	"v.io/x/ref/test/v23tests"

	"v.io/x/devtools/v23/testdata/transitive_external"
)

var cmd = "moduleInternalOnly"

func TestModulesExternal(t *testing.T) {
	transitive_external.Module(t)
}

func V23TestOneA(i *v23tests.T) {}
