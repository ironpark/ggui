package ggui

import (
	"runtime"
	"testing"
)

func TestMenuActionSkipsDisabled(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return sized(0, 0) })
	defer a.Close()
	enabled := State(false)
	runs := 0
	a.SetMenu(Menu("File",
		MenuAction("Save", "cmd+s", func() { runs++ }).BindEnabled(enabled),
		MenuSeparator(),
		Menu("Recent", MenuAction("a.txt", "", func() { runs += 10 })),
	))
	if len(a.actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(a.actions))
	}
	save, recent := a.menu.ids[0], a.menu.ids[1]
	a.runAction(save)
	if runs != 0 {
		t.Fatal("a disabled action ran")
	}
	enabled.Set(true)
	a.runAction(save)
	a.runAction(recent)
	a.runAction(recent + 100)
	if runs != 11 {
		t.Fatalf("runs = %d, want 11", runs)
	}
}

func TestSetMenuReplacesShortcuts(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return sized(0, 0) })
	defer a.Close()
	a.SetMenu(Menu("File", MenuAction("Save", "cmd+s", func() {}), MenuAction("Open", "", func() {})))
	a.addMenuShortcuts(a.Window)
	first := a.menuShortcuts
	if len(first) != 1 || first[0].Chord() != MustChord("cmd+s") {
		t.Fatalf("shortcuts = %v", first)
	}
	a.SetMenu(Menu("Edit", MenuAction("Undo", "cmd+z", func() {})))
	a.addMenuShortcuts(a.Window)
	if len(a.actions) != 1 {
		t.Fatalf("replaced menus left %d actions, want 1", len(a.actions))
	}
	if !first[0].removed || len(a.menuShortcuts) != 1 || a.menuShortcuts[0].Chord() != MustChord("cmd+z") {
		t.Fatalf("old shortcut kept or new one missing: %v", a.menuShortcuts)
	}
}

func TestKeyEquivalent(t *testing.T) {
	t.Parallel()
	for k, want := range map[KeyboardKey]string{
		KeyS: "s", KeyDigit1: "1", KeyF5: "", KeyEnter: "\r", KeyArrowUp: "", KeyComma: ",",
	} {
		if got := keyEquivalent(k); got != want {
			t.Errorf("keyEquivalent(%v) = %q, want %q", k, got, want)
		}
	}
}

func TestChordLabel(t *testing.T) {
	t.Parallel()
	c := MustChord("cmd+shift+s")
	want := "Ctrl+Shift+S"
	if runtime.GOOS == "darwin" {
		want = "⇧⌘S"
	}
	if got := c.Label(); got != want {
		t.Fatalf("Label = %q, want %q", got, want)
	}
}

func TestTrayNumbersItsActionsApart(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return sized(0, 0) })
	defer a.Close()
	a.SetMenu(Menu("File", MenuAction("Save", "cmd+s", func() {})))
	clicks := 0
	tray := a.AddTray(TrayConfig{Title: "t", OnClick: func() { clicks++ }})
	if len(tray.set.ids) != 1 || len(a.actions) != 2 {
		t.Fatalf("tray ids %v, actions %d", tray.set.ids, len(a.actions))
	}
	a.runAction(tray.set.ids[0])
	if clicks != 1 {
		t.Fatal("the tray's click did not run")
	}
	tray.SetMenu(MenuAction("A", "", func() {}), MenuAction("B", "", func() {}))
	if len(tray.set.ids) != 2 || len(a.actions) != 3 {
		t.Fatalf("after SetMenu: tray ids %v, actions %d", tray.set.ids, len(a.actions))
	}
	a.SetMenu()
	if len(a.actions) != 2 {
		t.Fatalf("replacing the menu bar touched the tray: %d actions", len(a.actions))
	}
	tray.Remove()
	tray.Remove()
	if len(a.actions) != 0 || len(a.trays) != 0 {
		t.Fatalf("removed tray left %d actions, %d trays", len(a.actions), len(a.trays))
	}
}

// editField is as much of a text field as the Edit menu reaches: a value
// Backspace shortens and ⌘Z puts back, and that chord, which it claims
// ahead of the menus and shortcuts while it has the focus.
type editField struct {
	Interactive
	value *StateValue[string]
	undo  []string
}

