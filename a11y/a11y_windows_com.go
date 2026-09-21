//go:build windows

package a11y

import (
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This file is the COM half of the Windows bridge: it builds the objects UI
// Automation talks to, without cgo and without handing the operating system
// a single Go pointer.
//
// A provider is a C++ object with several base classes, and that is what is
// built here by hand. The memory comes from GlobalAlloc rather than from Go,
// because UI Automation keeps the address for as long as it likes and Go's
// collector knows nothing about it; nothing inside it is a Go pointer, so
// there is nothing for the collector to lose track of either. The method
// tables are allocated the same way, once, and never freed -- they hold the
// results of syscall.NewCallback, which are never freed either.

// COM and UI Automation result codes, from winerror.h and
// uiautomationcoreapi.h. Zero is success; everything else is a failure UIA
// reports to its client as it sees fit.
const (
	sOK                    = 0
	eNoInterface           = 0x80004002
	ePointer               = 0x80004003
	eFail                  = 0x80004005
	uiaElementNotAvailable = 0x80040201
	uiaInvalidOperation    = 0x80131509
)

// VARTYPE values, from wtypes.h, for the variants a property is returned in.
const (
	vtEmpty = 0
	vtI4    = 3
	vtR8    = 5
	vtBSTR  = 8
	vtBool  = 11
)

// variantTrue is what a VT_BOOL holds for true: every bit set, not one.
const variantTrue = -1

// winGUID is a COM interface identifier, laid out as the operating system
// lays one out so that a QueryInterface argument can be compared to it
// directly.
type winGUID struct {
	a uint32
	b uint16
	c uint16
	d [8]byte
}

// The identifiers of every interface a provider answers to, from
// uiautomationcore.h, plus IUnknown's from unknwn.h.
var (
	iidUnknown                        = winGUID{0x00000000, 0x0000, 0x0000, [8]byte{0xc0, 0x00, 0, 0, 0, 0, 0, 0x46}}
	iidRawElementProviderSimple       = winGUID{0xd6dd68d1, 0x86fd, 0x4332, [8]byte{0x86, 0x66, 0x9a, 0xbe, 0xde, 0xa2, 0xd2, 0x4c}}
	iidRawElementProviderFragment     = winGUID{0xf7063da8, 0x8359, 0x439c, [8]byte{0x92, 0x97, 0xbb, 0xc5, 0x29, 0x9a, 0x7d, 0x87}}
	iidRawElementProviderFragmentRoot = winGUID{0x620ce2a5, 0xab8f, 0x40a9, [8]byte{0x86, 0xcb, 0xde, 0x3c, 0x75, 0x59, 0x9b, 0x58}}
	iidInvokeProvider                 = winGUID{0x54fcb24b, 0xe18e, 0x47a2, [8]byte{0xb4, 0xd3, 0xec, 0xcb, 0xe7, 0x75, 0x99, 0xa2}}
	iidValueProvider                  = winGUID{0xc7935180, 0x6fb3, 0x4201, [8]byte{0xb1, 0x74, 0x7d, 0xf7, 0x3a, 0xdb, 0xf6, 0x4a}}
	iidRangeValueProvider             = winGUID{0x36dc7aef, 0x33e6, 0x4691, [8]byte{0xaf, 0xe1, 0x2b, 0xe7, 0x27, 0x4b, 0x3d, 0x33}}
	iidToggleProvider                 = winGUID{0x56d00bd0, 0xc4f4, 0x433c, [8]byte{0xa8, 0x36, 0x1a, 0x52, 0xa5, 0x7e, 0x08, 0x92}}
	iidExpandCollapseProvider         = winGUID{0xd847d3a5, 0xcab0, 0x4a98, [8]byte{0x8c, 0x32, 0xec, 0xb4, 0x5c, 0x59, 0xad, 0x24}}
	iidSelectionItemProvider          = winGUID{0x2acad808, 0xb2d4, 0x452d, [8]byte{0xa4, 0x07, 0x91, 0xff, 0x1a, 0xd1, 0x67, 0xb2}}
)

// The interfaces a provider object exposes, one method table each. The
// order is the object's layout: interface k is addressed at the object's
// base plus k pointers, which is how a C++ compiler lays out multiple
// inheritance and what lets QueryInterface answer without allocating.
const (
	ifSimple = iota
	ifFragment
	ifFragmentRoot
	ifInvoke
	ifValue
	ifRangeValue
	ifToggle
	ifExpandCollapse
	ifSelectionItem
	ifCount
)

// winObj is the shape of the memory a provider object occupies. It is never
// allocated as a Go value: it is the template for reading and writing a
// block of GlobalAlloc'd memory, whose address the operating system holds.
type winObj struct {
	vtbl   [ifCount]uintptr
	refs   int32
	_      int32
	handle int64
}

// vtbls holds the address of each interface's method table. A method is
// called with a pointer to one of the object's table slots, and the table
// address found there is what says which interface was called, and
// therefore how far into the object that pointer points.
var vtbls [ifCount]uintptr

// objOf recovers the object a COM method was called on, together with the
// index of the interface it was called through. It fails only if the
// pointer is not one of ours, which should not happen and is answered with
// a failure rather than a crash.
func objOf(this uintptr) (*winObj, int, bool) {
	if this == 0 {
		return nil, 0, false
	}
	table := *(*uintptr)(unsafe.Pointer(this))
	for k, vt := range vtbls {
		if vt == table {
			return (*winObj)(unsafe.Pointer(this - uintptr(k)*unsafe.Sizeof(uintptr(0)))), k, true
		}
	}
	return nil, 0, false
}

// selfOf is what almost every provider method starts with: the object, the
// bridge, the frame being answered from and the node the element stands
// for. It answers false for an element whose node has left the tree, which
// is normal, since an assistive technology keeps the ones it was given.
func selfOf(this uintptr) (*winObj, *Bridge, SemNode, bool) {
	o, _, ok := objOf(this)
	if !ok {
		return nil, nil, SemNode{}, false
	}
	b := current.Load()
	if b == nil {
		return o, nil, SemNode{}, false
	}
	if o.handle == winRootHandle {
		return o, b, SemNode{}, false
	}
	n, ok := b.node(o.handle)
	return o, b, n, ok
}

// winRootHandle is the handle of the one object that stands for the window
// rather than for a node. No element cache handle can collide with it: a
// real one packs a non-negative slot into its low half.
const winRootHandle int64 = -1

// newWinObj allocates a provider object for handle and returns the address
// of its first interface, with one reference held by the caller.
func newWinObj(handle int64) uintptr {
	p := winAlloc(unsafe.Sizeof(winObj{}))
	if p == 0 {
		return 0
	}
	o := (*winObj)(unsafe.Pointer(p))
	o.vtbl = vtbls
	o.refs = 1
	o.handle = handle
	return p
}

// winAlloc takes a block of fixed, zeroed memory outside the Go heap.
func winAlloc(size uintptr) uintptr {
	// GMEM_FIXED | GMEM_ZEROINIT.
	p, _, _ := procGlobalAlloc.Call(0x0040, size)
	return p
}

// comAddRef and comRelease are IUnknown's, shared by every interface: the
// count belongs to the object, not to the interface a caller happens to
// hold. The object is freed when the last reference goes, which is the one
// moment its memory may be given back.
func comAddRef(this uintptr) uintptr {
	o, _, ok := objOf(this)
	if !ok {
		return 1
	}
	return uintptr(atomic.AddInt32(&o.refs, 1))
}

func comRelease(this uintptr) uintptr {
	o, _, ok := objOf(this)
	if !ok {
		return 0
	}
	n := atomic.AddInt32(&o.refs, -1)
	if n <= 0 {
		procGlobalFree.Call(uintptr(unsafe.Pointer(o)))
		return 0
	}
	return uintptr(n)
}

// comQueryInterface answers with the slot of whatever interface was asked
// for, which is an address inside the object the caller already holds.
// IUnknown is answered with the first slot every time, so that two queries
// for it on the same object compare equal, which is how COM says two
// pointers name one thing.
//
// The root object is the only one that is a fragment root, and only an
// element whose node offers the matching action is a pattern provider: an
// interface that would answer nothing is better refused, so that UIA does
// not tell the user about a control it cannot work.
func comQueryInterface(this, riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return ePointer
	}
	*(*uintptr)(unsafe.Pointer(ppv)) = 0
	o, _, ok := objOf(this)
	if !ok {
		return eNoInterface
	}
	id := (*winGUID)(unsafe.Pointer(riid))
	k := -1
	switch {
	case *id == iidUnknown || *id == iidRawElementProviderSimple:
		k = ifSimple
	case *id == iidRawElementProviderFragment:
		k = ifFragment
	case *id == iidRawElementProviderFragmentRoot:
		k = ifFragmentRoot
	case *id == iidInvokeProvider:
		k = ifInvoke
	case *id == iidValueProvider:
		k = ifValue
	case *id == iidRangeValueProvider:
		k = ifRangeValue
	case *id == iidToggleProvider:
		k = ifToggle
	case *id == iidExpandCollapseProvider:
		k = ifExpandCollapse
	case *id == iidSelectionItemProvider:
		k = ifSelectionItem
	}
	if k < 0 || !o.offers(k) {
		return eNoInterface
	}
	slot := uintptr(unsafe.Pointer(o)) + uintptr(k)*unsafe.Sizeof(uintptr(0))
	comAddRef(slot)
	*(*uintptr)(unsafe.Pointer(ppv)) = slot
	return sOK
}

