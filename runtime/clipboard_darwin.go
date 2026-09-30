//go:build darwin && !ios

package runtime

import (
	goruntime "runtime"
	"sync"

	"github.com/ebitengine/purego/cstrings"
	"github.com/ebitengine/purego/objc"

	"github.com/ironpark/ggui/internal/platform/cocoa"
)

// The macOS clipboard is the general NSPasteboard, which may be used from
// any thread but not from two at once: calls in parallel crash the process,
// so they take turns. Each call drains its own autorelease pool, since it
// runs on whichever goroutine asked and none of them has one. A pool
// belongs to the thread that made it, so the goroutine stays on its thread
// until the pool is drained; drained on another thread, it crashes the
// process too.

var (
	selGeneralPasteboard = objc.RegisterName("generalPasteboard")
	selStringForType     = objc.RegisterName("stringForType:")
	selSetStringForType  = objc.RegisterName("setString:forType:")
	selClearContents     = objc.RegisterName("clearContents")
	selNew               = objc.RegisterName("new")
	selDrain             = objc.RegisterName("drain")

	classNSPasteboard      = objc.ID(objc.GetClass("NSPasteboard"))
	classNSAutoreleasePool = objc.ID(objc.GetClass("NSAutoreleasePool"))
)

// pasteboardTypeString is NSPasteboardTypeString.
const pasteboardTypeString = "public.utf8-plain-text"

func nativeClipboard() Clipboard { return pasteboard{} }

// pasteboard is the system clipboard through NSPasteboard.
type pasteboard struct{}

// pasteboardMu makes the calls to NSPasteboard take turns.
var pasteboardMu sync.Mutex

// withPool runs fn alone, on one thread, inside an autorelease pool.
func withPool(fn func()) {
	pasteboardMu.Lock()
	defer pasteboardMu.Unlock()
	goruntime.LockOSThread()
	defer goruntime.UnlockOSThread()
	pool := classNSAutoreleasePool.Send(selNew)
	defer pool.Send(selDrain)
	fn()
}

func (pasteboard) Read() string {
	var out string
	withPool(func() {
		s := classNSPasteboard.Send(selGeneralPasteboard).Send(selStringForType, cocoa.String(pasteboardTypeString))
		if s != 0 {
			out = cstrings.NSStringToString(s)
		}
	})
	return out
}

func (pasteboard) Write(s string) {
	withPool(func() {
		pb := classNSPasteboard.Send(selGeneralPasteboard)
		pb.Send(selClearContents)
		pb.Send(selSetStringForType, cocoa.String(s), cocoa.String(pasteboardTypeString))
	})
}