func (f *editField) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(200, 20)) }
func (f *editField) Paint(dst *Canvas, r Rect)        { f.Hit(dst, r, f, 0) }
func (f *editField) HandlePointer(PointerEvent) bool  { return true }

func (f *editField) HandleKey(ev KeyEvent) {
	f.Keyboard(ev, nil)
	if ev.Kind != KeyPress {
		return
	}
	switch v := Untrack(f.value.Get); {
	case ev.Key == KeyBackspace && v != "":
		f.undo = append(f.undo, v)
		f.value.Set(v[:len(v)-1])
	case f.ClaimsChord(ev) && len(f.undo) > 0:
		f.value.Set(f.undo[len(f.undo)-1])
		f.undo = f.undo[:len(f.undo)-1]
	}
}

func (f *editField) ClaimsChord(ev KeyEvent) bool {
	return ev.Kind == KeyPress && ev.Key == KeyZ && ev.Mods.Cmd()
}

func TestEditCommandsGoToTheFocusedFieldFirst(t *testing.T) {
	t.Parallel()
	value := State("hello")
	field := &editField{value: value}
	a := New(Config{}, func() Widget { return field })
	defer a.Close()
	undos := 0
	a.SetMenu(EditMenu(EditActions{Undo: func() { undos++ }}))
	undo := a.menu.ids[0]
	a.runAction(undo)
	if undos != 1 {
		t.Fatalf("with no field focused, Undo ran the app's action %d times, want once", undos)
	}
	paintFrame(&a.input, field, Sz(200, 20))
	a.input.dispatch(frameInput{pos: Pt(190, 10), down: []MouseButton{MouseButtonLeft}})
	a.input.dispatch(frameInput{pos: Pt(190, 10), up: []MouseButton{MouseButtonLeft}})
	a.input.dispatch(frameInput{keys: []KeyboardKey{KeyBackspace}})
	if Untrack(value.Get) != "hell" {
		t.Fatalf("value = %q after Backspace", Untrack(value.Get))
	}
	a.runAction(undo)
	if undos != 1 || Untrack(value.Get) != "hello" {
		t.Fatalf("Undo with the field focused: app ran %d times, value %q; want the field undone", undos, Untrack(value.Get))
	}
	// A shortcut on the same chord waits for the field too.
	shortcut := 0
	a.Shortcut("cmd+z", func() { shortcut++ })
	a.input.dispatch(frameInput{keys: []KeyboardKey{KeyZ}, mods: MustChord("cmd+z").held()})
	if shortcut != 0 {
		t.Fatal("a shortcut took ⌘Z from the focused field")
	}
	saves := 0
	a.Shortcut("cmd+s", func() { saves++ })
	a.input.dispatch(frameInput{keys: []KeyboardKey{KeyS}, mods: MustChord("cmd+s").held()})
	if saves != 1 {
		t.Fatal("the field kept a chord it does not edit with")
	}
}

func TestContextMenuWithoutNativeWindow(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return sized(0, 0) })
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
	// Standalone window with nil app must not panic.
	standalone := newWindow(nil, nil, Config{}, func() Widget { return sized(0, 0) })
	defer standalone.Close()
	if standalone.ShowContextMenu(Pt(1, 1), MenuAction("Run", "", func() {})) {
		t.Fatal("window with nil app opened a context menu")
	}
}

func TestEmptySubmenuIsNotActionAndMenuSetUsesAppRuntime(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return sized(0, 0) })
	defer a.Close()
	enabled := State(true)
	a.SetMenu(
		Menu("File",
			Menu("Recent"), // empty submenu must not allocate an action
			MenuAction("Save", "cmd+s", func() {}).BindEnabled(enabled),
		),
	)
	if len(a.actions) != 1 {
		t.Fatalf("actions = %d, want 1 (empty submenu must not be an action)", len(a.actions))
	}
	if len(a.menu.native) != 1 || len(a.menu.native[0].Items) != 2 || a.menu.native[0].Items[0].ID != 0 {
		t.Fatalf("unexpected native menu structure: %+v", a.menu.native)
	}
	if a.rt.Count() == 0 {
		t.Fatal("menu watchers were not registered in app runtime")
	}
}
