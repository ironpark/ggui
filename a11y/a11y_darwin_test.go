//go:build darwin && !ios

package a11y

import (
	"runtime"
	"testing"

	"github.com/ebitengine/purego/cstrings"
	"github.com/ebitengine/purego/objc"
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
		tree := axRoots(SemNode{Role: RoleTextField, Value: value})
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
		role    Role
		value   string
		wantNil bool
	}{
		{"empty editor", RoleTextField, "", false},
		{"populated editor", RoleTextField, "hello", false},
		{"button without value", RoleButton, "", true},
	} {
		tree := axRoots(SemNode{Role: tc.role, Value: tc.value, Actions: ActionSetValue})
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
	n := axNode(RoleButton, "b", nil, Rct(Pt(10, 20), Sz(30, 40)))
	n.Rect, n.Offscreen = Rect{}, true
	x, y, w, h := axBounds(n, 600)
	if x != 10 || y != 600-60 || w != 30 || h != 40 {
		t.Errorf("bounds = %g,%g %gx%g; want 10,540 30x40", x, y, w, h)
	}
}

func TestAXRolesCoverEveryRole(t *testing.T) {
	all := []Role{
		RoleButton, RoleCheckbox, RoleRadio, RoleSwitch, RoleSlider, RoleTextField,
		RoleSelect, RoleOption, RoleMenu, RoleMenuItem, RoleTab, RoleTabs,
		RoleDisclosure, RoleDialog, RoleRow, RoleAccordion, RoleCombobox,
		RoleSeparator, RoleText, RoleHeading, RoleImage, RoleList, RoleListItem,
		RoleGroup, RoleProgress, RoleLink, RoleToolbar, RoleStatus, RoleWindow,
	}
	for _, r := range all {
		role, _ := axRole(r)
		if role == "AXUnknown" {
			t.Errorf("%s maps to no AppKit role", r)
		}
	}
	if role, sub := axRole(RoleSwitch); role != "AXCheckBox" || sub != "AXSwitch" {
		t.Errorf("switch = %s/%s, want AXCheckBox/AXSwitch", role, sub)
	}
	if role, sub := axRole(RoleTab); role != "AXRadioButton" || sub != "AXTabButton" {
		t.Errorf("tab = %s/%s, want AXRadioButton/AXTabButton", role, sub)
	}
	if role, sub := axRole(RoleDialog); role != "AXWindow" || sub != "AXDialog" {
		t.Errorf("dialog = %s/%s, want AXWindow/AXDialog", role, sub)
	}
	if role, _ := axRole(Role("nonsense")); role != "AXUnknown" {
		t.Errorf("an unknown role mapped to %s", role)
	}
}
