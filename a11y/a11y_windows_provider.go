//go:build windows

package a11y

import (
	"math"
	"unicode/utf16"
	"unsafe"
)

// The provider methods: what UI Automation asks an element, answered from
// the frame the bridge last published. Every one of them may run on any
// thread, so every one of them reads the frame through the bridge and
// touches nothing else.
//
// A method that cannot answer says so rather than guessing. An element
// whose node has left the tree answers UIA_E_ELEMENTNOTAVAILABLE, which is
// the honest reply to a question about something that is gone.

const ptrSize = unsafe.Sizeof(uintptr(0))

// winHandOut writes one of an object's interface slots into a caller's out
// parameter, with the reference COM requires the caller to own. A zero base
// means there is nothing there, which is a success with a null result:
// "no parent" is an answer, not a failure.
func winHandOut(base uintptr, k int, ppv uintptr) uintptr {
	if ppv == 0 {
		return ePointer
	}
	*(*uintptr)(unsafe.Pointer(ppv)) = 0
	if base == 0 {
		return sOK
	}
	slot := base + uintptr(k)*ptrSize
	comAddRef(slot)
	*(*uintptr)(unsafe.Pointer(ppv)) = slot
	return sOK
}

// winElementAt returns the cached provider object for tree index i.
func winElementAt(b *Bridge, f *axFrame, i int) uintptr {
	if i < 0 || i >= f.tree.Len() {
		return 0
	}
	return b.element(axKeyOf(f.tree.At(i).ID))
}

// winIndexOf finds where a node sits in the frame it came from.
func winIndexOf(f *axFrame, n SemNode) int {
	if i, ok := f.index[axKeyOf(n.ID)]; ok {
		return int(i)
	}
	return -1
}

// winSiblings is the list a node appears in among its peers: its parent's
// children, or the tree's roots when it has no parent.
func winSiblings(f *axFrame, n SemNode) []int {
	if n.Parent < 0 || n.Parent >= f.tree.Len() {
		return f.tree.Roots()
	}
	return f.tree.At(n.Parent).Children
}

// winFrame is the frame a query is answered from, or nil when the bridge
// has published none.
func winFrame(b *Bridge) *axFrame {
	if b == nil {
		return nil
	}
	return b.frame()
}

// --- IRawElementProviderSimple ---

// winGetProviderOptions says this is a server-side provider that does its
// own thread safety; see the note on uiaProviderOptionsServerSide.
func winGetProviderOptions(_, pRetVal uintptr) uintptr {
	if pRetVal == 0 {
		return ePointer
	}
	*(*int32)(unsafe.Pointer(pRetVal)) = uiaProviderOptionsServerSide
	return sOK
}

// winGetPatternProvider hands back the object itself for a pattern the node
// supports, and nothing for one it does not. Saying nothing is how an
// element tells a screen reader that a control it might have offered is not
// on the table, which is better than offering one that fails.
func winGetPatternProvider(this, patternID, ppRetVal uintptr) uintptr {
	o, _, ok := objOf(this)
	if !ok {
		return winHandOut(0, 0, ppRetVal)
	}
	k := winPatternIface(int32(patternID))
	if k < 0 || !o.offers(k) {
		return winHandOut(0, 0, ppRetVal)
	}
	return winHandOut(uintptr(unsafe.Pointer(o)), k, ppRetVal)
}

// winPatternIface maps a UIA pattern identifier onto the interface that
// implements it here.
func winPatternIface(id int32) int {
	switch id {
	case uiaInvokePatternID:
		return ifInvoke
	case uiaValuePatternID:
		return ifValue
	case uiaRangeValuePatternID:
		return ifRangeValue
	case uiaTogglePatternID:
		return ifToggle
	case uiaExpandCollapsePatternID:
		return ifExpandCollapse
	case uiaSelectionItemPatternID:
		return ifSelectionItem
	}
	return -1
}

