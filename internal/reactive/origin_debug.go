//go:build ggui_debug

package reactive

import (
	"log"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// Debug reports whether this build records effect origins.
const Debug = true

// Origin returns where the code outside ggui that called in sits, in a
// ggui_debug build; "" otherwise.
func Origin() string { return effectOrigin() }

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

// snapshots holds the places already reported by reportSnapshot.
var snapshots sync.Map

// reportSnapshot reports, once per place, a signal read while widgets are
// built; see Build.
func reportSnapshot() {
	origin := userOrigin()
	if _, seen := snapshots.LoadOrStore(origin, true); !seen {
		log.Printf("ggui: %s reads a signal while building widgets, which takes a snapshot that never updates; "+
			"bind the value instead (TextOf, a BindX setter, View or If), or read it in ggui.Untrack if a snapshot is meant", origin)
	}
}

// userOrigin returns the file and line of the nearest caller outside the
// ggui module, or in its tests and examples: a read made by a function such
// as theme.Use is reported where that function was called.
func userOrigin() string {
	var pcs [32]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if !strings.Contains(f.Function, "github.com/ironpark/ggui") ||
			strings.HasSuffix(f.File, "_test.go") || strings.Contains(f.Function, "github.com/ironpark/ggui/examples/") {
			return f.File + ":" + strconv.Itoa(f.Line)
		}
		if !more {
			return f.File + ":" + strconv.Itoa(f.Line)
		}
	}
}
