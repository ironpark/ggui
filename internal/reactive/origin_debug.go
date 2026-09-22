//go:build ggui_debug

package reactive

import (
	"runtime"
	"strconv"
	"strings"
)

// Debug reports whether this build records effect origins.
const Debug = true

// effectOrigin returns the file and line outside this package that created
// the Computation being registered, so that a cycle can name the effects it is
// made of. Only this build records it; see ErrCycle.
func effectOrigin() string {
	var pcs [32]uintptr
	n := runtime.Callers(2, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if f.Function != "" && (strings.HasSuffix(f.File, "_test.go") || !isGgui(f.Function)) {
			return f.File + ":" + strconv.Itoa(f.Line)
		}
		if !more {
			// Everything on the stack is ggui's own: report the last available
			// frame rather than nothing, which still says where to look.
			return f.File + ":" + strconv.Itoa(f.Line)
		}
	}
}

// isGgui reports whether fn is ggui's own plumbing -- the root package or
// one of its internals, including this one -- as opposed to the ui and
// icons packages built on it, whose frames are worth naming.
func isGgui(fn string) bool {
	const mod = "github.com/ironpark/ggui"
	i := strings.LastIndex(fn, mod)
	if i < 0 {
		return false
	}
	rest := fn[i+len(mod):]
	return strings.HasPrefix(rest, ".") || strings.HasPrefix(rest, "/internal/")
}