// winPattern reports whether a node supports the pattern interface k. It is
// what both QueryInterface and GetPatternProvider answer on, so that the
// two can never disagree about what an element can do.
func winPattern(n Node, k int) bool {
	switch k {
	case ifInvoke:
		return axAllows(n, axPress)
	case ifValue:
		switch n.Role {
		case RoleTextField, RoleSelect, RoleCombobox:
			return true
		}
		return false
	case ifRangeValue:
		return n.Role == RoleSlider || n.Role == RoleProgress
	case ifToggle:
		return n.Checked != TriNone
	case ifExpandCollapse:
		return n.Expanded != nil
	case ifSelectionItem:
		return n.Selected || axAllows(n, axPick)
	}
	return false
}

// winGetPropertyValue answers the properties a screen reader reads out.
// Everything else is left empty, which tells UIA to fall back to whatever
// default it has rather than to report an error.
func winGetPropertyValue(this, propertyID, pRetVal uintptr) uintptr {
	if pRetVal == 0 {
		return ePointer
	}
	v := (*winVariant)(unsafe.Pointer(pRetVal))
	varEmpty(v)
	o, b, n, ok := selfOf(this)
	if o == nil {
		return eFail
	}
	if o.handle == winRootHandle {
		return winRootProperty(int32(propertyID), v)
	}
	if !ok {
		return uiaElementNotAvailable
	}
	switch int32(propertyID) {
	case uiaControlTypeProperty:
		ctl, _ := axControlType(n.Role)
		return varI4(v, ctl)
	case uiaLocalizedControlTypeProperty:
		if _, name := axControlType(n.Role); name != "" {
			return varStr(v, name)
		}
		return sOK
	case uiaNameProperty:
		if n.Name == "" {
			return sOK
		}
		return varStr(v, n.Name)
	case uiaHelpTextProperty:
		if n.Description == "" {
			return sOK
		}
		return varStr(v, n.Description)
	case uiaIsEnabledProperty:
		return varBool(v, !n.Disabled)
	case uiaIsOffscreenProperty:
		return varBool(v, n.Offscreen)
	case uiaIsKeyboardFocusableProperty:
		return varBool(v, axAllows(n.Node, axSetFocus))
	case uiaHasKeyboardFocusProperty:
		f := winFrame(b)
		return varBool(v, f != nil && axFocusKey(f) == axKeyOf(n.ID))
	case uiaIsControlElementProperty, uiaIsContentElementProperty:
		return varBool(v, true)
	case uiaValueValueProperty:
		if !winPattern(n.Node, ifValue) {
			return sOK
		}
		return varStr(v, n.Value)
	case uiaRangeValueValueProperty:
		if !winPattern(n.Node, ifRangeValue) {
			return sOK
		}
		return varR8(v, n.Now)
	case uiaToggleToggleStateProperty:
		if n.Checked == TriNone {
			return sOK
		}
		return varI4(v, winToggleOf(n.Node))
	case uiaExpandCollapseStateProperty:
		if n.Expanded == nil {
			return sOK
		}
		if *n.Expanded {
			return varI4(v, uiaExpanded)
		}
		return varI4(v, uiaCollapsed)
	case uiaSelectionItemIsSelectedProperty:
		if !winPattern(n.Node, ifSelectionItem) {
			return sOK
		}
		return varBool(v, n.Selected)
	case uiaOrientationProperty:
		if n.Role != RoleSlider && n.Role != RoleSeparator {
			return sOK
		}
		if n.Full.Size.W >= n.Full.Size.H {
			return varI4(v, uiaOrientationHorizontal)
		}
		return varI4(v, uiaOrientationVertical)
	}
	return sOK
}

// winRootProperty is what the window's own element reports: a pane with no
// name, since the window already has a title and repeating it makes a
// screen reader say everything twice.
func winRootProperty(propertyID int32, v *winVariant) uintptr {
	switch propertyID {
	case uiaControlTypeProperty:
		return varI4(v, uiaPaneControlType)
	case uiaIsControlElementProperty, uiaIsContentElementProperty:
		return varBool(v, true)
	case uiaIsEnabledProperty:
		return varBool(v, true)
	case uiaIsKeyboardFocusableProperty, uiaHasKeyboardFocusProperty, uiaIsOffscreenProperty:
		return varBool(v, false)
	}
	return sOK
}

