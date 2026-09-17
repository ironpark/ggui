//go:build darwin && !ios

package ggui

import (
	"reflect"
	"runtime"
	"structs"
	"sync/atomic"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/hajimehoshi/ebiten/v2"
)

// The macOS half of the accessibility bridge, in Objective-C reached
// through purego: the project builds without cgo and stays that way.
//
// The shape is the one internal/textinput already uses for the IME. An
// invisible NSView is added over the window's content view, and everything
// ggui paints is reported as that view's accessibility children. Nothing is
// swizzled and GLFW's own view is left alone; the container simply refuses
// the mouse, so that adding it changes nothing but what the accessibility
// API can see.
//
// Every element is an NSAccessibilityElement subclass rather than a bare
// NSObject implementing the informal protocol. The subclass is what AppKit
// expects of an element that is not a view, it already conforms to
// NSAccessibility so purego does not have to add the protocol by hand, and
// its default answers are sensible for the handful of attributes below that
// are not overridden.
//
// An element carries nothing but an integer handle in an instance variable.
// It holds no pointer to Go memory, so nothing has to be pinned, and a
// handle that outlives its node resolves to nothing rather than to whatever
// took its place. Every answer is read out of the last published SemTree,
// on the thread that asked, without waiting for a frame.

// theAX is the bridge the Objective-C methods answer from. A class is
// registered once per process and its methods are plain functions with
// nowhere to keep a receiver, so the one App that started a bridge puts
// itself here. A second App would replace it, which is the same limit the
// IME already has.
var theAX atomic.Pointer[axBridge]

type nsPoint struct {
	_ structs.HostLayout
	x float64
	y float64
}

type nsSize struct {
	_      structs.HostLayout
	width  float64
	height float64
}

type nsRect struct {
	_      structs.HostLayout
	origin nsPoint
	size   nsSize
}

var (
	axSelAlloc                = objc.RegisterName("alloc")
	axSelInit                 = objc.RegisterName("init")
	axSelRelease              = objc.RegisterName("release")
	axSelSharedApplication    = objc.RegisterName("sharedApplication")
	axSelMainWindow           = objc.RegisterName("mainWindow")
	axSelContentView          = objc.RegisterName("contentView")
	axSelAddSubview           = objc.RegisterName("addSubview:")
	axSelBounds               = objc.RegisterName("bounds")
	axSelSetFrame             = objc.RegisterName("setFrame:")
	axSelSetAutoresizingMask  = objc.RegisterName("setAutoresizingMask:")
	axSelWindow               = objc.RegisterName("window")
	axSelConvertRectToView    = objc.RegisterName("convertRect:toView:")
	axSelConvertRectFromView  = objc.RegisterName("convertRect:fromView:")
	axSelConvertRectToScreen  = objc.RegisterName("convertRectToScreen:")
	axSelConvertRectFromScr   = objc.RegisterName("convertRectFromScreen:")
	axSelStringWithUTF8String = objc.RegisterName("stringWithUTF8String:")
	axSelNumberWithDouble     = objc.RegisterName("numberWithDouble:")
	axSelArray                = objc.RegisterName("array")
	axSelArrayWithObjects     = objc.RegisterName("arrayWithObjects:count:")
	axSelSharedWorkspace      = objc.RegisterName("sharedWorkspace")
	axSelVoiceOverEnabled     = objc.RegisterName("isVoiceOverEnabled")

	axClassNSString = objc.GetClass("NSString")
	axClassNSNumber = objc.GetClass("NSNumber")
	axClassNSArray  = objc.GetClass("NSArray")
	axClassNSView   = objc.GetClass("NSView")

	axIDNSApplication = objc.ID(objc.GetClass("NSApplication"))
	axIDNSWorkspace   = objc.ID(objc.GetClass("NSWorkspace"))
)

// axRoleDescription is AppKit's own NSAccessibilityRoleDescription, which
// turns a role and a subrole into the localized noun VoiceOver says after
// the label: "button", "check box". Working it out here would mean shipping
// a translation of AppKit.
var axRoleDescription func(role, subrole uintptr) uintptr

// nsString returns s as an autoreleased NSString. The bytes are copied by
// AppKit before the call returns, so the Go slice need only outlive it.
func nsString(s string) objc.ID {
	b := append([]byte(s), 0)
	id := objc.ID(axClassNSString).Send(axSelStringWithUTF8String, unsafe.Pointer(&b[0]))
	runtime.KeepAlive(b)
	return id
}

// nsNumber returns v as an autoreleased NSNumber.
func nsNumber(v float64) objc.ID {
	return objc.ID(axClassNSNumber).Send(axSelNumberWithDouble, v)
}

