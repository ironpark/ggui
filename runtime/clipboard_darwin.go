//go:build darwin && !ios

package runtime

import (
	goruntime "runtime"
	"sync"
	"unsafe"

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
	selDataForType       = objc.RegisterName("dataForType:")
	selBytes             = objc.RegisterName("bytes")
	selLength            = objc.RegisterName("length")
	selImageRepWithData  = objc.RegisterName("imageRepWithData:")
	selRepresentation    = objc.RegisterName("representationUsingType:properties:")
	selDictionary        = objc.RegisterName("dictionary")
	selNew               = objc.RegisterName("new")
	selDrain             = objc.RegisterName("drain")

	classNSPasteboard      = objc.ID(objc.GetClass("NSPasteboard"))
	classNSAutoreleasePool = objc.ID(objc.GetClass("NSAutoreleasePool"))
	classNSBitmapImageRep  = objc.ID(objc.GetClass("NSBitmapImageRep"))
	classNSDictionary      = objc.ID(objc.GetClass("NSDictionary"))
)

// The pasteboard types read: NSPasteboardTypeString, NSPasteboardTypePNG
// and NSPasteboardTypeTIFF.
const (
	pasteboardTypeString = "public.utf8-plain-text"
	pasteboardTypePNG    = "public.png"
	pasteboardTypeTIFF   = "public.tiff"
)

// bitmapFileTypePNG is NSBitmapImageFileTypePNG.
const bitmapFileTypePNG = 4

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

// ReadImage implements ImageClipboard: the PNG on the pasteboard, or its
// TIFF, which is what most apps copy an image as, turned into PNG.
func (pasteboard) ReadImage() ([]byte, bool) {
	var out []byte
	withPool(func() {
		pb := classNSPasteboard.Send(selGeneralPasteboard)
		data := pb.Send(selDataForType, cocoa.String(pasteboardTypePNG))
		if data == 0 {
			tiff := pb.Send(selDataForType, cocoa.String(pasteboardTypeTIFF))
			if tiff == 0 {
				return
			}
			rep := classNSBitmapImageRep.Send(selImageRepWithData, tiff)
			if rep == 0 {
				return
			}
			data = rep.Send(selRepresentation, uint(bitmapFileTypePNG), classNSDictionary.Send(selDictionary))
		}
		out = nsDataBytes(data)
	})
	return out, len(out) > 0
}

// nsDataBytes copies the bytes NSData data holds.
func nsDataBytes(data objc.ID) []byte {
	if data == 0 {
		return nil
	}
	n := objc.Send[uint](data, selLength)
	p := objc.Send[unsafe.Pointer](data, selBytes)
	if n == 0 || p == nil {
		return nil
	}
	return append([]byte(nil), unsafe.Slice((*byte)(p), n)...)
}