// winGetHostProvider gives UIA the window's own provider, so that it can
// fill in everything a window has that a drawn tree does not: the title,
// the process, the position among the desktop's windows. Only the root has
// one; a node inside it is not a window.
func winGetHostProvider(this, ppRetVal uintptr) uintptr {
	if ppRetVal == 0 {
		return ePointer
	}
	*(*uintptr)(unsafe.Pointer(ppRetVal)) = 0
	o, _, ok := objOf(this)
	if !ok || o.handle != winRootHandle {
		return sOK
	}
	h := winHWND.Load()
	if h == 0 {
		return sOK
	}
	r, _, _ := procUiaHostProviderFromHwnd.Call(h, ppRetVal)
	if r != sOK {
		*(*uintptr)(unsafe.Pointer(ppRetVal)) = 0
	}
	return sOK
}

// --- IRawElementProviderFragment ---

// winNavigate walks the published tree. The root's children are the tree's
// roots, and a top-level node's parent is the root, which is what joins the
// drawn tree to the window it is drawn in.
func winNavigate(this, direction, ppRetVal uintptr) uintptr {
	o, b, n, ok := selfOf(this)
	if o == nil {
		return winHandOut(0, 0, ppRetVal)
	}
	f := winFrame(b)
	if f == nil {
		return winHandOut(0, 0, ppRetVal)
	}
	root := winRoot.Load()
	if o.handle == winRootHandle {
		roots := f.tree.Roots()
		var el uintptr
		switch direction {
		case uiaNavigateFirstChild:
			if len(roots) > 0 {
				el = winElementAt(b, f, roots[0])
			}
		case uiaNavigateLastChild:
			if len(roots) > 0 {
				el = winElementAt(b, f, roots[len(roots)-1])
			}
		}
		return winHandOut(el, ifFragment, ppRetVal)
	}
	if !ok {
		return winHandOut(0, 0, ppRetVal)
	}
	switch direction {
	case uiaNavigateParent:
		if n.Parent < 0 {
			return winHandOut(root, ifFragment, ppRetVal)
		}
		return winHandOut(winElementAt(b, f, n.Parent), ifFragment, ppRetVal)
	case uiaNavigateFirstChild:
		if len(n.Children) == 0 {
			return winHandOut(0, 0, ppRetVal)
		}
		return winHandOut(winElementAt(b, f, n.Children[0]), ifFragment, ppRetVal)
	case uiaNavigateLastChild:
		if len(n.Children) == 0 {
			return winHandOut(0, 0, ppRetVal)
		}
		return winHandOut(winElementAt(b, f, n.Children[len(n.Children)-1]), ifFragment, ppRetVal)
	case uiaNavigateNextSibling, uiaNavigatePreviousSibling:
		self := winIndexOf(f, n)
		peers := winSiblings(f, n)
		for at, p := range peers {
			if p != self {
				continue
			}
			want := at + 1
			if direction == uiaNavigatePreviousSibling {
				want = at - 1
			}
			if want < 0 || want >= len(peers) {
				return winHandOut(0, 0, ppRetVal)
			}
			return winHandOut(winElementAt(b, f, peers[want]), ifFragment, ppRetVal)
		}
	}
	return winHandOut(0, 0, ppRetVal)
}

// winGetRuntimeID gives an element an identity that survives a frame. The
// root has none of its own: its host provider supplies it, which is what
// UIA expects of a fragment root sitting on a window.
func winGetRuntimeID(this, ppRetVal uintptr) uintptr {
	if ppRetVal == 0 {
		return ePointer
	}
	*(*uintptr)(unsafe.Pointer(ppRetVal)) = 0
	o, _, ok := objOf(this)
	if !ok || o.handle == winRootHandle {
		return sOK
	}
	*(*uintptr)(unsafe.Pointer(ppRetVal)) = winRuntimeID(o.handle)
	return sOK
}

