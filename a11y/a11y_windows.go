//go:build windows

package a11y

import (
	"sync/atomic"

	"github.com/ironpark/ggfx"
)

// The Windows half of the accessibility bridge speaks UI Automation, from
// the provider side: every node in the published tree becomes a provider
// object, and the window answers WM_GETOBJECT with the one that stands for
// itself. UI Automation bridges the result to the older MSAA clients on its
// own, so this is the only interface that has to exist.
//
// Everything is done through the DLLs directly, without cgo, the way the
// macOS half goes through the Objective-C runtime: the project builds
// without a C toolchain and stays that way.
//
// Nothing in this file may call into Ebitengine except where it says it is
// on the main thread. UI Automation calls a provider on its own threads,
// and the answers all come from the published frame, which is built to be
// read from anywhere.

func init() { buildVtables() }

type windowsAX struct {
	bridge     *Bridge
	window     *ggfx.Window
	root, hwnd atomic.Uintptr
}

func newAXPlatform() axPlatform { return &windowsAX{} }

func (d *windowsAX) setWindow(b *Bridge, w *ggfx.Window) {
	d.bridge, d.window = b, w
	h := w.NativeHandle()
	if h == 0 {
		return
	}
	root := newWinObj(b, winRootHandle)
	if root == 0 {
		return
	}
	d.root.Store(root)
	d.hwnd.Store(h)
	w.SetGetObjectHandler(func(wparam, lparam uintptr) (uintptr, bool) {
		if int32(uint32(lparam)) != uiaRootObjectID {
			return 0, false
		}
		root := d.root.Load()
		if root == 0 {
			return 0, false
		}
		r, _, _ := procUiaReturnRawElementProvider.Call(h, wparam, lparam, root)
		return r, true
	})
}

func (d *windowsAX) active() bool {
	if d.hwnd.Load() == 0 {
		return false
	}
	on, _, _ := procUiaClientsAreListening.Call()
	return on != 0
}

func (d *windowsAX) close() {
	if d.window != nil {
		d.window.SetGetObjectHandler(nil)
	}
	h := d.hwnd.Swap(0)
	root := d.root.Swap(0)
	if h != 0 {
		ggfx.RunOnMainThread(func() { procUiaReturnRawElementProvider.Call(h, 0, 0, 0) })
	}
	if root != 0 {
		winObjectBridges.Delete(root)
		procUiaDisconnectProvider.Call(root)
		comRelease(root)
	}
	d.window = nil
}

func winHandle(b *Bridge) uintptr {
	if b != nil {
		if p, ok := b.plat.(*windowsAX); ok {
			return p.hwnd.Load()
		}
	}
	return 0
}
func winRootFor(b *Bridge) uintptr {
	if b != nil {
		if p, ok := b.plat.(*windowsAX); ok {
			return p.root.Load()
		}
	}
	return 0
}

// element makes a provider object for one node, carrying its handle. The
// bridge's cache holds the reference this returns; a copy handed to UI
// Automation gets one of its own.
func (d *windowsAX) element(handle int64) uintptr { return newWinObj(d.bridge, handle) }

// release tells UI Automation that these elements answer nothing more and
// then drops the cache's reference. Disconnecting first is what keeps a
// client from holding a provider the bridge has finished with, which is a
// documented way to leak a whole tree into a screen reader.
func (windowsAX) release(elems []uintptr) {
	for _, e := range elems {
		winObjectBridges.Delete(e)
		procUiaDisconnectProvider.Call(e)
		comRelease(e)
	}
}

// notify delivers a frame's worth of events. It hops to the main thread
// once for the whole batch, the way the macOS half does: raising an event
// can call straight back into a provider, and doing that from the thread
// that owns the window is the arrangement every client expects.
//
// publish holds no lock across this call, so a re-entrant query that wants
// the element cache gets it.
func (d *windowsAX) notify(notes []axNote) {
	b := d.bridge
	if b == nil || winHandle(b) == 0 {
		return
	}
	ggfx.RunOnMainThread(func() {
		for _, n := range notes {
			winPost(b, n)
		}
	})
}

