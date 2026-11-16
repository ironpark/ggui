//go:build darwin && !ios

package runtime

import (
	"github.com/ebitengine/purego/objc"

	"github.com/ironpark/ggui/internal/platform/cocoa"
	"github.com/ironpark/ggui/internal/platform/remotedisplay"
)

// An app ggui dev shows in its viewer has no window of its own and is not
// the active application: the viewer is. Its dialogs are still its own,
// standing alone, since there is no window of the app's to attach them
// to; for one to come to the front and take the keyboard, the process
// turns into an accessory application, which has neither a Dock icon nor
// a menu bar, and activates while the dialog is up. The application that
// was active before, the viewer when the user worked in its window, is
// activated again after.

var (
	selSetActivationPolicy       = objc.RegisterName("setActivationPolicy:")
	selActivateIgnoringOtherApps = objc.RegisterName("activateIgnoringOtherApps:")
	selSharedWorkspace           = objc.RegisterName("sharedWorkspace")
	selFrontmostApplication      = objc.RegisterName("frontmostApplication")
	selActivateWithOptions       = objc.RegisterName("activateWithOptions:")
	selRetain                    = objc.RegisterName("retain")

	classNSWorkspace = objc.ID(objc.GetClass("NSWorkspace"))
)

// nsApplicationActivationPolicyAccessory is NSApplicationActivationPolicyAccessory.
const nsApplicationActivationPolicyAccessory = 1

// inFront runs fn, which shows a modal dialog, with the app in front when
// a viewer shows its window. It must run on the main thread.
func inFront(fn func()) {
	if !remotedisplay.On() {
		fn()
		return
	}
	app := cocoa.App()
	app.Send(selSetActivationPolicy, nsApplicationActivationPolicyAccessory)
	prev := classNSWorkspace.Send(selSharedWorkspace).Send(selFrontmostApplication)
	if prev != 0 {
		prev.Send(selRetain)
		defer prev.Send(selRelease)
		defer prev.Send(selActivateWithOptions, uint(0))
	}
	app.Send(selActivateIgnoringOtherApps, true)
	fn()
}