// winBoundingRectangle is where the element is on the desktop, in physical
// pixels, which is the one coordinate space UIA works in. The tree measures
// in logical pixels from the top left of the client area, so the answer is
// scaled by the window's DPI and then offset by where that client area
// starts on screen.
//
// The rectangle is the one the node painted at rather than the one it was
// clipped to, so that a row scrolled out of a list still says where it
// would be; an element that is offscreen says so separately.
func winBoundingRectangle(this, pRetVal uintptr) uintptr {
	if pRetVal == 0 {
		return ePointer
	}
	r := (*winRect)(unsafe.Pointer(pRetVal))
	*r = winRect{}
	o, _, n, ok := selfOf(this)
	if o == nil {
		return eFail
	}
	h := winHWND.Load()
	if h == 0 {
		return sOK
	}
	ox, oy := winClientOrigin(h)
	s := winScale(h)
	if o.handle == winRootHandle {
		w, ht := winClientSize(h)
		*r = winRect{left: ox, top: oy, width: w, height: ht}
		return sOK
	}
	if !ok {
		return uiaElementNotAvailable
	}
	f := n.Full
	*r = winRect{
		left:   ox + f.Origin.X*s,
		top:    oy + f.Origin.Y*s,
		width:  f.Size.W * s,
		height: f.Size.H * s,
	}
	return sOK
}

// winRect is UiaRect: where an element is, in physical screen pixels.
type winRect struct{ left, top, width, height float64 }

// winGetEmbeddedFragmentRoots is always empty: everything drawn here
// belongs to the one tree.
func winGetEmbeddedFragmentRoots(_, ppRetVal uintptr) uintptr {
	if ppRetVal == 0 {
		return ePointer
	}
	*(*uintptr)(unsafe.Pointer(ppRetVal)) = 0
	return sOK
}

// winSetFocus asks the app to move keyboard focus here. Like every action,
// it queues and returns; the next frame is where the answer shows up.
func winSetFocus(this uintptr) uintptr {
	return winAct(this, axSetFocus, Action{Kind: ActionFocus})
}

// winGetFragmentRoot is the window's element, for every element including
// itself.
func winGetFragmentRoot(_, ppRetVal uintptr) uintptr {
	return winHandOut(winRoot.Load(), ifFragmentRoot, ppRetVal)
}

// --- IRawElementProviderFragmentRoot ---

// winElementFromPoint finds the deepest element under a point given in
// physical screen pixels, which is how a screen reader follows the mouse.
// The point is converted back into the tree's own logical pixels before the
// tree is walked.
//
// The two coordinates arrive as raw bits because a Go callback cannot
// receive a floating-point argument; see the thunk that puts them where
// this can read them.
func winElementFromPoint(this, xBits, yBits, ppRetVal uintptr) uintptr {
	_, b, _, _ := selfOf(this)
	f := winFrame(b)
	h := winHWND.Load()
	if f == nil || h == 0 {
		return winHandOut(0, 0, ppRetVal)
	}
	ox, oy := winClientOrigin(h)
	s := winScale(h)
	if s == 0 {
		return winHandOut(0, 0, ppRetVal)
	}
	p := Pt(
		(math.Float64frombits(uint64(xBits))-ox)/s,
		(math.Float64frombits(uint64(yBits))-oy)/s,
	)
	i := axHitTest(f.tree, p)
	if i < 0 {
		return winHandOut(0, 0, ppRetVal)
	}
	return winHandOut(winElementAt(b, f, i), ifFragment, ppRetVal)
}

// winGetFocus is the element keyboard focus was on when the frame was
// published.
func winGetFocus(this, ppRetVal uintptr) uintptr {
	_, b, _, _ := selfOf(this)
	f := winFrame(b)
	if f == nil {
		return winHandOut(0, 0, ppRetVal)
	}
	n, ok := f.tree.Focused()
	if !ok {
		return winHandOut(0, 0, ppRetVal)
	}
	return winHandOut(b.element(axKeyOf(n.ID)), ifFragment, ppRetVal)
}

// --- the patterns ---

// winAct is what every pattern method that does something comes down to:
// check the node still offers the action, then queue it. Nothing waits for
// the result, because waiting is the one thing that deadlocks; see the note
// at the top of actions.go.
func winAct(this uintptr, want axAct, a Action) uintptr {
	_, b, n, ok := selfOf(this)
	if !ok {
		return uiaElementNotAvailable
	}
	if !axAllows(n.Node, want) {
		return uiaInvalidOperation
	}
	b.perform(n.ID, a)
	return sOK
}

