//go:build darwin && !ios

// Package cocoa is the little of AppKit that ggui's platform services
// share: strings and arrays for Objective-C, the app's window, and a block
// for the APIs that take a completion handler. Everything is reached
// through purego, so the project stays free of cgo.
package cocoa

import (
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	selSharedApplication    = objc.RegisterName("sharedApplication")
	selWindows              = objc.RegisterName("windows")
	selCount                = objc.RegisterName("count")
	selObjectAtIndex        = objc.RegisterName("objectAtIndex:")
	selContentView          = objc.RegisterName("contentView")
	selIsKindOfClass        = objc.RegisterName("isKindOfClass:")
	selStringWithUTF8String = objc.RegisterName("stringWithUTF8String:")
	selArrayWithObjects     = objc.RegisterName("arrayWithObjects:count:")

	classNSString = objc.ID(objc.GetClass("NSString"))
	classNSArray  = objc.ID(objc.GetClass("NSArray"))
	classNSApp    = objc.ID(objc.GetClass("NSApplication"))
)

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

// StringArray returns ss as an autoreleased NSArray of NSString.
func StringArray(ss []string) objc.ID {
	ids := make([]objc.ID, len(ss))
	for i, s := range ss {
		ids[i] = String(s)
	}
	arr := classNSArray.Send(selArrayWithObjects, unsafe.Pointer(&ids[0]), uint(len(ids)))
	runtime.KeepAlive(ids)
	return arr
}

// AppWindow finds Ebitengine's window, which is the one whose content view
// is GLFW's, or zero before it exists. NSApplication.mainWindow can be nil
// on a background launch, and AppKit owns panels and helper windows too,
// so nothing else will do. Like everything that touches a window, it must
// run on the main thread.
func AppWindow() objc.ID {
	contentClass := objc.GetClass("GLFWContentView")
	if contentClass == 0 {
		return 0
	}
	windows := App().Send(selWindows)
	n := objc.Send[uint](windows, selCount)
	for i := range n {
		window := windows.Send(selObjectAtIndex, i)
		content := window.Send(selContentView)
		if content != 0 && objc.Send[bool](content, selIsKindOfClass, contentClass) {
			return window
		}
	}
	return 0
}

// A completion handler is an Objective-C block, which purego cannot make,
// so one is laid out here by hand: the literal the compiler would emit for
// a global block with no captures, which AppKit copies by returning it and
// never frees. It holds no Go pointer but the callback, which is a C
// function for the life of the process.

// blockLiteral is struct Block_literal from the blocks ABI.
type blockLiteral struct {
	isa        uintptr
	flags      int32
	reserved   int32
	invoke     uintptr
	descriptor *blockDescriptor
}

// blockDescriptor is the part of struct Block_descriptor every block has.
type blockDescriptor struct {
	reserved uintptr
	size     uintptr
}

// blockIsGlobal marks a block that lives in static storage, so that copying
// it is a no-op and there is nothing to release.
const blockIsGlobal = 1 << 28

var (
	globalBlockISA  uintptr
	globalBlockDesc = &blockDescriptor{size: unsafe.Sizeof(blockLiteral{})}
)

func init() {
	libSystem, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	globalBlockISA, err = purego.Dlsym(libSystem, "_NSConcreteGlobalBlock")
	if err != nil {
		panic(err)
	}
}

// Block is a global block whose body is fn. It is made once and kept for
// the life of the process, since the callback it holds is.
type Block struct {
	lit blockLiteral
}

// NewBlock makes a block that calls fn. fn takes the block itself as its
// first argument, as every block body does, followed by the arguments the
// API passes to its handler; it must be a function purego.NewCallback
// accepts.
func NewBlock(fn any) *Block {
	b := &Block{lit: blockLiteral{
		isa:        globalBlockISA,
		flags:      blockIsGlobal,
		invoke:     purego.NewCallback(fn),
		descriptor: globalBlockDesc,
	}}
	return b
}

// Ptr is what to pass where an API takes a block.
func (b *Block) Ptr() unsafe.Pointer { return unsafe.Pointer(&b.lit) }
