package lib

import (
	"fmt"
	"io"

	xm "v.io/x/ref/lib/modules"
	_ "v.io/x/ref/profiles"
)

var cmd = "moduleInternalOnly"

func Init() {
	xm.RegisterChild(cmd, "", moduleInternalOnly)
}

// Oh..
func moduleInternalOnly(stdin io.Reader, stdout, stderr io.Writer, env map[string]string, args ...string) error {
	fmt.Fprintln(stdout, cmd)
	return nil
}
