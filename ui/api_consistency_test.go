package ui_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

type namedControl interface {
	ggui.Widget
	ggui.Semantic
	ui.Named
}

func TestFieldNamePriority(t *testing.T) {
	cases := []struct {
		name string
		make func() namedControl
	}{
		{"input OTP", func() namedControl { return ui.InputOTP(ggui.State(""), 6) }},
		{"editor", func() namedControl { return ggui.TextInput(ggui.State("")).Placeholder("Hint") }},
		{"textfield", func() namedControl { return ui.TextField(ggui.State("")).Placeholder("Hint") }},
		{"combobox", func() namedControl { return ui.Combobox(ggui.State(1)).Options([]int{1, 2}) }},
		{"datepicker", func() namedControl { return ui.DatePicker(ggui.State(time.Time{})) }},
		{"menu", func() namedControl { return ui.MenuOf(ggui.Text("Icon"), ui.MenuItem("Open", nil)) }},
		{"radios", func() namedControl { return ui.Radios(ggui.State(1)).Options([]int{1, 2}) }},
		{"toggle group", func() namedControl { return ui.ToggleGroup(ggui.State(1)).Options([]int{1, 2}) }},
		{"calendar", func() namedControl { return ui.Calendar(ggui.State(time.Time{})) }},
		{"accordion", func() namedControl {
			return ui.Accordion(ggui.State([]string{}), ui.AccordionItem("a", "A", ggui.Text("Body")))
		}},
		{"input group", func() namedControl { return ui.InputGroup(ggui.TextInput(ggui.State("")).Placeholder("Hint")) }},
	}
	for _, tc := range cases {
		for _, explicit := range []bool{false, true} {
			t.Run(tc.name+strconv.FormatBool(explicit), func(t *testing.T) {
				control := tc.make()
				if control.HasName() {
					t.Fatal("fallback counted as explicit name")
				}
				want := "Field name"
				if explicit {
					control.SetName("Custom")
					want = "Custom"
				}
				field := ui.Field("Field name", control)
				role, name := control.Semantics()
				if name != want || !control.HasName() {
					t.Fatalf("name=%q, want %q", name, want)
				}
				p := ggui.NewProbe(field, ggui.Sz(600, 700))
				defer p.Close()
				node(t, p.Semantics(), role, want)
				if role == ggui.RoleTextField || role == ggui.RoleCombobox || role == ggui.RoleMenu {
					if _, ok := p.Find(want); !ok {
						t.Fatalf("Probe missing %q", want)
					}
				}
			})
		}
	}
	// User-provided visible labels count as explicit names.
	b := ui.Button("Visible", nil)
	ui.Field("Outer", b)
	if _, name := b.Semantics(); name != "Visible" {
		t.Fatal(name)
	}
}

func TestNamingDoesNotChangeOptionFormatting(t *testing.T) {
	selected := ggui.State(2)
	s := ui.Select(selected).Options([]int{1, 2}).Format(func(n int) string { return "Option " + strconv.Itoa(n) }).Name("Choice")
	p := ggui.NewProbe(ui.Field("Outer", s), ggui.Sz(300, 300))
	defer p.Close()
	if n := node(t, p.Semantics(), ggui.RoleSelect, "Choice"); n.Value != "Option 2" {
		t.Fatal(n)
	}
	s.Popup().Show()
	node(t, p.Semantics(), ggui.RoleOption, "Option 1")
}

// Exercise the public fluent setters, including their last-setting-wins contract,
// on each implementation instead of merely checking that the methods exist.
func checkDisabled[W interface {
	ggui.Widget
	Disabled(bool) W
	BindDisabled(ggui.Readable[bool]) W
}](t *testing.T, w W, role ggui.Role, name string) {
	t.Helper()
	busy := ggui.State(false)
	w.BindDisabled(busy)
	p := ggui.NewProbe(w, ggui.Sz(500, 500))
	defer p.Close()
	check := func(want bool) {
		t.Helper()
		if n := node(t, p.Semantics(), role, name); n.Disabled != want {
			t.Fatalf("disabled=%v, want %v", n.Disabled, want)
		}
	}
	check(false)
	busy.Set(true)
	check(true)
	w.Disabled(false)
	check(false)
	busy.Set(false)
	busy.Set(true)
	check(false)
	w.Disabled(true).BindDisabled(busy)
	check(true)
	busy.Set(false)
	check(false)
	replacement := ggui.State(false)
	w.BindDisabled(replacement)
	busy.Set(true)
	check(false)
	replacement.Set(true)
	check(true)
}