// nsArray returns ids as an autoreleased NSArray, which is what every
// accessibility attribute returning a list must be.
func nsArray(ids []objc.ID) objc.ID {
	if len(ids) == 0 {
		return objc.ID(axClassNSArray).Send(axSelArray)
	}
	a := objc.ID(axClassNSArray).Send(axSelArrayWithObjects, unsafe.Pointer(&ids[0]), len(ids))
	runtime.KeepAlive(ids)
	return a
}

// darwinAX is the platform half of the bridge: the container view, and the
// three things the portable half asks of it.
type darwinAX struct {
	app       *App
	container objc.ID
}

// axAttach makes b the bridge the Objective-C methods answer from.
func axAttach(b *axBridge) { theAX.Store(b) }

// newAXPlatform returns the macOS bridge. Nothing is created here: the
// window does not exist until Run has started, so the container is attached
// from the first poll that finds the app running.
func newAXPlatform(a *App) axPlatform { return &darwinAX{app: a} }

// active reports whether VoiceOver is running, and takes the chance to
// attach the container view, which is work only the main thread may do.
// It is asked once a second, so the hop is nothing; asking every frame
// would put a main-thread round trip in the middle of every paint.
func (d *darwinAX) active() bool {
	if !appRunning.Load() {
		return false
	}
	var on bool
	ebiten.RunOnMainThread(func() {
		d.attach()
		on = axIDNSWorkspace.Send(axSelSharedWorkspace).Send(axSelVoiceOverEnabled) != 0
	})
	return on
}

// attach puts the container view over the window's content view, once. It
// runs on the main thread.
//
// The view is added last, so it sits above the IME's own subview, and it
// must therefore refuse the mouse: hitTest: returns nil, so a click lands
// wherever it would have without it. It resizes with the content view, so
// nothing has to touch its frame again.
func (d *darwinAX) attach() {
	if d.container != 0 {
		return
	}
	window := axIDNSApplication.Send(axSelSharedApplication).Send(axSelMainWindow)
	if window == 0 {
		return
	}
	content := window.Send(axSelContentView)
	if content == 0 {
		return
	}
	view := objc.ID(axClassContainer).Send(axSelAlloc).Send(axSelInit)
	view.Send(axSelSetFrame, objc.Send[nsRect](content, axSelBounds))
	// NSViewWidthSizable | NSViewHeightSizable.
	view.Send(axSelSetAutoresizingMask, uint(2|16))
	content.Send(axSelAddSubview, view)
	d.container = view
}

// element makes the accessibility object for one node, carrying its handle.
func (d *darwinAX) element(handle int64) uintptr {
	e := objc.ID(axClassElement).Send(axSelAlloc).Send(axSelInit)
	e.SetIvar(axHandleIvar, objc.ID(handle))
	return uintptr(e)
}

// release drops the bridge's reference to retired elements. It is sent from
// the frame goroutine rather than hopped onto the main thread: release is
// atomic, and an element owns nothing but an integer, so there is nothing
// in its teardown that wants a particular thread. Hopping would put a
// main-thread round trip into any frame that removed a widget.
func (d *darwinAX) release(elems []uintptr) {
	for _, e := range elems {
		objc.ID(e).Send(axSelRelease)
	}
}

// axHandleIvar is where an element keeps the handle naming its node.
var axHandleIvar objc.Ivar

var (
	axClassElement   objc.Class
	axClassContainer objc.Class
)

