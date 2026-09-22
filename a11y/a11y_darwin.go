//go:build darwin && !ios

package a11y

import (
	"math"
	"reflect"
	"runtime"
	"structs"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/cstrings"
	"github.com/ebitengine/purego/objc"

	"github.com/ironpark/ggfx"

	"github.com/ironpark/ggui/internal/platform/cocoa"
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
	axSelAlloc               = objc.RegisterName("alloc")
	axSelInit                = objc.RegisterName("init")
	axSelRelease             = objc.RegisterName("release")
	axSelContentView         = objc.RegisterName("contentView")
	axSelAddSubview          = objc.RegisterName("addSubview:")
	axSelBounds              = objc.RegisterName("bounds")
	axSelSetFrame            = objc.RegisterName("setFrame:")
	axSelSetAutoresizingMask = objc.RegisterName("setAutoresizingMask:")
	axSelWindow              = objc.RegisterName("window")
	axSelConvertRectToView   = objc.RegisterName("convertRect:toView:")
	axSelConvertRectFromView = objc.RegisterName("convertRect:fromView:")
	axSelConvertRectToScreen = objc.RegisterName("convertRectToScreen:")
	axSelConvertRectFromScr  = objc.RegisterName("convertRectFromScreen:")
	axSelNumberWithDouble    = objc.RegisterName("numberWithDouble:")
	axSelSharedWorkspace     = objc.RegisterName("sharedWorkspace")
	axSelVoiceOverEnabled    = objc.RegisterName("isVoiceOverEnabled")

	axClassNSString = objc.GetClass("NSString")
	axClassNSNumber = objc.GetClass("NSNumber")
	axClassNSView   = objc.GetClass("NSView")

	axIDNSWorkspace = objc.ID(objc.GetClass("NSWorkspace"))
)

// axRoleDescription is AppKit's own NSAccessibilityRoleDescription, which
// turns a role and a subrole into the localized noun VoiceOver says after
// the label: "button", "check box". Working it out here would mean shipping
// a translation of AppKit.
var axRoleDescription func(role, subrole uintptr) uintptr

// nsNumber returns v as an autoreleased NSNumber.
func nsNumber(v float64) objc.ID {
	return objc.ID(axClassNSNumber).Send(axSelNumberWithDouble, v)
}

// darwinAX is the platform half of the bridge: the container view, and the
// three things the portable half asks of it.
type darwinAX struct {
	container objc.ID
}

// newAXPlatform returns the macOS bridge. Nothing is created here: the
// window does not exist until Run has started, so the container is attached
// from the first poll that finds the app running.
func newAXPlatform() axPlatform { return &darwinAX{} }

