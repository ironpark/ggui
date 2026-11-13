package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// comboboxProbe is a combobox over three fruit with a button after it.
func comboboxProbe(t *testing.T, value *ggui.StateValue[string], configure func(*ui.ComboboxWidget[string])) (*ggui.Probe, *ui.ComboboxWidget[string]) {
	t.Helper()
	c := ui.Combobox(value).Options([]string{"Apple", "Banana", "Cherry"}).Name("Fruit")
	if configure != nil {
		configure(c)
	}
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(c, ui.Button("After", nil))), ggui.Sz(360, 360))
	t.Cleanup(p.Close)
	return p, c
}

// comboboxText is what the combobox's input holds.
func comboboxText(t *testing.T, p *ggui.Probe) string {
	t.Helper()
	return node(t, p.Semantics(), ggui.RoleCombobox, "Fruit").Value
}

func TestComboboxTypingFiltersInPlaceAndKeepsTheKeyboard(t *testing.T) {
	t.Parallel()
	value := ggui.State("Apple")
	p, c := comboboxProbe(t, value, nil)
	p.Type(ggui.Mods{}, ggui.KeyTab)
	if c.Popup().IsOpen() {
		t.Fatal("focusing the input opened the list")
	}
	p.Type(ggui.Mods{Meta: true}, ggui.KeyA)
	p.Type(ggui.Mods{Ctrl: true}, ggui.KeyA)
	p.Type(ggui.Mods{}, ggui.KeyBackspace)
	p.Text("an")
	p.Frame()
	if !c.Popup().IsOpen() {
		t.Fatal("typing did not open the list")
	}
	if _, ok := p.Find("Banana"); !ok {
		t.Fatal("the match is not listed")
	}
	for _, gone := range []string{"Apple", "Cherry"} {
		if _, ok := p.Find(gone); ok {
			t.Fatalf("%s does not match but is listed", gone)
		}
	}
	if got := focusedName(t, p); got != "Fruit" {
		t.Fatalf("%q has the keyboard, not the input", got)
	}
	if got := comboboxText(t, p); got != "an" {
		t.Fatalf("the input holds %q", got)
	}
	if value.Get() != "Apple" {
		t.Fatal("typing changed the value")
	}
}

