package ggui

import "testing"

func TestHideToTrayBringsBackTheWindowsItHid(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return sized(0, 0) })
	defer a.Close()
	shown, err := a.OpenWindow(Config{Title: "Shown"}, func() Widget { return sized(0, 0) })
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := a.OpenWindow(Config{Title: "Hidden", Hidden: true}, func() Widget { return sized(0, 0) })
	if err != nil {
		t.Fatal(err)
	}
	a.HideToTray()
	a.HideToTray() // a second call forgets nothing the first hid
	for _, w := range []*Window{a.Window, shown, hidden} {
		if w.Visible() {
			t.Fatalf("%q is shown after HideToTray", w.cfg.Title)
		}
	}
	a.ShowFromTray()
	if !a.Window.Visible() || !shown.Visible() {
		t.Fatal("ShowFromTray left a window it hid hidden")
	}
	if hidden.Visible() {
		t.Fatal("ShowFromTray showed a window that was hidden before")
	}

	// With nothing it hid, ShowFromTray shows the window that would have
	// the focus.
	a.Window.Hide()
	shown.Hide()
	a.ShowFromTray()
	if !a.Window.Visible() {
		t.Fatal("ShowFromTray with nothing hid left the main window hidden")
	}
}
