package ggui

import (
	"fmt"
	"os"
	"sync"

	"github.com/ironpark/ggui/internal/platform/remotedisplay"
)

// An app that ggui dev shows in its viewer, the default of ggui dev on
// macOS, has one window (the viewer's) and forwards its native menu bar
// to the viewer, but has no tray or native popup of its own: see
// ggwd.Remote. NativeContextMenu and TraySupported report false there, so
// that the app takes the paths it takes on a platform without them, and
// what still cannot be done is said once on stderr, where ggui dev shows
// the app's output.

// remoteNotices holds the notices already given.
var remoteNotices sync.Map

// remoteNotice says, once per what, that what does not happen while ggui
// dev's viewer shows the app. It does nothing in any other app.
func remoteNotice(what string) {
	if !remotedisplay.On() {
		return
	}
	if _, dup := remoteNotices.LoadOrStore(what, true); !dup {
		fmt.Fprintf(os.Stderr, "ggui: %s while ggui dev's viewer shows the app; ggui dev --display=window or a release build does\n", what)
	}
}

// errOneWindow wraps the error of a window the viewer cannot show.
func errOneWindow(err error) error {
	if !remotedisplay.On() {
		return err
	}
	return fmt.Errorf("ggui: ggui dev's viewer shows one window; ggui dev --display=window opens more: %w", err)
}