func TestDisabledSettingsAcrossControls(t *testing.T) {
	t.Run("input OTP", func(t *testing.T) {
		checkDisabled(t, ui.InputOTP(ggui.State(""), 6).Name("Code"), ggui.RoleTextField, "Code")
	})
	t.Run("carousel", func(t *testing.T) {
		checkDisabled(t, ui.Carousel(ggui.State(0), ggui.Text("1"), ggui.Text("2")).Name("Slides"), ggui.RoleGroup, "Slides")
	})
	t.Run("button", func(t *testing.T) { checkDisabled(t, ui.Button("B", nil), ggui.RoleButton, "B") })
	t.Run("checkbox", func(t *testing.T) { checkDisabled(t, ui.Checkbox(ggui.State(false), "C"), ggui.RoleCheckbox, "C") })
	t.Run("switch", func(t *testing.T) { checkDisabled(t, ui.Switch(ggui.State(false), "S"), ggui.RoleSwitch, "S") })
	t.Run("radio", func(t *testing.T) { checkDisabled(t, ui.Radio(ggui.State(1), 1, "R"), ggui.RoleRadio, "R") })
	t.Run("radios", func(t *testing.T) {
		checkDisabled(t, ui.Radios(ggui.State(1)).Options([]int{1, 2}), ggui.RoleRadio, "1")
	})
	t.Run("slider", func(t *testing.T) {
		checkDisabled(t, ui.Slider(ggui.State(0.0), 0, 10).Name("S"), ggui.RoleSlider, "S")
	})
	t.Run("editor", func(t *testing.T) {
		checkDisabled(t, ggui.TextInput(ggui.State("")).Name("E"), ggui.RoleTextField, "E")
	})
	t.Run("textfield", func(t *testing.T) { checkDisabled(t, ui.TextField(ggui.State("")).Name("E"), ggui.RoleTextField, "E") })
	t.Run("input group", func(t *testing.T) {
		checkDisabled(t, ui.InputGroup(ggui.TextInput(ggui.State("")).Name("E")), ggui.RoleTextField, "E")
	})
	t.Run("select", func(t *testing.T) {
		checkDisabled(t, ui.Select(ggui.State(1)).Options([]int{1, 2}).Name("S"), ggui.RoleSelect, "S")
	})
	t.Run("combobox", func(t *testing.T) {
		checkDisabled(t, ui.Combobox(ggui.State(1)).Options([]int{1, 2}), ggui.RoleCombobox, "Choose option")
	})
	t.Run("datepicker", func(t *testing.T) {
		checkDisabled(t, ui.DatePicker(ggui.State(time.Time{})), ggui.RoleButton, "Choose date")
	})
	t.Run("menu", func(t *testing.T) { checkDisabled(t, ui.Menu("M", ui.MenuItem("Open", nil)), ggui.RoleMenu, "M") })
	t.Run("menu item", func(t *testing.T) { checkDisabled(t, ui.MenuItem("Open", nil), ggui.RoleMenuItem, "Open") })
	t.Run("contextmenu", func(t *testing.T) {
		checkDisabled(t, ui.ContextMenu(ggui.Text("Content"), ui.MenuItem("Open", nil)), ggui.RoleMenu, "Context menu")
	})
	t.Run("tabs", func(t *testing.T) {
		checkDisabled(t, ui.Tabs(ggui.State(0), ui.Tab("T", ggui.Text("Page"))), ggui.RoleTab, "T")
	})
	t.Run("calendar", func(t *testing.T) {
		checkDisabled(t, ui.Calendar(ggui.State(time.Time{})), ggui.RoleButton, "Next month")
	})
	t.Run("pagination", func(t *testing.T) {
		checkDisabled(t, ui.Pagination(ggui.State(2), ggui.State(3)), ggui.RoleButton, "Next")
	})
	t.Run("sidebar", func(t *testing.T) {
		checkDisabled(t, ui.Sidebar(ggui.State("a"), ui.SidebarItem("a", "A")), ggui.RoleTab, "A")
	})
	t.Run("resizable", func(t *testing.T) {
		checkDisabled(t, ui.Resizable(ggui.State(.5), ggui.Text("A"), ggui.Text("B")), ggui.RoleSeparator, "Resize panels")
	})
	t.Run("collapsible", func(t *testing.T) {
		checkDisabled(t, ui.Collapsible(ggui.State(true), "C", ggui.Text("Body")), ggui.RoleDisclosure, "C")
	})
	t.Run("toggle group", func(t *testing.T) {
		checkDisabled(t, ui.ToggleGroup(ggui.State(1)).Options([]int{1, 2}), ggui.RoleRadio, "1")
	})
}

