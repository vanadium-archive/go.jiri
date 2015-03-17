package util

import "fmt"

type Color int

const (
	Black Color = iota + 30
	Red
	Green
	Yellow
	Blue
	Magenta
	Cyan
	White

	start = "\033["
	reset = "\033[0m"
)

func ColorString(str string, color Color) string {
	return fmt.Sprintf("%s0;%dm%s%s", start, color, str, reset)
}