// offers reports whether this object really implements interface k, which
// is what QueryInterface answers on and therefore what decides the patterns
// UIA believes the element supports.
func (o *winObj) offers(k int) bool {
	root := o.handle == winRootHandle
	switch k {
	case ifSimple, ifFragment:
		return true
	case ifFragmentRoot:
		return root
	}
	if root {
		return false
	}
	b := current.Load()
	if b == nil {
		return false
	}
	n, ok := b.node(o.handle)
	if !ok {
		return false
	}
	return winPattern(n.Node, k)
}

// winStr turns a Go string into a BSTR, which is the only kind of string
// that may be handed to UI Automation: it allocates the result out of the
// same allocator UIA frees it with. An empty string still gets a BSTR,
// since a property that is present but blank is not the same as absent.
func winStr(s string) uintptr {
	u, err := windows.UTF16PtrFromString(s)
	if err != nil {
		// A NUL inside the string: hand over the empty string rather
		// than a truncated one.
		u = new(uint16)
	}
	p, _, _ := procSysAllocString.Call(uintptr(unsafe.Pointer(u)))
	return p
}

// winVariant is a VARIANT as the operating system lays one out. The union
// is two pointers wide rather than one, because it holds a BRECORD, which
// is two; getting this wrong writes past the end of the caller's variant.
type winVariant struct {
	vt         uint16
	r1, r2, r3 uint16
	val        [2]uintptr
}

