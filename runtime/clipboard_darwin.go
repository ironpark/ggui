//go:build darwin && !ios

package runtime

import (
	"github.com/ebitengine/purego/cstrings"
	"github.com/ebitengine/purego/objc"

	"github.com/ironpark/ggui/internal/platform/cocoa"
)

// The macOS clipboard is the general NSPasteboard, which may be used from
// any thread. Each call drains its own autorelease pool, since it runs on
// whichever goroutine asked and none of them has one.

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

func (pasteboard) Read() string {
	pool := classNSAutoreleasePool.Send(selNew)
	defer pool.Send(selDrain)
	s := classNSPasteboard.Send(selGeneralPasteboard).Send(selStringForType, cocoa.String(pasteboardTypeString))
	if s == 0 {
		return ""
	}
	return cstrings.NSStringToString(s)
}

func (pasteboard) Write(s string) {
	pool := classNSAutoreleasePool.Send(selNew)
	defer pool.Send(selDrain)
	pb := classNSPasteboard.Send(selGeneralPasteboard)
	pb.Send(selClearContents)
	pb.Send(selSetStringForType, cocoa.String(s), cocoa.String(pasteboardTypeString))
}
