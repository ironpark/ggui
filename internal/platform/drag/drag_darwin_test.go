//go:build darwin && !ios

package drag

import (
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// The test binary has no GLFW window, so it stands in a content view class
// of the same name, sized like a window, and a sender that answers
// draggingLocation, and drives the added methods as AppKit would.
func TestInstallFollowsADragOnTheContentView(t *testing.T) {
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		t.Fatal(err)
	}
	if Install() {
		t.Fatal("Install succeeded before the content view class existed")
	}
	view, err := objc.RegisterClass("GLFWContentView", objc.GetClass("NSView"), nil, nil, []objc.MethodDef{
		{Cmd: selFrame, Fn: func(_ objc.ID, _ objc.SEL) nsRect { return nsRect{size: nsSize{width: 800, height: 600}} }},
	})
	if err != nil {
		t.Fatal(err)
	}
	sender, err := objc.RegisterClass("GgUIDragSender", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{
		{Cmd: selDraggingLocation, Fn: func(_ objc.ID, _ objc.SEL) nsPoint { return nsPoint{x: 100, y: 50} }},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !Install() || !Install() {
		t.Fatal("Install failed with the content view class present")
	}
	alloc, init := objc.RegisterName("alloc"), objc.RegisterName("init")
	v := objc.ID(view).Send(alloc).Send(init)
	s := objc.ID(sender).Send(alloc).Send(init)

	if _, _, over := Position(); over {
		t.Fatal("a drag was reported before one began")
	}
	if op := objc.Send[uintptr](v, selDraggingUpdated, s); op != nsDragOperationGeneric {
		t.Fatalf("draggingUpdated: answered %d, want generic", op)
	}
	x, y, over := Position()
	if !over || x != 100 || y != 550 {
		t.Fatalf("drag at (%v, %v, %v), want (100, 550, true): y is flipped from the bottom-left origin", x, y, over)
	}
	v.Send(selDraggingExited, s)
	if _, _, over := Position(); over {
		t.Fatal("draggingExited: left the drag reported")
	}
	v.Send(selDraggingUpdated, s)
	v.Send(selDraggingEnded, s)
	if _, _, over := Position(); over {
		t.Fatal("draggingEnded: left the drag reported")
	}
}
