//go:build darwin && !ios

package a11ybridge

import (
	"runtime"
	"testing"

	"github.com/ebitengine/purego/cstrings"
	"github.com/ebitengine/purego/objc"
	"github.com/ironpark/ggui/a11y"
)

func TestAXElementsResolveTheirOwningBridge(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(axSelAlloc).Send(axSelInit)
	defer pool.Send(objc.RegisterName("drain"))
	var bridges [2]*Bridge
	var elements [2]objc.ID
	for i, value := range []string{"first window", "second window"} {
		d := &darwinAX{}
		b := &Bridge{plat: d}
		d.bridge = b
		bridges[i] = b
		tree := axRoots(a11y.SemNode{Role: a11y.RoleTextField, Value: value})
		b.cur.Store(newAXFrame(tree))
		elements[i] = objc.ID(b.element(axKeyOf(tree.At(0).ID)))
		defer b.clear()
	}
	for i, want := range []string{"first window", "second window"} {
		got := cstrings.NSStringToString(elements[i].Send(objc.RegisterName("accessibilityValue")))
		if got != want {
			t.Fatalf("window %d value = %q, want %q", i, got, want)
		}
	}
	bridges[0].clear()
	if got := cstrings.NSStringToString(elements[1].Send(objc.RegisterName("accessibilityValue"))); got != "second window" {
		t.Fatalf("closing first bridge changed second: %q", got)
	}
}

// Exercise the actual Objective-C value returned to AppKit. An empty editor
// must expose AXValue so clients can discover it and write the first text.
func TestAXEmptyEditorValue(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(axSelAlloc).Send(axSelInit)
	defer pool.Send(objc.RegisterName("drain"))

	b := &Bridge{plat: &darwinAX{}}
	b.plat.(*darwinAX).bridge = b

	for _, tc := range []struct {
		name    string
		role    a11y.Role
		value   string
		wantNil bool
	}{
		{"empty editor", a11y.RoleTextField, "", false},
		{"populated editor", a11y.RoleTextField, "hello", false},
		{"button without value", a11y.RoleButton, "", true},
	} {
		tree := axRoots(a11y.SemNode{Role: tc.role, Value: tc.value, Actions: a11y.ActionSetValue})
		b.cur.Store(newAXFrame(tree))
		e := objc.ID(b.element(axKeyOf(tree.At(0).ID)))
		value := e.Send(objc.RegisterName("accessibilityValue"))
		if (value == 0) != tc.wantNil {
			t.Fatalf("%s: nil AXValue = %v, want %v", tc.name, value == 0, tc.wantNil)
		}
		if value != 0 && cstrings.NSStringToString(value) != tc.value {
			t.Fatalf("AXValue = %q, want %q", cstrings.NSStringToString(value), tc.value)
		}
	}
	b.clear()
}

func TestAXBoundsFlipTheYAxis(t *testing.T) {
	// ggui measures down from the top of the window and Cocoa measures up
	// from the bottom, and the bounds reported are the ones the node
	// painted at, not the ones it was clipped to.
	n := axNode(a11y.RoleButton, "b", nil, a11y.Rct(a11y.Pt(10, 20), a11y.Sz(30, 40)))
	n.Rect, n.Offscreen = a11y.Rect{}, true
	x, y, w, h := axBounds(n, 600)
	if x != 10 || y != 600-60 || w != 30 || h != 40 {
		t.Errorf("bounds = %g,%g %gx%g; want 10,540 30x40", x, y, w, h)
	}
}

func TestAXRolesCoverEveryRole(t *testing.T) {
	all := []a11y.Role{
		a11y.RoleButton, a11y.RoleCheckbox, a11y.RoleRadio, a11y.RoleSwitch, a11y.RoleSlider, a11y.RoleTextField,
		a11y.RoleSelect, a11y.RoleOption, a11y.RoleMenu, a11y.RoleMenuItem, a11y.RoleTab, a11y.RoleTabs,
		a11y.RoleDisclosure, a11y.RoleDialog, a11y.RoleRow, a11y.RoleAccordion, a11y.RoleCombobox,
		a11y.RoleSeparator, a11y.RoleText, a11y.RoleHeading, a11y.RoleImage, a11y.RoleList, a11y.RoleListItem,
		a11y.RoleGroup, a11y.RoleProgress, a11y.RoleLink, a11y.RoleToolbar, a11y.RoleStatus, a11y.RoleWindow,
	}
	for _, r := range all {
		role, _ := axRole(r)
		if role == "AXUnknown" {
			t.Errorf("%s maps to no AppKit role", r)
		}
	}
	if role, sub := axRole(a11y.RoleSwitch); role != "AXCheckBox" || sub != "AXSwitch" {
		t.Errorf("switch = %s/%s, want AXCheckBox/AXSwitch", role, sub)
	}
	if role, sub := axRole(a11y.RoleTab); role != "AXRadioButton" || sub != "AXTabButton" {
		t.Errorf("tab = %s/%s, want AXRadioButton/AXTabButton", role, sub)
	}
	if role, sub := axRole(a11y.RoleDialog); role != "AXWindow" || sub != "AXDialog" {
		t.Errorf("dialog = %s/%s, want AXWindow/AXDialog", role, sub)
	}
	if role, _ := axRole(a11y.Role("nonsense")); role != "AXUnknown" {
		t.Errorf("an unknown role mapped to %s", role)
	}
}