func init() {
	appkit, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		panic("ggui: " + err.Error())
	}
	purego.RegisterLibFunc(&axRoleDescription, appkit, "NSAccessibilityRoleDescription")

	axClassElement, err = objc.RegisterClass(
		"GgUIAccessibilityElement",
		objc.GetClass("NSAccessibilityElement"),
		nil,
		[]objc.FieldDef{{
			Name:      "handle",
			Type:      reflect.TypeFor[int64](),
			Attribute: objc.ReadOnly,
		}},
		[]objc.MethodDef{
			{Cmd: objc.RegisterName("isAccessibilityElement"), Fn: axIsElement},
			{Cmd: objc.RegisterName("accessibilityRole"), Fn: axElementRole},
			{Cmd: objc.RegisterName("accessibilitySubrole"), Fn: axElementSubrole},
			{Cmd: objc.RegisterName("accessibilityRoleDescription"), Fn: axElementRoleDescription},
			{Cmd: objc.RegisterName("accessibilityLabel"), Fn: axElementLabel},
			{Cmd: objc.RegisterName("accessibilityTitle"), Fn: axElementTitle},
			{Cmd: objc.RegisterName("accessibilityHelp"), Fn: axElementHelp},
			{Cmd: objc.RegisterName("accessibilityValue"), Fn: axElementValue},
			{Cmd: objc.RegisterName("accessibilityMinValue"), Fn: axElementMinValue},
			{Cmd: objc.RegisterName("accessibilityMaxValue"), Fn: axElementMaxValue},
			{Cmd: objc.RegisterName("accessibilityIdentifier"), Fn: axElementIdentifier},
			{Cmd: objc.RegisterName("accessibilityFrame"), Fn: axElementFrame},
			{Cmd: objc.RegisterName("accessibilityParent"), Fn: axElementParent},
			{Cmd: objc.RegisterName("accessibilityChildren"), Fn: axElementChildren},
			{Cmd: objc.RegisterName("isAccessibilityEnabled"), Fn: axElementEnabled},
			{Cmd: objc.RegisterName("isAccessibilityFocused"), Fn: axElementFocused},
			{Cmd: objc.RegisterName("isAccessibilitySelected"), Fn: axElementSelected},
			{Cmd: objc.RegisterName("isAccessibilityExpanded"), Fn: axElementExpanded},
			{Cmd: objc.RegisterName("accessibilityHitTest:"), Fn: axElementHitTest},
		},
	)
	if err != nil {
		panic("ggui: " + err.Error())
	}
	axHandleIvar = axClassElement.InstanceVariable("handle")

	axClassContainer, err = objc.RegisterClass(
		"GgUIAccessibilityContainer",
		axClassNSView,
		nil,
		nil,
		[]objc.MethodDef{
			{Cmd: objc.RegisterName("isAccessibilityElement"), Fn: axContainerIsElement},
			{Cmd: objc.RegisterName("accessibilityRole"), Fn: axContainerRole},
			{Cmd: objc.RegisterName("accessibilityChildren"), Fn: axContainerChildren},
			{Cmd: objc.RegisterName("accessibilityHitTest:"), Fn: axContainerHitTest},
			{Cmd: objc.RegisterName("hitTest:"), Fn: axContainerMouseHitTest},
		},
	)
	if err != nil {
		panic("ggui: " + err.Error())
	}
}

// axSelf returns the bridge, the frame being answered from and the node the
// element stands for. Every method starts with it, and every one of them
// answers nothing when it fails: an element outliving its node is normal,
// since an assistive technology keeps the ones it was given.
func axSelf(self objc.ID) (*axBridge, *axFrame, SemNode, bool) {
	b := theAX.Load()
	if b == nil {
		return nil, nil, SemNode{}, false
	}
	f := b.frame()
	if f == nil {
		return nil, nil, SemNode{}, false
	}
	n, ok := b.node(int64(self.GetIvar(axHandleIvar)))
	return b, f, n, ok
}

func axIsElement(objc.ID, objc.SEL) bool { return true }

func axElementRole(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if !ok {
		return 0
	}
	role, _ := axRole(n.Role)
	return nsString(role)
}

func axElementSubrole(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if !ok {
		return 0
	}
	_, sub := axRole(n.Role)
	if sub == "" {
		return 0
	}
	return nsString(sub)
}

func axElementRoleDescription(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if !ok {
		return 0
	}
	role, sub := axRole(n.Role)
	var subID objc.ID
	if sub != "" {
		subID = nsString(sub)
	}
	return objc.ID(axRoleDescription(uintptr(nsString(role)), uintptr(subID)))
}

func axElementLabel(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if !ok || n.Name == "" {
		return 0
	}
	return nsString(n.Name)
}

// axElementTitle reports no title, always. A label and a title both set are
// read out one after the other, and ggui has one name per node; the label
// is the one that works for an element with no drawn text of its own, which
// most of these are.
func axElementTitle(objc.ID, objc.SEL) objc.ID { return 0 }

func axElementHelp(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if !ok || n.Description == "" {
		return 0
	}
	return nsString(n.Description)
}

func axElementValue(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if !ok {
		return 0
	}
	if v, num := axNumber(n.Node); num {
		return nsNumber(v)
	}
	if n.Value == "" {
		return 0
	}
	return nsString(n.Value)
}

func axElementMinValue(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if lo, _, has := axRange(n.Node); ok && has {
		return nsNumber(lo)
	}
	return 0
}

func axElementMaxValue(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if _, hi, has := axRange(n.Node); ok && has {
		return nsNumber(hi)
	}
	return 0
}

// axElementIdentifier reports the identity a widget was given through Key,
// when it is a string. It is what an automated test drives the app by, and
// what Accessibility Inspector shows; a node keyed by something else has
// none rather than a rendering of it.
func axElementIdentifier(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if !ok {
		return 0
	}
	if s, is := n.ID.ID.(string); is && s != "" {
		return nsString(s)
	}
	return 0
}

func axElementFrame(self objc.ID, _ objc.SEL) nsRect {
	b, _, n, ok := axSelf(self)
	if !ok {
		return nsRect{}
	}
	d, is := b.plat.(*darwinAX)
	if !is || d.container == 0 {
		return nsRect{}
	}
	x, y, w, h := axBounds(n, objc.Send[nsRect](d.container, axSelBounds).size.height)
	return axToScreen(d.container, nsRect{origin: nsPoint{x: x, y: y}, size: nsSize{width: w, height: h}})
}

