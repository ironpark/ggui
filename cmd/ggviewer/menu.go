//go:build (unix && !ios && !android) || windows

package main

import (
	"github.com/ironpark/ggui/internal/platform/nativemenu"
	"github.com/ironpark/ggwd/remote"
)

// menuBar is what of the platform's menu bar the viewer drives for the
// attached app. Replaced in tests so menu syncing can be verified without
// touching AppKit's global NSMenu.
type menuBar interface {
	Set(menus []nativemenu.Item)
	SetEnabled(id int, on bool)
	SetChecked(id int, on bool)
}

type nativeMenuBar struct{}

func (nativeMenuBar) Set(menus []nativemenu.Item) { nativemenu.Set(menus) }
func (nativeMenuBar) SetEnabled(id int, on bool)  { nativemenu.SetEnabled(id, on) }
func (nativeMenuBar) SetChecked(id int, on bool)  { nativemenu.SetChecked(id, on) }

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