func TestComboboxKeyboardChoosesAndEscapeCloses(t *testing.T) {
	t.Parallel()
	value := ggui.State("Apple")
	changes := 0
	p, c := comboboxProbe(t, value, func(c *ui.ComboboxWidget[string]) { c.OnChange(func(string) { changes++ }) })
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyArrowDown)
	if !c.Popup().IsOpen() {
		t.Fatal("Down did not open the list")
	}
	// The chosen option is highlighted first; Down moves on from it.
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyArrowDown, ggui.KeyEnter)
	if value.Get() != "Cherry" || changes != 1 || c.Popup().IsOpen() {
		t.Fatalf("Enter chose %q (%d changes), open=%v", value.Get(), changes, c.Popup().IsOpen())
	}
	if got := comboboxText(t, p); got != "Cherry" {
		t.Fatalf("the input holds %q after choosing", got)
	}
	// Up from the top wraps to the bottom.
	p.Type(ggui.Mods{}, ggui.KeyArrowUp)
	if !c.Popup().IsOpen() {
		t.Fatal("Up did not open the list")
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowUp, ggui.KeyEnter)
	if value.Get() != "Banana" || changes != 2 {
		t.Fatalf("Up then Enter chose %q", value.Get())
	}

	// Escape closes the list and keeps what was typed; a second Escape puts
	// the chosen label back.
	p.Text("x")
	if !c.Popup().IsOpen() {
		t.Fatal("typing did not open the list")
	}
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if c.Popup().IsOpen() {
		t.Fatal("Escape did not close the list")
	}
	if got := comboboxText(t, p); got != "Bananax" {
		t.Fatalf("the first Escape changed the text to %q", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if got := comboboxText(t, p); got != "Banana" || value.Get() != "Banana" || changes != 2 {
		t.Fatalf("the second Escape left %q, value %q", got, value.Get())
	}
	if got := focusedName(t, p); got != "Fruit" {
		t.Fatalf("%q has the keyboard after Escape", got)
	}
}

func TestComboboxEnterWithNoMatchChoosesNothing(t *testing.T) {
	t.Parallel()
	value := ggui.State("Apple")
	p, c := comboboxProbe(t, value, nil)
	p.Tap("Fruit")
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	p.Text("zz")
	if !c.Popup().IsOpen() {
		t.Fatal("the list closed on no matches")
	}
	for _, o := range []string{"Apple", "Banana", "Cherry"} {
		if _, ok := p.Find(o); ok {
			t.Fatalf("%s is listed for a query nothing matches", o)
		}
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if value.Get() != "Apple" {
		t.Fatalf("Enter with nothing listed chose %q", value.Get())
	}
}

func TestComboboxChevronOpensWithTheKeyboardInTheInput(t *testing.T) {
	t.Parallel()
	value := ggui.State("Apple")
	p, c := comboboxProbe(t, value, nil)
	p.Tap("Show options")
	if !c.Popup().IsOpen() {
		t.Fatal("the chevron did not open the list")
	}
	p.Frame()
	if got := focusedName(t, p); got != "Fruit" {
		t.Fatalf("%q has the keyboard, not the input", got)
	}
	// Every option is listed: the chosen label is not a query.
	for _, o := range []string{"Apple", "Banana", "Cherry"} {
		if _, ok := p.Find(o); !ok {
			t.Fatalf("%s is not listed", o)
		}
	}
	p.Tap("Banana")
	if value.Get() != "Banana" || c.Popup().IsOpen() {
		t.Fatalf("a click chose %q, open=%v", value.Get(), c.Popup().IsOpen())
	}
	if got := focusedName(t, p); got != "Fruit" {
		t.Fatalf("%q has the keyboard after a click on an option", got)
	}
	p.Tap("Show options")
	p.Tap("Show options")
	if c.Popup().IsOpen() {
		t.Fatal("the chevron did not close the list")
	}
}

func TestComboboxClearButtonClearsTheValue(t *testing.T) {
	t.Parallel()
	value := ggui.State("Banana")
	var changed []string
	p, c := comboboxProbe(t, value, func(c *ui.ComboboxWidget[string]) {
		c.ClearButton(true).Placeholder("Pick a fruit").OnChange(func(v string) { changed = append(changed, v) })
	})
	if _, ok := p.Find("Show options"); ok {
		t.Fatal("the chevron shows beside the clear button")
	}
	p.Tap("Clear")
	if value.Get() != "" || len(changed) != 1 || changed[0] != "" {
		t.Fatalf("clear left %q, changes %q", value.Get(), changed)
	}
	if got := comboboxText(t, p); got != "" {
		t.Fatalf("the input holds %q after clearing", got)
	}
	if _, ok := p.Find("Clear"); ok {
		t.Fatal("the clear button stays with no value")
	}
	if _, ok := p.Find("Show options"); !ok {
		t.Fatal("the chevron did not come back")
	}
	if c.Popup().IsOpen() {
		t.Fatal("clearing opened the list")
	}
}

func TestComboboxLeavingTheInputRestoresTheChosenLabel(t *testing.T) {
	t.Parallel()
	value := ggui.State("Cherry")
	p, c := comboboxProbe(t, value, nil)
	p.Tap("Fruit")
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	p.Type(ggui.Mods{}, ggui.KeyBackspace, ggui.KeyBackspace, ggui.KeyBackspace)
	p.Text("zzz")
	if got := comboboxText(t, p); got != "Chezzz" || !c.Popup().IsOpen() {
		t.Fatalf("typed %q, open=%v", got, c.Popup().IsOpen())
	}
	p.Type(ggui.Mods{}, ggui.KeyTab)
	if got := focusedName(t, p); got != "After" {
		t.Fatalf("Tab moved the keyboard to %q", got)
	}
	if c.Popup().IsOpen() {
		t.Fatal("the list stayed open after Tab")
	}
	if got := comboboxText(t, p); got != "Cherry" || value.Get() != "Cherry" {
		t.Fatalf("leaving left %q, value %q", got, value.Get())
	}
}

func TestComboboxShowsAnExternalValueAndDisables(t *testing.T) {
	t.Parallel()
	value := ggui.State("Apple")
	p, c := comboboxProbe(t, value, nil)
	value.Set("Cherry")
	p.Frame()
	if got := comboboxText(t, p); got != "Cherry" {
		t.Fatalf("the input shows %q for Cherry", got)
	}
	value.Set("Durian")
	p.Frame()
	if got := comboboxText(t, p); got != "" {
		t.Fatalf("the input shows %q for a value not among the options", got)
	}
	c.Disabled(true)
	p.Frame()
	if _, ok := p.Find("Fruit"); ok {
		t.Fatal("the disabled input takes input")
	}
	if _, ok := p.Find("Show options"); ok {
		t.Fatal("the disabled chevron takes input")
	}
}
