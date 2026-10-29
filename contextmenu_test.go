package ggui

import "testing"

func TestContextMenuWithoutNativeWindow(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return Box() })
	defer a.Close()
	if a.ShowContextMenu(Pt(10, 20), MenuAction("Run", "", func() { t.Fatal("action ran") })) {
		t.Fatal("opened a native menu before the window exists")
	}
	if len(a.actions) != 0 {
		t.Fatal("unopened context menu leaked actions")
	}
	var w *Window
	if w.ShowContextMenu(Point{}) {
		t.Fatal("nil window opened a menu")
	}
}
