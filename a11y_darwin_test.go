//go:build darwin && !ios

package ggui

import (
	"runtime"
	"testing"

	"github.com/ebitengine/purego/cstrings"
	"github.com/ebitengine/purego/objc"
)

// Exercise the actual Objective-C value returned to AppKit. An empty editor
// must expose AXValue so clients can discover it and write the first text.
func TestAXEmptyEditorValue(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(axSelAlloc).Send(axSelInit)
	defer pool.Send(objc.RegisterName("drain"))

	b := &axBridge{plat: &darwinAX{}}
	previous := theAX.Swap(b)
	defer theAX.Store(previous)

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
		tree := axRoots(SemNode{Node: Node{Role: tc.role, Value: tc.value, Actions: ActionSetValue}})
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