func winInvoke(this uintptr) uintptr {
	return winAct(this, axPress, Action{Kind: ActionPress})
}

// winValueSetValue replaces a text field's contents outright. The string
// arrives as a BSTR, which is UTF-16 and counted, but is always terminated
// too, so it reads like any other wide string.
func winValueSetValue(this, val uintptr) uintptr {
	text := ""
	if val != 0 {
		text = winFromBSTR(val)
	}
	return winAct(this, axSetValue, Action{Kind: ActionSetValue, Text: text})
}

func winValueGetValue(this, ppRetVal uintptr) uintptr {
	if ppRetVal == 0 {
		return ePointer
	}
	*(*uintptr)(unsafe.Pointer(ppRetVal)) = 0
	_, _, n, ok := selfOf(this)
	if !ok {
		return uiaElementNotAvailable
	}
	*(*uintptr)(unsafe.Pointer(ppRetVal)) = winStr(n.Value)
	return sOK
}

func winValueIsReadOnly(this, pRetVal uintptr) uintptr {
	return winBoolOut(this, pRetVal, func(n Node) bool { return !axAllows(n, axSetValue) })
}

// winRangeSetValue moves a slider to a number. Like the hit test, the
// number arrives as raw bits from a thunk.
func winRangeSetValue(this, valBits uintptr) uintptr {
	return winAct(this, axSetValue, Action{
		Kind: ActionSetValue,
		Num:  math.Float64frombits(uint64(valBits)),
	})
}

func winRangeGetValue(this, pRetVal uintptr) uintptr {
	return winFloatOut(this, pRetVal, func(n Node) float64 { return n.Now })
}

func winRangeMinimum(this, pRetVal uintptr) uintptr {
	return winFloatOut(this, pRetVal, func(n Node) float64 { return n.Min })
}

func winRangeMaximum(this, pRetVal uintptr) uintptr {
	return winFloatOut(this, pRetVal, func(n Node) float64 { return n.Max })
}

// winRangeSmallChange and winRangeLargeChange are what one press of an
// arrow key and one of a page key move a slider by. The tree does not carry
// a step, so these are the hundredth and the tenth of the range that a
// slider without one conventionally moves in.
func winRangeSmallChange(this, pRetVal uintptr) uintptr {
	return winFloatOut(this, pRetVal, func(n Node) float64 { return (n.Max - n.Min) / 100 })
}

func winRangeLargeChange(this, pRetVal uintptr) uintptr {
	return winFloatOut(this, pRetVal, func(n Node) float64 { return (n.Max - n.Min) / 10 })
}

func winRangeIsReadOnly(this, pRetVal uintptr) uintptr {
	return winBoolOut(this, pRetVal, func(n Node) bool { return !axAllows(n, axSetValue) })
}

func winToggle(this uintptr) uintptr {
	return winAct(this, axPress, Action{Kind: ActionPress})
}

func winToggleState(this, pRetVal uintptr) uintptr {
	if pRetVal == 0 {
		return ePointer
	}
	*(*int32)(unsafe.Pointer(pRetVal)) = uiaToggleOff
	_, _, n, ok := selfOf(this)
	if !ok {
		return uiaElementNotAvailable
	}
	*(*int32)(unsafe.Pointer(pRetVal)) = winToggleOf(n.Node)
	return sOK
}

func winExpand(this uintptr) uintptr {
	return winAct(this, axShowMenu, Action{Kind: ActionExpand})
}

// winCollapse closes what Expand opened. Collapsing has no axAct of its
// own, since AppKit asks for both through one action, so the check is done
// here against the node's own claim.
func winCollapse(this uintptr) uintptr {
	_, b, n, ok := selfOf(this)
	if !ok {
		return uiaElementNotAvailable
	}
	if n.Disabled || !n.Actions.Has(ActionCollapse) {
		return uiaInvalidOperation
	}
	b.perform(n.ID, Action{Kind: ActionCollapse})
	return sOK
}