// axToScreen converts a rectangle in the container view's coordinates to
// the screen coordinates every accessibility frame is in, through the
// window, so that a content view inset from the window's edge is accounted
// for rather than assumed away.
func axToScreen(view objc.ID, r nsRect) nsRect {
	window := view.Send(axSelWindow)
	if window == 0 {
		return r
	}
	inWindow := objc.Send[nsRect](view, axSelConvertRectToView, r, objc.ID(0))
	return objc.Send[nsRect](window, axSelConvertRectToScreen, inWindow)
}

// axFromScreen is axToScreen backwards, for a hit test, which arrives in
// screen coordinates.
func axFromScreen(view objc.ID, r nsRect) nsRect {
	window := view.Send(axSelWindow)
	if window == 0 {
		return r
	}
	inWindow := objc.Send[nsRect](window, axSelConvertRectFromScr, r)
	return objc.Send[nsRect](view, axSelConvertRectFromView, inWindow, objc.ID(0))
}

func axElementParent(self objc.ID, _ objc.SEL) objc.ID {
	b, f, n, ok := axSelf(self)
	if !ok {
		return 0
	}
	if n.Parent < 0 {
		d, is := b.plat.(*darwinAX)
		if !is {
			return 0
		}
		return d.container
	}
	return objc.ID(b.element(axKeyOf(f.tree.At(n.Parent).ID)))
}

func axElementChildren(self objc.ID, _ objc.SEL) objc.ID {
	b, f, n, ok := axSelf(self)
	if !ok {
		return nsArray(nil)
	}
	return nsArray(axElements(b, f, n.Children))
}

// axElements turns a list of node indices into the elements standing for
// them, leaving out any the cache could not produce.
func axElements(b *axBridge, f *axFrame, idx []int) []objc.ID {
	out := make([]objc.ID, 0, len(idx))
	for _, i := range idx {
		if e := b.element(axKeyOf(f.tree.At(i).ID)); e != 0 {
			out = append(out, objc.ID(e))
		}
	}
	return out
}

func axElementEnabled(self objc.ID, _ objc.SEL) bool {
	_, _, n, ok := axSelf(self)
	return ok && !n.Disabled
}

func axElementFocused(self objc.ID, _ objc.SEL) bool {
	_, f, n, ok := axSelf(self)
	if !ok {
		return false
	}
	cur, has := f.tree.Focused()
	return has && axKeyOf(cur.ID) == axKeyOf(n.ID)
}

func axElementSelected(self objc.ID, _ objc.SEL) bool {
	_, _, n, ok := axSelf(self)
	return ok && n.Selected
}

func axElementExpanded(self objc.ID, _ objc.SEL) bool {
	_, _, n, ok := axSelf(self)
	return ok && n.Expanded != nil && *n.Expanded
}

func axElementHitTest(self objc.ID, _ objc.SEL, p nsPoint) objc.ID {
	b, _, _, ok := axSelf(self)
	if !ok {
		return 0
	}
	return axHitTestAt(b, p)
}

func axContainerIsElement(objc.ID, objc.SEL) bool { return false }

func axContainerRole(objc.ID, objc.SEL) objc.ID { return nsString("AXGroup") }

func axContainerChildren(_ objc.ID, _ objc.SEL) objc.ID {
	b := theAX.Load()
	if b == nil {
		return nsArray(nil)
	}
	f := b.frame()
	if f == nil {
		return nsArray(nil)
	}
	return nsArray(axElements(b, f, f.tree.Roots()))
}

func axContainerHitTest(_ objc.ID, _ objc.SEL, p nsPoint) objc.ID {
	b := theAX.Load()
	if b == nil {
		return 0
	}
	return axHitTestAt(b, p)
}

// axContainerMouseHitTest refuses the mouse. The container covers the whole
// window, so without this every click would land on it instead of on the
// view Ebitengine draws and listens on.
func axContainerMouseHitTest(objc.ID, objc.SEL, nsPoint) objc.ID { return 0 }

// axHitTestAt answers a hit test, which arrives in screen coordinates and
// must come back as the deepest element under the point, or nothing.
func axHitTestAt(b *axBridge, p nsPoint) objc.ID {
	f := b.frame()
	d, is := b.plat.(*darwinAX)
	if f == nil || !is || d.container == 0 {
		return 0
	}
	local := axFromScreen(d.container, nsRect{origin: p})
	h := objc.Send[nsRect](d.container, axSelBounds).size.height
	i := axHitTest(f.tree, Pt(local.origin.x, h-local.origin.y))
	if i < 0 {
		return 0
	}
	return objc.ID(b.element(axKeyOf(f.tree.At(i).ID)))
}