func checkPopupDisabled[W interface {
	ggui.Widget
	BindDisabled(ggui.Readable[bool]) W
	Popup() *ggui.PopupWidget
}](t *testing.T, w W) {
	busy := ggui.State(false)
	w.BindDisabled(busy)
	p := ggui.NewProbe(w, ggui.Sz(500, 500))
	defer p.Close()
	p.Frame()
	w.Popup().Show()
	p.Frame()
	if !w.Popup().IsOpen() {
		t.Fatal("popup did not open")
	}
	busy.Set(true)
	p.Frame()
	if w.Popup().IsOpen() {
		t.Fatal("disabled popup stayed open")
	}
	w.Popup().Show()
	p.Frame()
	if w.Popup().IsOpen() {
		t.Fatal("disabled popup reopened")
	}
	busy.Set(false)
	p.Frame()
	w.Popup().Show()
	p.Frame()
	if !w.Popup().IsOpen() {
		t.Fatal("enabled popup could not reopen")
	}
}

func TestReactiveDisabledClosesPopups(t *testing.T) {
	t.Run("select", func(t *testing.T) { checkPopupDisabled(t, ui.Select(ggui.State(1)).Options([]int{1, 2})) })
	t.Run("combobox", func(t *testing.T) { checkPopupDisabled(t, ui.Combobox(ggui.State(1)).Options([]int{1, 2})) })
	t.Run("datepicker", func(t *testing.T) { checkPopupDisabled(t, ui.DatePicker(ggui.State(time.Time{}))) })
	t.Run("menu", func(t *testing.T) { checkPopupDisabled(t, ui.Menu("M", ui.MenuItem("Open", nil))) })
	t.Run("contextmenu", func(t *testing.T) {
		checkPopupDisabled(t, ui.ContextMenu(ggui.Text("Content"), ui.MenuItem("Open", nil)))
	})
}

func TestInputGroupPreservesEditorBindingAndAddon(t *testing.T) {
	value := ggui.State("abc")
	childDisabled, groupDisabled := ggui.State(false), ggui.State(false)
	editor := ggui.TextInput(value).Name("Editor").BindDisabled(childDisabled)
	clicks := 0
	group := ui.InputGroup(editor).BindDisabled(groupDisabled).Trailing(ui.Button("Addon", func() { clicks++ }))
	p := ggui.NewProbe(group, ggui.Sz(400, 50))
	defer p.Close()
	p.Tap("Editor")
	groupDisabled.Set(true)
	n := node(t, p.Semantics(), ggui.RoleTextField, "Editor")
	if !n.Disabled {
		t.Fatal("editor is enabled")
	}
	if _, ok := p.Find("Editor"); ok {
		t.Fatal("disabled editor has hit regions")
	}
	p.Click(ggui.Pt(2, 20))
	p.Type(ggui.Mods{}, ggui.KeyDelete)
	p.Perform(n.ID, ggui.Action{Kind: ggui.ActionSetValue, Text: "changed"})
	if ggui.Untrack(value.Get) != "abc" {
		t.Fatal("disabled editor accepted input")
	}
	p.Tap("Addon")
	if clicks != 1 {
		t.Fatal("addon should remain usable")
	}
	childDisabled.Set(true)
	groupDisabled.Set(false)
	if !node(t, p.Semantics(), ggui.RoleTextField, "Editor").Disabled {
		t.Fatal("group erased child disabled setting")
	}
	childDisabled.Set(false)
	if node(t, p.Semantics(), ggui.RoleTextField, "Editor").Disabled {
		t.Fatal("child binding no longer updates")
	}
	p.Tap("Editor")
	p.Type(ggui.Mods{}, ggui.KeyBackspace)
	if ggui.Untrack(value.Get) == "abc" {
		t.Fatal("re-enabled editor did not accept input")
	}
}

