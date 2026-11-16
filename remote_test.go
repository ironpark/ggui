//go:build ggwd_remote && unix && !android && !ios

package ggui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/platform/a11ybridge"
)

// An app ggui dev shows in its viewer takes the paths of a platform
// without a menu bar, a tray or native popups, keeps no accessibility
// bridge, and says why a second window does not open.
func TestRemoteDisplay(t *testing.T) {
	// ggwd/remote.EnvSocket, which ggui does not import: see package
	// remotedisplay.
	t.Setenv("GGWD_REMOTE_SOCKET", filepath.Join(t.TempDir(), "viewer.sock"))
	if NativeMenu() || NativeContextMenu() || TraySupported() {
		t.Errorf("NativeMenu, NativeContextMenu, TraySupported = %v, %v, %v in a remote app; want all false",
			NativeMenu(), NativeContextMenu(), TraySupported())
	}
	var b a11ybridge.Bridge
	b.Start(func(a11y.NodeID, a11y.Action) {}, a11y.Always)
	if b.Listening() {
		t.Error("the accessibility bridge listens in a remote app, which has no native window")
	}
	base := errors.New("ui: an app shown by a remote viewer has one window")
	err := errOneWindow(base)
	if !errors.Is(err, base) || !strings.Contains(err.Error(), "--display=window") {
		t.Errorf("errOneWindow = %v; want it to wrap the cause and name ggui dev --display=window", err)
	}
}
