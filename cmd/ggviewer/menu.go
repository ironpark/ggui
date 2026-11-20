//go:build (unix && !ios && !android) || windows

package main

import (
	"github.com/ironpark/ggui/internal/platform/nativemenu"
	"github.com/ironpark/ggwd/remote"
)

// menuBar is what of the platform's menu bar and system tray the viewer drives
// for the attached app. Replaced in tests so menu and tray syncing can be
// verified without touching AppKit's global NSMenu / NSStatusBar or Win32
// shell32 tray icons.
type menuBar interface {
	Set(menus []nativemenu.Item)
	SetEnabled(id int, on bool)
	SetChecked(id int, on bool)
	SetTray(key int, t nativemenu.Tray)
	RemoveTray(key int)
}

type nativeMenuBar struct{}

func (nativeMenuBar) Set(menus []nativemenu.Item)        { nativemenu.Set(menus) }
func (nativeMenuBar) SetEnabled(id int, on bool)         { nativemenu.SetEnabled(id, on) }
func (nativeMenuBar) SetChecked(id int, on bool)         { nativemenu.SetChecked(id, on) }
func (nativeMenuBar) SetTray(key int, t nativemenu.Tray) { nativemenu.SetTray(key, t) }
func (nativeMenuBar) RemoveTray(key int)                 { nativemenu.RemoveTray(key) }

// remoteToNativeTray converts the remote protocol's tray update into the
// platform nativemenu tray description.
func remoteToNativeTray(m *remote.SetTray) nativemenu.Tray {
	return nativemenu.Tray{
		Icon:     m.Icon,
		Template: m.Template,
		Title:    m.Title,
		Tooltip:  m.Tooltip,
		Menu:     remoteToNativeMenu(m.Menu),
		ClickID:  m.ClickID,
	}
}

// remoteToNativeMenu converts the remote protocol's menu tree into the
// platform nativemenu tree.
func remoteToNativeMenu(items []remote.MenuItem) []nativemenu.Item {
	if len(items) == 0 {
		return nil
	}
	out := make([]nativemenu.Item, len(items))
	for i, it := range items {
		out[i] = nativemenu.Item{
			Title:     it.Title,
			Separator: it.Separator,
			Items:     remoteToNativeMenu(it.Items),
			ID:        it.ID,
			Key:       it.Key,
			Cmd:       it.Cmd,
			Shift:     it.Shift,
			Alt:       it.Alt,
			Ctrl:      it.Ctrl,
			Enabled:   it.Enabled,
			Checked:   it.Checked,
		}
	}
	return out
}
