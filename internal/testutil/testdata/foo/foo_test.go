package foo_test

import (
	"testing"

	"v.io/x/devtools/internal/testutil/testdata/foo"
)

func Test1(t *testing.T) {
	if foo.Foo() != "hello" {
		t.Fatalf("that's rude")
	}
}