func winExpandCollapseState(this, pRetVal uintptr) uintptr {
	if pRetVal == 0 {
		return ePointer
	}
	*(*int32)(unsafe.Pointer(pRetVal)) = uiaLeafNode
	_, _, n, ok := selfOf(this)
	if !ok {
		return uiaElementNotAvailable
	}
	switch {
	case n.Expanded == nil:
		*(*int32)(unsafe.Pointer(pRetVal)) = uiaLeafNode
	case *n.Expanded:
		*(*int32)(unsafe.Pointer(pRetVal)) = uiaExpanded
	default:
		*(*int32)(unsafe.Pointer(pRetVal)) = uiaCollapsed
	}
	return sOK
}

func winSelect(this uintptr) uintptr {
	return winAct(this, axPick, Action{Kind: ActionSelect})
}

// winAddToSelection and winRemoveFromSelection are refused: nothing ggui
// describes holds more than one selected child at a time, so adding is
// either the same as selecting or a lie, and removing is neither.
func winAddToSelection(this uintptr) uintptr { return winSelect(this) }

func winRemoveFromSelection(uintptr) uintptr { return uiaInvalidOperation }

func winIsSelected(this, pRetVal uintptr) uintptr {
	return winBoolOut(this, pRetVal, func(n Node) bool { return n.Selected })
}

// winSelectionContainer is the nearest ancestor that holds the selection,
// which is the element's parent whenever there is one.
func winSelectionContainer(this, ppRetVal uintptr) uintptr {
	_, b, n, ok := selfOf(this)
	f := winFrame(b)
	if !ok || f == nil || n.Parent < 0 {
		return winHandOut(0, 0, ppRetVal)
	}
	return winHandOut(winElementAt(b, f, n.Parent), ifSimple, ppRetVal)
}

// --- small shared shapes ---

// winBoolOut answers a BOOL out parameter from the node. A pattern
// interface returns WINBOOL, which is a four byte int and is one for true,
// not the two byte all-bits-set VARIANT_BOOL that goes inside a variant:
// writing the smaller one leaves the caller's upper half as it found it,
// and whatever was on its stack then reads as true.
func winBoolOut(this, pRetVal uintptr, of func(Node) bool) uintptr {
	if pRetVal == 0 {
		return ePointer
	}
	*(*int32)(unsafe.Pointer(pRetVal)) = 0
	_, _, n, ok := selfOf(this)
	if !ok {
		return uiaElementNotAvailable
	}
	if of(n.Node) {
		*(*int32)(unsafe.Pointer(pRetVal)) = 1
	}
	return sOK
}

// winFloatOut answers a double out parameter from the node.
func winFloatOut(this, pRetVal uintptr, of func(Node) float64) uintptr {
	if pRetVal == 0 {
		return ePointer
	}
	*(*float64)(unsafe.Pointer(pRetVal)) = 0
	_, _, n, ok := selfOf(this)
	if !ok {
		return uiaElementNotAvailable
	}
	*(*float64)(unsafe.Pointer(pRetVal)) = of(n.Node)
	return sOK
}

// winFromBSTR reads a wide string the caller owns into a Go string. The
// length prefix is ignored in favour of the terminator, which a BSTR from
// SysAllocString always has.
func winFromBSTR(p uintptr) string {
	var u []uint16
	for i := uintptr(0); ; i += 2 {
		c := *(*uint16)(unsafe.Pointer(p + i))
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

// --- window geometry ---

// winScale is the window's DPI over the ninety-six a logical pixel is
// defined against. GetDpiForWindow is not on every Windows this may run on,
// and its absence means the process is not per-monitor aware, in which case
// one is the right answer.
func winScale(hwnd uintptr) float64 {
	if procGetDpiForWindow.Find() != nil {
		return 1
	}
	dpi, _, _ := procGetDpiForWindow.Call(hwnd)
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96
}

// winClientOrigin is where the window's client area starts on the desktop,
// in physical pixels.
func winClientOrigin(hwnd uintptr) (x, y float64) {
	var p struct{ x, y int32 }
	procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&p)))
	return float64(p.x), float64(p.y)
}

// winClientSize is how big that client area is, in physical pixels.
func winClientSize(hwnd uintptr) (w, h float64) {
	var r struct{ left, top, right, bottom int32 }
	procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	return float64(r.right - r.left), float64(r.bottom - r.top)
}
