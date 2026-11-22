//go:build darwin && !ios

// Package cocoa is the little of Foundation and AppKit that ggui's platform
// services share: common selectors, strings, arrays, ABI-matched geometry
// types, autorelease pools, and the shared application. Everything is reached
// through purego, so the project stays free of cgo.
package cocoa

import (
	"iter"
	"runtime"
	"structs"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Common Foundation and AppKit selectors shared across platform packages.
var (
	SelAlloc           = objc.RegisterName("alloc")
	SelInit            = objc.RegisterName("init")
	SelNew             = objc.RegisterName("new")
	SelRetain          = objc.RegisterName("retain")
	SelRelease         = objc.RegisterName("release")
	SelAutorelease     = objc.RegisterName("autorelease")
	SelDrain           = objc.RegisterName("drain")
	SelUTF8String      = objc.RegisterName("UTF8String")
	SelCount           = objc.RegisterName("count")
	SelObjectAtIndex   = objc.RegisterName("objectAtIndex:")
	SelIsKindOfClass   = objc.RegisterName("isKindOfClass:")
	SelBounds          = objc.RegisterName("bounds")
	SelWindow          = objc.RegisterName("window")
	SelSharedWorkspace = objc.RegisterName("sharedWorkspace")
	SelSetTitle        = objc.RegisterName("setTitle:")
)

var (
	selSharedApplication    = objc.RegisterName("sharedApplication")
	selStringWithUTF8String = objc.RegisterName("stringWithUTF8String:")
	selArrayWithObjects     = objc.RegisterName("arrayWithObjects:count:")
	selArray                = objc.RegisterName("array")

	classNSAutoreleasePool objc.ID
	classNSString          objc.ID
	classNSArray           objc.ID
	classNSApp             objc.ID
)

// Point matches NSPoint / CGPoint in the C ABI.
type Point struct {
	_    structs.HostLayout
	X, Y float64
}

// Size matches NSSize / CGSize in the C ABI.
type Size struct {
	_    structs.HostLayout
	W, H float64
}

// Rect matches NSRect / CGRect in the C ABI.
type Rect struct {
	_      structs.HostLayout
	Origin Point
	Size   Size
}

func init() {
	if _, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		panic(err)
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		panic(err)
	}
	classNSAutoreleasePool = objc.ID(objc.GetClass("NSAutoreleasePool"))
	classNSString = objc.ID(objc.GetClass("NSString"))
	classNSArray = objc.ID(objc.GetClass("NSArray"))
	classNSApp = objc.ID(objc.GetClass("NSApplication"))
}

// WithAutoreleasePool runs fn on a locked OS thread inside a fresh
// NSAutoreleasePool that is drained before returning.
func WithAutoreleasePool(fn func()) {
	if fn == nil {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := classNSAutoreleasePool.Send(SelAlloc).Send(SelInit)
	defer pool.Send(SelDrain)
	fn()
}

// App is the shared NSApplication.
func App() objc.ID { return classNSApp.Send(selSharedApplication) }

// String returns s as an autoreleased NSString. AppKit copies the bytes
// before the call returns, so the Go slice need only outlive it.
func String(s string) objc.ID {
	b := append([]byte(s), 0)
	id := classNSString.Send(selStringWithUTF8String, unsafe.Pointer(&b[0]))
	runtime.KeepAlive(b)
	return id
}

// GoString copies an NSString's UTF-8 text into a Go string, returning ""
// if id is nil or has no UTF-8 representation.
func GoString(id objc.ID) string {
	if id == 0 {
		return ""
	}
	p := objc.Send[*byte](id, SelUTF8String)
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// Array returns ids as an autoreleased NSArray; an empty one for no ids.
func Array(ids []objc.ID) objc.ID {
	if len(ids) == 0 {
		return classNSArray.Send(selArray)
	}
	arr := classNSArray.Send(selArrayWithObjects, unsafe.Pointer(&ids[0]), uint(len(ids)))
	runtime.KeepAlive(ids)
	return arr
}

// StringArray returns ss as an autoreleased NSArray of NSString.
func StringArray(ss []string) objc.ID {
	ids := make([]objc.ID, len(ss))
	for i, s := range ss {
		ids[i] = String(s)
	}
	return Array(ids)
}

// Objects walks the elements of an NSArray, in order; a loop that breaks
// asks for no more than it looked at.
func Objects(arr objc.ID) iter.Seq[objc.ID] {
	return func(yield func(objc.ID) bool) {
		n := objc.Send[uint](arr, SelCount)
		for i := range n {
			if !yield(arr.Send(SelObjectAtIndex, i)) {
				return
			}
		}
	}
}