// The variant writers. Each one fills a variant the caller owns, and is
// what GetPropertyValue answers a supported property with.
func varEmpty(v *winVariant) uintptr { *v = winVariant{vt: vtEmpty}; return sOK }

func varBool(v *winVariant, b bool) uintptr {
	*v = winVariant{vt: vtBool}
	if b {
		*(*int16)(unsafe.Pointer(&v.val[0])) = variantTrue
	}
	return sOK
}

func varI4(v *winVariant, n int32) uintptr {
	*v = winVariant{vt: vtI4}
	*(*int32)(unsafe.Pointer(&v.val[0])) = n
	return sOK
}

func varR8(v *winVariant, f float64) uintptr {
	*v = winVariant{vt: vtR8}
	*(*float64)(unsafe.Pointer(&v.val[0])) = f
	return sOK
}

func varStr(v *winVariant, s string) uintptr {
	*v = winVariant{vt: vtBSTR}
	v.val[0] = winStr(s)
	return sOK
}

// winRuntimeID builds the SAFEARRAY of integers that identifies an element
// across frames. The tail is the element cache's handle, split into its
// slot and generation, which is exactly the pair that stays the same while
// a node stays in the tree and changes the moment a slot is reused.
func winRuntimeID(handle int64) uintptr {
	slot, gen := axUnpack(handle)
	ids := [3]int32{uiaAppendRuntimeID, slot, int32(gen)}
	// SafeArrayCreateVector(VT_I4, 0, 3).
	arr, _, _ := procSafeArrayCreateVector.Call(vtI4, 0, uintptr(len(ids)))
	if arr == 0 {
		return 0
	}
	for i := range ids {
		idx := int32(i)
		procSafeArrayPutElement.Call(arr, uintptr(unsafe.Pointer(&idx)), uintptr(unsafe.Pointer(&ids[i])))
	}
	return arr
}