func TestInheritedInputDisabledCannotBeClearedByDescendant(t *testing.T) {
	editor := ggui.TextInput(ggui.State("value")).Name("Editor")
	w := ggui.Provide(ggui.InputDisabled, true, ggui.Provide(ggui.InputDisabled, false, ui.InputGroup(editor).Disabled(false)))
	p := ggui.NewProbe(w, ggui.Sz(300, 50))
	defer p.Close()
	if !node(t, p.Semantics(), ggui.RoleTextField, "Editor").Disabled {
		t.Fatal("descendant cleared inherited disabling")
	}
	if _, ok := p.Find("Editor"); ok {
		t.Fatal("disabled editor can receive focus")
	}
}

func TestPaginationDisabledDoesNotSubscribeBuilder(t *testing.T) {
	pages := ggui.State(3)
	builds := 0
	p := ggui.ProbeBuilder(func() ggui.Widget {
		builds++
		return ui.Pagination(ggui.State(1), pages).Disabled(false)
	}, ggui.Sz(500, 60))
	defer p.Close()
	p.Frame()
	pages.Set(4)
	p.Frame()
	if builds != 1 {
		t.Fatalf("pages triggered %d builds", builds)
	}
	node(t, p.Semantics(), ggui.RoleButton, "Page 4")
}

func TestDisabledMenuClosesSharedMenubarPopup(t *testing.T) {
	busy := ggui.State(false)
	menu := ui.Menu("File", ui.MenuItem("Open", nil)).BindDisabled(busy)
	bar := ui.Menubar(menu)
	p := ggui.NewProbe(bar, ggui.Sz(400, 300))
	defer p.Close()
	p.Tap("File")
	if !bar.Popup().IsOpen() {
		t.Fatal("menu did not open")
	}
	busy.Set(true)
	p.Frame()
	if bar.Popup().IsOpen() {
		t.Fatal("shared popup stayed open after its menu was disabled")
	}
}

func TestTextFieldPreservesEditorDisabledBinding(t *testing.T) {
	busy := ggui.State(true)
	field := ui.TextField(ggui.State("text")).Name("Field")
	field.Input().BindDisabled(busy)
	p := ggui.NewProbe(field, ggui.Sz(300, 50))
	defer p.Close()
	if !node(t, p.Semantics(), ggui.RoleTextField, "Field").Disabled {
		t.Fatal("editor binding lost")
	}
	field.Disabled(true)
	busy.Set(false)
	p.Frame()
	if !field.Input().IsDisabled() {
		t.Fatal("field did not disable editor")
	}
	field.Disabled(false)
	p.Frame()
	if field.Input().IsDisabled() {
		t.Fatal("inherited disabling was not cleared")
	}
	busy.Set(true)
	p.Frame()
	if !field.Input().IsDisabled() {
		t.Fatal("field erased editor binding")
	}
	if _, ok := p.Find("Field"); ok {
		t.Fatal("padding can focus disabled editor")
	}
}

func TestInputGroupNamesFollowEditor(t *testing.T) {
	editor := ggui.TextInput(ggui.State("")).Placeholder("Hint")
	group := ui.InputGroup(editor)
	p := ggui.NewProbe(group, ggui.Sz(300, 50))
	defer p.Close()
	role, name := group.Semantics()
	if group.HasName() || name != "Hint" {
		t.Fatal("placeholder must be a fallback")
	}
	node(t, p.Semantics(), role, name)
	editor.Name("Explicit")
	ui.Field("Outer", group)
	role, name = group.Semantics()
	if !group.HasName() || name != "Explicit" {
		t.Fatal("group did not preserve editor's current name")
	}
	node(t, p.Semantics(), role, name)
}
