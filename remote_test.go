//go:build ggwd_remote && ((unix && !android && !ios) || windows)

package ggui

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/platform/a11ybridge"
)

// An app ggui dev shows in its viewer supports the native menu bar on macOS
// (forwarded to the viewer), takes the paths of a platform without a tray or
// native popups, keeps no accessibility bridge, and says why a second window
// does not open.
func TestRemoteDisplay(t *testing.T) {
	// ggwd/remote.EnvSocket, which ggui does not import: see package
	// remotedisplay.
	t.Setenv("GGWD_REMOTE_SOCKET", filepath.Join(t.TempDir(), "viewer.sock"))
	wantNativeMenu := runtime.GOOS == "darwin"
	if NativeMenu() != wantNativeMenu {
		t.Errorf("NativeMenu() = %v in a remote app on %s; want %v", NativeMenu(), runtime.GOOS, wantNativeMenu)
	}
	if NativeContextMenu() || TraySupported() {
		t.Errorf("NativeContextMenu, TraySupported = %v, %v in a remote app; want both false",
			NativeContextMenu(), TraySupported())
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

func TestRemoteMenuAndWindowConfig(t *testing.T) {
	t.Setenv("GGWD_REMOTE_SOCKET", filepath.Join(t.TempDir(), "viewer.sock"))
	pos := Pt(100, 50)
	tl := Pt(16, 14)
	cfg := Config{
		Title:          "Custom",
		Width:          900,
		Height:         600,
		MinWidth:       400,
		MinHeight:      300,
		MaxWidth:       1600,
		MaxHeight:      1200,
		Position:       &pos,
		Resizable:      true,
		Frameless:      false,
		TitlebarHidden: true,
		TrafficLights:  &tl,
		AlwaysOnTop:    true,
		Transparent:    true,
		Hidden:         true,
		Maximized:      true,
		Unfocused:      true,
	}
	o := cfg.options()
	if !o.TitlebarHidden || o.TrafficLights == nil || o.TrafficLights.X != 16 || o.TrafficLights.Y != 14 {
		t.Fatalf("titlebar options lost: hidden=%v tl=%+v", o.TitlebarHidden, o.TrafficLights)
	}
	if o.Position == nil || o.Position.X != 100 || o.Position.Y != 50 {
		t.Fatalf("position lost: %+v", o.Position)
	}
	if !o.Resizable || o.Undecorated || !o.Floating || !o.Transparent || !o.Hidden || !o.Maximized || !o.Unfocused {
		t.Fatalf("window flags lost: %+v", o)
	}

	a := New(cfg, func() Widget { return sized(0, 0) })
	defer a.Close()
	enabled := State(false)
	checked := State(true)
	ran := 0
	a.SetMenu(Menu("File",
		MenuAction("Save", "cmd+s", func() { ran++ }).BindEnabled(enabled),
		MenuSeparator(),
		MenuAction("Dark", "", func() { ran += 10 }).BindChecked(checked),
	))
	if runtime.GOOS == "darwin" {
		if len(a.menuShortcuts) != 0 {
			t.Fatalf("remote macOS app installed %d duplicate in-window shortcuts, want 0", len(a.menuShortcuts))
		}
	} else if len(a.menuShortcuts) != 1 {
		t.Fatalf("remote %s app installed %d in-window shortcuts, want 1", runtime.GOOS, len(a.menuShortcuts))
	}
	if len(a.menu.ids) != 2 {
		t.Fatalf("menu ids = %v, want 2", a.menu.ids)
	}
	saveID, darkID := a.menu.ids[0], a.menu.ids[1]
	a.runAction(saveID)
	if ran != 0 {
		t.Fatalf("disabled remote menu item ran")
	}
	enabled.Set(true)
	a.runAction(saveID)
	a.runAction(darkID)
	if ran != 11 {
		t.Fatalf("ran = %d, want 11", ran)
	}
}
