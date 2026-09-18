//go:build ggui_debug

package ggui

import (
	"runtime"
	"strconv"
	"strings"
)

// effectOrigin returns the file and line outside this package that created
// the effect being registered, so that a cycle can name the effects it is
// made of. Only this build records it; see ErrCycle.
func effectOrigin() string {
	var pcs [8]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if f.Function != "" && !isGgui(f.Function) {
			return f.File + ":" + strconv.Itoa(f.Line)
		}
		if !more {
			// Everything on the stack is ggui's own: report the innermost
			// frame rather than nothing, which still says where to look.
			return f.File + ":" + strconv.Itoa(f.Line)
		}
	}
}

// isGgui reports whether fn belongs to this package, as opposed to the ui
// and icons packages built on it, whose frames are worth naming.
func isGgui(fn string) bool {
	const pkg = "github.com/ironpark/ggui."
	i := strings.LastIndex(fn, pkg)
	return i >= 0 && !strings.Contains(fn[i+len(pkg):], "/")
}
