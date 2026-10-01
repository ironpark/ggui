package ggui

import "testing"

func TestMenuActionSkipsDisabled(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return Box() })
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
	a := New(Config{}, func() Widget { return Box() })
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
	if runtimeIsDarwin() {
		want = "⇧⌘S"
	}
	if got := c.Label(); got != want {
		t.Fatalf("Label = %q, want %q", got, want)
	}
}

func TestTrayNumbersItsActionsApart(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return Box() })
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

func TestEditCommandsGoToTheFocusedFieldFirst(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value := State("hello")
	field := TextInput(value)
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
	typeKeys(&a.input, Mods{}, KeyBackspace)
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
	typeKeys(&a.input, MustChord("cmd+z").held(), KeyZ)
	if shortcut != 0 {
		t.Fatal("a shortcut took ⌘Z from the focused field")
	}
	saves := 0
	a.Shortcut("cmd+s", func() { saves++ })
	typeKeys(&a.input, MustChord("cmd+s").held(), KeyS)
	if saves != 1 {
		t.Fatal("the field kept a chord it does not edit with")
	}
}