// active reports whether VoiceOver is running, and takes the chance to
// attach the container view, which is work only the main thread may do.
// It is asked once a second, so the hop is nothing; asking every frame
// would put a main-thread round trip in the middle of every paint.
func (d *darwinAX) active() bool {
	var on bool
	ggfx.RunOnMainThread(func() {
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
	window := cocoa.AppWindow()
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
	purego.RegisterLibFunc(&axPost, appkit, "NSAccessibilityPostNotification")
	purego.RegisterLibFunc(&axPostWithUserInfo, appkit, "NSAccessibilityPostNotificationWithUserInfo")

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
			{Cmd: axSelPerformPress, Fn: axElementPress},
			{Cmd: axSelPerformConfirm, Fn: axElementConfirm},
			{Cmd: axSelPerformIncrement, Fn: axElementIncrement},
			{Cmd: axSelPerformDecrement, Fn: axElementDecrement},
			{Cmd: axSelPerformShowMenu, Fn: axElementShowMenu},
			{Cmd: axSelPerformPick, Fn: axElementPick},
			{Cmd: axSelSetValue, Fn: axElementSetValue},
			{Cmd: axSelSetFocused, Fn: axElementSetFocused},
			{Cmd: objc.RegisterName("isAccessibilitySelectorAllowed:"), Fn: axElementSelectorAllowed},
			{Cmd: axSelNumberOfCharacters, Fn: axElementCharacterCount},
			{Cmd: axSelSelectedText, Fn: axElementSelectedText},
			{Cmd: axSelSelectedTextRange, Fn: axElementSelectedRange},
			{Cmd: axSelSetSelectedTextRange, Fn: axElementSetSelectedRange},
			{Cmd: axSelStringForRange, Fn: axElementStringForRange},
			{Cmd: axSelRangeForLine, Fn: axElementRangeForLine},
			{Cmd: axSelLineForIndex, Fn: axElementLineForIndex},
			{Cmd: axSelFrameForRange, Fn: axElementFrameForRange},
			{Cmd: axSelInsertionPointLine, Fn: axElementInsertionLine},
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
func axSelf(self objc.ID) (*Bridge, *axFrame, SemNode, bool) {
	b := current.Load()
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
	return cocoa.String(role)
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
	return cocoa.String(sub)
}

func axElementRoleDescription(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if !ok {
		return 0
	}
	role, sub := axRole(n.Role)
	var subID objc.ID
	if sub != "" {
		subID = cocoa.String(sub)
	}
	return objc.ID(axRoleDescription(uintptr(cocoa.String(role)), uintptr(subID)))
}

func axElementLabel(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if !ok || n.Name == "" {
		return 0
	}
	return cocoa.String(n.Name)
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
	return cocoa.String(n.Description)
}

func axElementValue(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if !ok {
		return 0
	}
	if v, num := axNumber(n.Node); num {
		return nsNumber(v)
	}
	// An empty editor still has an AXValue. Returning nil makes AppKit omit
	// the attribute, so accessibility clients cannot set an initially empty
	// field even though setAccessibilityValue: is allowed.
	if n.Value == "" && !axTextual(n.Node) {
		return 0
	}
	return cocoa.String(n.Value)
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

// axElementIdentifier reports the identity a widget was given through KeyboardKey,
// when it is a string. It is what an automated test drives the app by, and
// what Accessibility Inspector shows; a node keyed by something else has
// none rather than a rendering of it.
func axElementIdentifier(self objc.ID, _ objc.SEL) objc.ID {
	_, _, n, ok := axSelf(self)
	if !ok {
		return 0
	}
	if s, is := n.ID.ID.(string); is && s != "" {
		return cocoa.String(s)
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
		return cocoa.Array(nil)
	}
	return cocoa.Array(axElements(b, f, n.Children))
}

// axElements turns a list of node indices into the elements standing for
// them, leaving out any the cache could not produce.
func axElements(b *Bridge, f *axFrame, idx []int) []objc.ID {
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

func axContainerRole(objc.ID, objc.SEL) objc.ID { return cocoa.String("AXGroup") }

func axContainerChildren(_ objc.ID, _ objc.SEL) objc.ID {
	b := current.Load()
	if b == nil {
		return cocoa.Array(nil)
	}
	f := b.frame()
	if f == nil {
		return cocoa.Array(nil)
	}
	return cocoa.Array(axElements(b, f, f.tree.Roots()))
}

func axContainerHitTest(_ objc.ID, _ objc.SEL, p nsPoint) objc.ID {
	b := current.Load()
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
func axHitTestAt(b *Bridge, p nsPoint) objc.ID {
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

// Notifications are how an assistive technology hears about a change it did
// not ask for. They must be posted from the main thread, so a frame's worth
// is batched and delivered in one hop, and only when there is something to
// deliver: a still frame, or a frame that only moved pixels, costs nothing.

var (
	axSelDictionary    = objc.RegisterName("dictionaryWithObjects:forKeys:count:")
	axSelNumberWithInt = objc.RegisterName("numberWithInt:")
	axSelIsKindOfClass = objc.RegisterName("isKindOfClass:")
	axSelDoubleValue   = objc.RegisterName("doubleValue")

	axSelPerformPress     = objc.RegisterName("accessibilityPerformPress")
	axSelPerformIncrement = objc.RegisterName("accessibilityPerformIncrement")
	axSelPerformDecrement = objc.RegisterName("accessibilityPerformDecrement")
	axSelPerformShowMenu  = objc.RegisterName("accessibilityPerformShowMenu")
	axSelPerformPick      = objc.RegisterName("accessibilityPerformPick")
	axSelPerformConfirm   = objc.RegisterName("accessibilityPerformConfirm")
	axSelSetValue         = objc.RegisterName("setAccessibilityValue:")
	axSelSetFocused       = objc.RegisterName("setAccessibilityFocused:")
)

// axPost and axPostWithUserInfo are AppKit's own posting functions. The
// notification names are the values of the NSAccessibility*Notification
// constants, which, like the role names, are part of the API.
var (
	axPost             func(element, notification uintptr)
	axPostWithUserInfo func(element, notification, userInfo uintptr)
)

// axNoticeName is the AppKit notification for a kind of change.
func axNoticeName(k axNotice) string {
	switch k {
	case axValueChanged:
		return "AXValueChanged"
	case axSelectionChanged:
		return "AXSelectedChildrenChanged"
	case axFocusChanged:
		return "AXFocusedUIElementChanged"
	case axAnnouncement:
		return "AXAnnouncementRequested"
	}
	return "AXLayoutChanged"
}

// axPriority maps Politeness onto NSAccessibilityPriorityLevel: medium for
// a polite announcement, which waits its turn, and high for an assertive
// one, which interrupts.
func axPriority(loud bool) int32 {
	if loud {
		return 90
	}
	return 50
}

// notify posts a frame's notifications in one hop to the main thread, which
// is the only thread they may be posted from. Elements are resolved here
// rather than in the diff, because making one is also main-thread work.
func (d *darwinAX) notify(notes []axNote) {
	ggfx.RunOnMainThread(func() {
		b := current.Load()
		if b == nil || d.container == 0 {
			return
		}
		for _, n := range notes {
			d.post(b, n)
		}
	})
}

// post delivers one notification. It runs on the main thread.
func (d *darwinAX) post(b *Bridge, n axNote) {
	target := d.container
	if !n.root {
		e := b.element(n.key)
		if e == 0 {
			return
		}
		target = objc.ID(e)
	}
	if n.kind != axAnnouncement {
		axPost(uintptr(target), uintptr(cocoa.String(axNoticeName(n.kind))))
		return
	}
	// An announcement is addressed to the window rather than to an element:
	// it is not about anything on screen, which is the whole reason a live
	// region needs one.
	window := d.container.Send(axSelWindow)
	if window == 0 {
		window = target
	}
	values := []objc.ID{cocoa.String(n.text), objc.ID(axClassNSNumber).Send(axSelNumberWithInt, axPriority(n.loud))}
	keys := []objc.ID{cocoa.String("AXAnnouncementKey"), cocoa.String("AXPriorityKey")}
	info := objc.ID(objc.GetClass("NSDictionary")).Send(axSelDictionary,
		unsafe.Pointer(&values[0]), unsafe.Pointer(&keys[0]), 2)
	runtime.KeepAlive(values)
	runtime.KeepAlive(keys)
	axPostWithUserInfo(uintptr(window), uintptr(cocoa.String("AXAnnouncementRequested")), uintptr(info))
}

// axPerform hands one action to the app and answers the platform at once,
// without waiting to find out what came of it. The result says only that
// the action was accepted, which is all a caller blocking the main thread
// can safely be told; the next frame's tree says what actually happened.
func axPerform(self objc.ID, a axAct, act Action) bool {
	b, _, n, ok := axSelf(self)
	if !ok || !axAllows(n.Node, a) {
		return false
	}
	act.Kind = axActOf(a)
	b.perform(n.ID, act)
	return true
}

func axElementPress(self objc.ID, _ objc.SEL) bool {
	return axPerform(self, axPress, Action{})
}

func axElementConfirm(self objc.ID, _ objc.SEL) bool {
	return axPerform(self, axConfirm, Action{})
}

func axElementIncrement(self objc.ID, _ objc.SEL) bool {
	return axPerform(self, axIncrement, Action{})
}

func axElementDecrement(self objc.ID, _ objc.SEL) bool {
	return axPerform(self, axDecrement, Action{})
}

func axElementShowMenu(self objc.ID, _ objc.SEL) bool {
	return axPerform(self, axShowMenu, Action{})
}

func axElementPick(self objc.ID, _ objc.SEL) bool {
	return axPerform(self, axPick, Action{})
}

// axElementSetValue takes whatever the platform's text or numeric API
// replaced the value with. A slider is given a number and everything else a
// string, which is what dictation and a braille display send.
func axElementSetValue(self objc.ID, _ objc.SEL, v objc.ID) {
	if _, _, _, ok := axSelf(self); !ok || v == 0 {
		return
	}
	var act Action
	if v.Send(axSelIsKindOfClass, axClassNSString) != 0 {
		act.Text = cstrings.NSStringToString(v)
	} else {
		act.Num = objc.Send[float64](v, axSelDoubleValue)
	}
	axPerform(self, axSetValue, act)
}

// axElementSetFocused moves keyboard focus onto the node, and scrolls it
// into view first when it is clipped out of one: focus is what VoiceOver
// uses to walk the tree, and a node it cannot see must be brought where the
// sighted user can. Unfocusing is left alone; ggui moves focus, it does not
// clear it.
func axElementSetFocused(self objc.ID, _ objc.SEL, on bool) {
	if !on {
		return
	}
	_, _, n, ok := axSelf(self)
	if ok && n.Offscreen && n.Actions.Has(ActionScrollIntoView) {
		if b := current.Load(); b != nil {
			b.perform(n.ID, Action{Kind: ActionScrollIntoView})
		}
	}
	axPerform(self, axSetFocus, Action{})
}

// axElementSelectorAllowed is how AppKit asks which actions this element
// offers, and so what an assistive technology puts in front of the user.
// Without it every element on the one registered class would advertise
// every action, since the class responds to all of them.
func axElementSelectorAllowed(self objc.ID, _ objc.SEL, sel objc.SEL) bool {
	_, _, n, ok := axSelf(self)
	if !ok {
		return false
	}
	if axTextSelector(sel) {
		return axTextual(n.Node)
	}
	a, isAction := axActOfSel(sel)
	if !isAction {
		return true
	}
	return axAllows(n.Node, a)
}

// axActOfSel maps an AppKit action selector onto the action it asks for.
// The selectors are compared by value rather than by name: a SEL is unique
// for a name, so the ones registered at startup are the ones that arrive.
func axActOfSel(sel objc.SEL) (axAct, bool) {
	switch sel {
	case axSelPerformPress:
		return axPress, true
	case axSelPerformConfirm:
		return axConfirm, true
	case axSelPerformIncrement:
		return axIncrement, true
	case axSelPerformDecrement:
		return axDecrement, true
	case axSelPerformShowMenu:
		return axShowMenu, true
	case axSelPerformPick:
		return axPick, true
	case axSelSetValue:
		return axSetValue, true
	case axSelSetFocused:
		return axSetFocus, true
	}
	return 0, false
}

// The text protocol. Without it VoiceOver knows a text field is there and
// will not read a word of what is in it: reading a field means asking for
// its characters, its lines and the rectangle a range of it covers, in
// UTF-16 offsets. Every answer comes out of the node the last frame froze,
// which carries the selection and, while this is being asked, the layout.

type nsRange struct {
	_        structs.HostLayout
	location uint
	length   uint
}

// axNotFound is NSNotFound: the answer to a question about a range that
// does not exist.
const axNotFound = uint(math.MaxInt)

var (
	axSelNumberOfCharacters   = objc.RegisterName("accessibilityNumberOfCharacters")
	axSelSelectedText         = objc.RegisterName("accessibilitySelectedText")
	axSelSelectedTextRange    = objc.RegisterName("accessibilitySelectedTextRange")
	axSelSetSelectedTextRange = objc.RegisterName("setAccessibilitySelectedTextRange:")
	axSelStringForRange       = objc.RegisterName("accessibilityStringForRange:")
	axSelRangeForLine         = objc.RegisterName("accessibilityRangeForLine:")
	axSelLineForIndex         = objc.RegisterName("accessibilityLineForIndex:")
	axSelFrameForRange        = objc.RegisterName("accessibilityFrameForRange:")
	axSelInsertionPointLine   = objc.RegisterName("accessibilityInsertionPointLineNumber")
)

// axTextSelector reports whether a selector belongs to the text protocol,
// which only a text field should be seen to answer.
func axTextSelector(sel objc.SEL) bool {
	switch sel {
	case axSelNumberOfCharacters, axSelSelectedText, axSelSelectedTextRange,
		axSelSetSelectedTextRange, axSelStringForRange, axSelRangeForLine,
		axSelLineForIndex, axSelFrameForRange, axSelInsertionPointLine:
		return true
	}
	return false
}

// axText returns the node behind an element when it is a text field, which
// is the only kind that answers any of this.
func axText(self objc.ID) (*Bridge, SemNode, bool) {
	b, _, n, ok := axSelf(self)
	if !ok || !axTextual(n.Node) {
		return nil, SemNode{}, false
	}
	return b, n, true
}

func axElementCharacterCount(self objc.ID, _ objc.SEL) int {
	_, n, ok := axText(self)
	if !ok {
		return 0
	}
	return CharCount(n.Node)
}

func axElementSelectedText(self objc.ID, _ objc.SEL) objc.ID {
	_, n, ok := axText(self)
	if !ok {
		return 0
	}
	return cocoa.String(Selected(n.Node))
}

func axElementSelectedRange(self objc.ID, _ objc.SEL) nsRange {
	_, n, ok := axText(self)
	if !ok {
		return nsRange{location: axNotFound}
	}
	loc, length := Selection(n.Node)
	return nsRange{location: uint(loc), length: uint(length)}
}

// axElementSetSelectedRange moves the caret, which is how a screen reader
// reads along a field. Like every other action it is queued and answered at
// once; the next frame's tree shows where the caret ended up.
func axElementSetSelectedRange(self objc.ID, _ objc.SEL, r nsRange) {
	b, n, ok := axText(self)
	if !ok || r.location >= axNotFound || !axAllows(n.Node, axSetSelection) {
		return
	}
	start, end := ByteRange(n.Node, int(r.location), int(r.length))
	b.perform(n.ID, Action{Kind: ActionSetSelection, SelStart: start, SelEnd: end})
}

func axElementStringForRange(self objc.ID, _ objc.SEL, r nsRange) objc.ID {
	_, n, ok := axText(self)
	if !ok || r.location >= axNotFound {
		return 0
	}
	return cocoa.String(StringForRange(n.Node, int(r.location), int(r.length)))
}

func axElementRangeForLine(self objc.ID, _ objc.SEL, line int) nsRange {
	_, n, ok := axText(self)
	if !ok {
		return nsRange{location: axNotFound}
	}
	loc, length, has := RangeForLine(n.Node, line)
	if !has {
		return nsRange{location: axNotFound}
	}
	return nsRange{location: uint(loc), length: uint(length)}
}

func axElementLineForIndex(self objc.ID, _ objc.SEL, index int) int {
	_, n, ok := axText(self)
	if !ok {
		return 0
	}
	return LineForIndex(n.Node, index)
}

func axElementInsertionLine(self objc.ID, _ objc.SEL) int {
	_, n, ok := axText(self)
	if !ok {
		return 0
	}
	return InsertionLine(n.Node)
}

func axElementFrameForRange(self objc.ID, _ objc.SEL, r nsRange) nsRect {
	b, n, ok := axText(self)
	if !ok || r.location >= axNotFound {
		return nsRect{}
	}
	d, is := b.plat.(*darwinAX)
	if !is || d.container == 0 {
		return nsRect{}
	}
	box := RectForRange(n, int(r.location), int(r.length))
	h := objc.Send[nsRect](d.container, axSelBounds).size.height
	return axToScreen(d.container, nsRect{
		origin: nsPoint{x: box.Origin.X, y: h - (box.Origin.Y + box.Size.H)},
		size:   nsSize{width: box.Size.W, height: box.Size.H},
	})
}

// axRole maps a ggui role onto the role and subrole an AppKit accessibility
// element reports. The names are the values of the NSAccessibility*Role
// constants rather than the constants themselves, which are NSStrings that
// would have to be looked up out of AppKit one at a time; they are part of
// the API and do not change. An empty subrole means the element reports
// none, which is the usual case.
//
// A few of these are not one-for-one, and the choice is the one that makes
// VoiceOver say the right thing rather than the one that reads best in a
// table. A switch is a check box with the switch subrole, because AppKit
// has no switch role. A tab is a radio button with the tab subrole, which
// is what a real NSTabView reports. A dialog is a window with the dialog
// subrole, so that VoiceOver treats it as a thing to be dismissed.
func axRole(r Role) (role, subrole string) {
	switch r {
	case RoleButton:
		return "AXButton", ""
	case RoleCheckbox:
		return "AXCheckBox", ""
	case RoleRadio:
		return "AXRadioButton", ""
	case RoleSwitch:
		return "AXCheckBox", "AXSwitch"
	case RoleSlider:
		return "AXSlider", ""
	case RoleTextField:
		return "AXTextField", ""
	case RoleSelect:
		return "AXPopUpButton", ""
	case RoleOption:
		return "AXMenuItem", ""
	case RoleMenu:
		return "AXMenu", ""
	case RoleMenuItem:
		return "AXMenuItem", ""
	case RoleTab:
		return "AXRadioButton", "AXTabButton"
	case RoleTabs:
		return "AXTabGroup", ""
	case RoleDisclosure:
		return "AXDisclosureTriangle", ""
	case RoleDialog:
		return "AXWindow", "AXDialog"
	case RoleRow, RoleListItem:
		return "AXRow", ""
	case RoleAccordion:
		return "AXGroup", "AXDisclosureTriangle"
	case RoleCombobox:
		return "AXComboBox", ""
	case RoleSeparator:
		return "AXSplitter", ""
	case RoleText:
		return "AXStaticText", ""
	case RoleHeading:
		return "AXHeading", ""
	case RoleImage:
		return "AXImage", ""
	case RoleList:
		return "AXList", ""
	case RoleProgress:
		return "AXProgressIndicator", ""
	case RoleLink:
		return "AXLink", ""
	case RoleToolbar:
		return "AXToolbar", ""
	case RoleStatus:
		return "AXGroup", ""
	case RoleWindow:
		return "AXWindow", ""
	case RoleGroup:
		return "AXGroup", ""
	}
	return "AXUnknown", ""
}

// axBounds is the rectangle an element reports, in the window's own
// coordinates: logical pixels, which are what AppKit calls points, with the
// y axis flipped, since ggui measures down from the top of the window and
// Cocoa measures up from the bottom. The caller turns the result into
// screen coordinates, which is the one step that needs the window.
//
// The bounds are the ones the node painted at rather than the ones it was
// clipped to, so that a row scrolled out of a list still says where it
// would be; an element that is offscreen says so separately.
func axBounds(n SemNode, viewHeight float64) (x, y, w, h float64) {
	r := n.Full
	return r.Origin.X, viewHeight - (r.Origin.Y + r.Size.H), r.Size.W, r.Size.H
}
