//go:build ggui_debug

package reactive

import (
	"runtime"
	"strconv"
	"strings"
)

// Debug reports whether this build records effect origins.
const Debug = true

// Origin returns where the code outside ggui that called in sits, in a
// ggui_debug build; "" otherwise.
func Origin() string { return effectOrigin() }

// effectOrigin returns the file and line outside this package that created
// the Computation being registered, so that a cycle can name the effects it is
// made of. Only this build records it; see ErrCycle.
func effectOrigin() string { return callerOutside(3, isGgui) }

// callerOutside returns the file and line of the nearest caller, skip
// frames up, whose function own does not claim; code in a test file is
// always the caller's own. With every frame claimed it names the last one,
// which still says where to look.
func callerOutside(skip int, own func(fn string) bool) string {
	var pcs [32]uintptr
	n := runtime.Callers(skip, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if (f.Function != "" && (strings.HasSuffix(f.File, "_test.go") || !own(f.Function))) || !more {
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

// reportSnapshot reports, once per place, a signal read while widgets are
// built; see Build. A read a function such as theme.Use makes for its
// caller is reported where that function was called.
func reportSnapshot() {
	origin := callerOutside(3, inModule)
	ReportOnce(origin, "ggui: %s reads a signal while building widgets, which takes a snapshot that never updates; "+
		"bind the value instead (TextOf, a BindX setter, View or If), or read it in ggui.Untrack if a snapshot is meant", origin)
}

// inModule reports whether fn is ggui's own, ui and theme included, as
// opposed to its examples.
func inModule(fn string) bool {
	return strings.Contains(fn, "github.com/ironpark/ggui") && !strings.Contains(fn, "github.com/ironpark/ggui/examples/")
}