// winPost raises the one event a note stands for. An element that has left
// the tree between the diff and here simply says nothing.
func winPost(b *Bridge, n axNote) {
	root := winRootFor(b)
	if root == 0 {
		return
	}
	if n.kind == axAnnouncement {
		// The notification event arrived in Windows 10 1709. Calling an
		// export that is not there panics, so an older Windows gets a
		// live region change instead: less precise, but it is said.
		if procUiaRaiseNotificationEvent.Find() != nil {
			procUiaRaiseAutomationEvent.Call(root, uiaLiveRegionChangedEvent)
			return
		}
		proc := uintptr(uiaNotificationProcessingAll)
		if n.loud {
			proc = uiaNotificationProcessingImportantAll
		}
		text := winStr(n.text)
		empty := winStr("")
		procUiaRaiseNotificationEvent.Call(root, uiaNotificationKindOther, proc, text, empty)
		// Raising an event does not take the strings: it reads them and
		// returns, and what is not freed here is never freed at all.
		procSysFreeString.Call(text)
		procSysFreeString.Call(empty)
		return
	}
	if n.kind == axLayoutChanged {
		procUiaRaiseStructureChanged.Call(root, uiaStructureChildrenInvalidated, 0, 0)
		return
	}
	el := root
	if !n.root {
		el = b.element(n.key)
	}
	if el == 0 {
		return
	}
	switch n.kind {
	case axFocusChanged:
		procUiaRaiseAutomationEvent.Call(el, uiaAutomationFocusChangedEvent)
	case axSelectionChanged:
		procUiaRaiseAutomationEvent.Call(el, uiaSelectionItemElementSelectedEvent)
	case axValueChanged:
		winPostValue(b, el, n.key)
	}
}

// uiaSelectionItemElementSelectedEvent is what a tab, option or row raises
// when it becomes the chosen one.
const uiaSelectionItemElementSelectedEvent = 20012

// winPostValue raises the property-changed event for whichever property a
// node's value actually lives in, carrying the new value rather than an
// empty variant: a client that is told only that something changed has to
// go and ask, and several do not bother.
func winPostValue(b *Bridge, el uintptr, k axKey) {
	f := b.frame()
	if f == nil {
		return
	}
	n, ok := f.at(k)
	if !ok {
		return
	}
	var old, fresh winVariant
	prop := uintptr(uiaNameProperty)
	switch {
	case winPattern(n.Node, ifToggle):
		prop = uiaToggleToggleStateProperty
		varI4(&fresh, winToggleOf(n.Node))
	case winPattern(n.Node, ifRangeValue):
		prop = uiaRangeValueValueProperty
		varR8(&fresh, n.Now)
	case winPattern(n.Node, ifValue):
		prop = uiaValueValueProperty
		varStr(&fresh, n.Value)
	default:
		varStr(&fresh, n.Name)
	}
	winRaisePropertyChanged(el, prop, &old, &fresh)
	// The variant is this function's, not UIA's, so its string is this
	// function's to free. A slider raises one of these every frame it
	// moves, which is how a leak here becomes a real one.
	if fresh.vt == vtBSTR && fresh.val[0] != 0 {
		procSysFreeString.Call(fresh.val[0])
	}
}

// uiaLiveRegionChangedEvent is the older way of saying that something
// worth announcing changed, for a Windows without notification events.
const uiaLiveRegionChangedEvent = 20024

// winToggleOf is the ToggleState a checkable node reports.
func winToggleOf(n Node) int32 {
	switch n.Checked {
	case TriOn:
		return uiaToggleOn
	case TriMixed:
		return uiaToggleIndeterminate
	}
	return uiaToggleOff
}

// winExpandStateOf is the ExpandCollapseState an expandable node reports,
// and a leaf for one that is not.
func winExpandStateOf(n Node) int32 {
	switch {
	case n.Expanded == nil:
		return uiaLeafNode
	case *n.Expanded:
		return uiaExpanded
	}
	return uiaCollapsed
}
