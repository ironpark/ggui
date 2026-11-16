//go:build !((unix && !ios && !android) || windows)

package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	fmt.Fprintf(os.Stderr, "ggviewer: not supported on %s/%s: an app's window is shown by another process only on unix systems and Windows\n", runtime.GOOS, runtime.GOARCH)
	os.Exit(1)
}