// buildVtables lays out every method table once, at startup. It runs before
// any window exists, which is fine: nothing here touches one.
//
// Two methods take their arguments as doubles, and a Go callback cannot
// receive one -- the operating system passes it in a floating-point
// register and Go's callback entry saves only the integer registers. Those
// two go through a short assembly thunk that copies the registers across;
// see a11y_windows_thunk.go for what happens where there is no thunk.
func buildVtables() {
	qi := syscall.NewCallback(comQueryInterface)
	ar := syscall.NewCallback(comAddRef)
	rl := syscall.NewCallback(comRelease)
	unk := []uintptr{qi, ar, rl}
	table := func(methods ...uintptr) uintptr {
		all := append(append([]uintptr{}, unk...), methods...)
		p := winAlloc(uintptr(len(all)) * unsafe.Sizeof(uintptr(0)))
		copy(unsafe.Slice((*uintptr)(unsafe.Pointer(p)), len(all)), all)
		return p
	}
	vtbls[ifSimple] = table(
		syscall.NewCallback(winGetProviderOptions),
		syscall.NewCallback(winGetPatternProvider),
		syscall.NewCallback(winGetPropertyValue),
		syscall.NewCallback(winGetHostProvider),
	)
	vtbls[ifFragment] = table(
		syscall.NewCallback(winNavigate),
		syscall.NewCallback(winGetRuntimeID),
		syscall.NewCallback(winBoundingRectangle),
		syscall.NewCallback(winGetEmbeddedFragmentRoots),
		syscall.NewCallback(winSetFocus),
		syscall.NewCallback(winGetFragmentRoot),
	)
	vtbls[ifFragmentRoot] = table(
		winFromPointEntry(),
		syscall.NewCallback(winGetFocus),
	)
	vtbls[ifInvoke] = table(
		syscall.NewCallback(winInvoke),
	)
	vtbls[ifValue] = table(
		syscall.NewCallback(winValueSetValue),
		syscall.NewCallback(winValueGetValue),
		syscall.NewCallback(winIsReadOnly),
	)
	vtbls[ifRangeValue] = table(
		winRangeSetValueEntry(),
		syscall.NewCallback(winRangeGetValue),
		syscall.NewCallback(winIsReadOnly),
		syscall.NewCallback(winRangeMaximum),
		syscall.NewCallback(winRangeMinimum),
		syscall.NewCallback(winRangeLargeChange),
		syscall.NewCallback(winRangeSmallChange),
	)
	vtbls[ifToggle] = table(
		syscall.NewCallback(winInvoke),
		syscall.NewCallback(winToggleState),
	)
	vtbls[ifExpandCollapse] = table(
		syscall.NewCallback(winExpand),
		syscall.NewCallback(winCollapse),
		syscall.NewCallback(winExpandCollapseState),
	)
	vtbls[ifSelectionItem] = table(
		syscall.NewCallback(winSelect),
		syscall.NewCallback(winAddToSelection),
		syscall.NewCallback(winRemoveFromSelection),
		syscall.NewCallback(winIsSelected),
		syscall.NewCallback(winSelectionContainer),
	)
}

// The libraries the bridge calls into. NewLazySystemDLL resolves them out
// of the system directory rather than out of whatever is beside the
// executable, and nothing is loaded until the first call.
var (
	dllUser32   = windows.NewLazySystemDLL("user32.dll")
	dllKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	dllOleAut32 = windows.NewLazySystemDLL("oleaut32.dll")
	dllComctl32 = windows.NewLazySystemDLL("comctl32.dll")
	dllUIA      = windows.NewLazySystemDLL("uiautomationcore.dll")

	procGlobalAlloc        = dllKernel32.NewProc("GlobalAlloc")
	procGlobalFree         = dllKernel32.NewProc("GlobalFree")
	procGetCurrentThreadID = dllKernel32.NewProc("GetCurrentThreadId")

	procEnumThreadWindows = dllUser32.NewProc("EnumThreadWindows")
	procGetClassNameW     = dllUser32.NewProc("GetClassNameW")
	procIsWindowVisible   = dllUser32.NewProc("IsWindowVisible")
	procClientToScreen    = dllUser32.NewProc("ClientToScreen")
	procGetDpiForWindow   = dllUser32.NewProc("GetDpiForWindow")
	procGetClientRect     = dllUser32.NewProc("GetClientRect")

	procSysAllocString        = dllOleAut32.NewProc("SysAllocString")
	procSysFreeString         = dllOleAut32.NewProc("SysFreeString")
	procSafeArrayCreateVector = dllOleAut32.NewProc("SafeArrayCreateVector")
	procSafeArrayPutElement   = dllOleAut32.NewProc("SafeArrayPutElement")

	procSetWindowSubclass    = dllComctl32.NewProc("SetWindowSubclass")
	procRemoveWindowSubclass = dllComctl32.NewProc("RemoveWindowSubclass")
	procDefSubclassProc      = dllComctl32.NewProc("DefSubclassProc")

	procUiaReturnRawElementProvider = dllUIA.NewProc("UiaReturnRawElementProvider")
	procUiaHostProviderFromHwnd     = dllUIA.NewProc("UiaHostProviderFromHwnd")
	procUiaDisconnectProvider       = dllUIA.NewProc("UiaDisconnectProvider")
	procUiaClientsAreListening      = dllUIA.NewProc("UiaClientsAreListening")
	procUiaRaiseAutomationEvent     = dllUIA.NewProc("UiaRaiseAutomationEvent")
	procUiaRaisePropertyChanged     = dllUIA.NewProc("UiaRaiseAutomationPropertyChangedEvent")
	procUiaRaiseStructureChanged    = dllUIA.NewProc("UiaRaiseStructureChangedEvent")
	procUiaRaiseNotificationEvent   = dllUIA.NewProc("UiaRaiseNotificationEvent")
)
