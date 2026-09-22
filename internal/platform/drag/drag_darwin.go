//go:build darwin && !ios

// Package drag reports files being dragged over the window, before they
// are dropped. Ebitengine reports only the drop, so this listens on the
// platform's own drag session where it can.
package drag

import (
	"structs"
	"sync"

	"github.com/ebitengine/purego/objc"
)

// GLFW's content view registers for dragged file URLs and implements
// draggingEntered: and performDragOperation:, and nothing between them.
// AppKit sends draggingUpdated: to a destination that implements it as the
// cursor moves, and draggingExited: and draggingEnded: when the drag leaves
// or finishes, so adding those three methods to the class is enough to
// follow the drag without touching how GLFW accepts the drop.

var (
	selDraggingLocation = objc.RegisterName("draggingLocation")
	selFrame            = objc.RegisterName("frame")
	selDraggingUpdated  = objc.RegisterName("draggingUpdated:")
	selDraggingExited   = objc.RegisterName("draggingExited:")
	selDraggingEnded    = objc.RegisterName("draggingEnded:")
)

// nsDragOperationGeneric is what GLFW's draggingEntered: answers too.
const nsDragOperationGeneric = 4

type nsPoint struct {
	_    structs.HostLayout
	x, y float64
}

type nsSize struct {
	_             structs.HostLayout
	width, height float64
}

type nsRect struct {
	_      structs.HostLayout
	origin nsPoint
	size   nsSize
}

var state struct {
	mu        sync.Mutex
	installed bool
	over      bool
	x, y      float64
}

// Install adds the drag observers to the window's content view class, once.
// It must run on the main thread, and reports false until the window exists.
func Install() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.installed {
		return true
	}
	class := objc.GetClass("GLFWContentView")
	if class == 0 {
		return false
	}
	// AddMethod refuses a selector the class already implements. GLFW
	// implements none of these today; should it grow one, that part of the
	// drag goes unreported rather than breaking the drop.
	class.AddMethod(selDraggingUpdated, objc.NewIMP(draggingUpdated), "Q@:@")
	class.AddMethod(selDraggingExited, objc.NewIMP(draggingEnded), "v@:@")
	class.AddMethod(selDraggingEnded, objc.NewIMP(draggingEnded), "v@:@")
	state.installed = true
	return true
}

// Position is where files are being dragged over the window, in the view's
// points with the origin at the top left, as the cursor is reported.
func Position() (x, y float64, over bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.x, state.y, state.over
}

func draggingUpdated(self objc.ID, _ objc.SEL, sender objc.ID) uintptr {
	pos := objc.Send[nsPoint](sender, selDraggingLocation)
	frame := objc.Send[nsRect](self, selFrame)
	state.mu.Lock()
	state.over, state.x, state.y = true, pos.x, frame.size.height-pos.y
	state.mu.Unlock()
	return nsDragOperationGeneric
}

func draggingEnded(_ objc.ID, _ objc.SEL, _ objc.ID) {
	state.mu.Lock()
	state.over = false
	state.mu.Unlock()
}
