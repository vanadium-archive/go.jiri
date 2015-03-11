package internal_transitive_external_test

import (
	"testing"

	_ "v.io/x/ref/profiles"

	"v.io/x/devtools/v23/testdata/internal_transitive_external"
)

var cmd = "moduleInternalOnly"

func TestModulesExternal(t *testing.T) {
	internal_transitive_external.Module(t)
}
