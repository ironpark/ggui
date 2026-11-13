package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestPopoverFocusDismissalAndBinding(t *testing.T) {
	t.Parallel()
	open := ggui.State(false)
	disabled := ggui.State(false)
	closes, outside := 0, 0
	pover := ui.Popover("Settings", ggui.Column(ui.Button("First", nil), ui.Button("Second", nil))).Modal(true).BindOpen(open).BindDisabled(disabled).OnClose(func() { closes++ })
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(pover, ggui.Box().Height(160), ui.Button("Outside", func() { outside++ }))), ggui.Sz(440, 380))
	defer p.Close()
	p.Key("tab")
	p.Key("enter")
	if !open.Get() {
		t.Fatal("Enter did not open bound popup")
	}
	p.Move(ggui.Pt(430, 370))
	focused, _ := p.Semantics().Focused()
	if focused.Name != "First" {
		t.Fatalf("initial focus = %s", focused.Name)
	}
	p.Key("shift+tab")
	focused, _ = p.Semantics().Focused()
	if focused.Name != "Second" {
		t.Fatalf("focus escaped panel: %s", focused.Name)
	}
	p.Key("escape")
	p.Move(ggui.Pt(430, 370))
	focused, _ = p.Semantics().Focused()
	if open.Get() || focused.Name != "Settings" || closes != 1 {
		t.Fatalf("Escape: open=%v focused=%s closes=%d", open.Get(), focused.Name, closes)
	}
	p.Key("space")
	p.Tap("Outside")
	if open.Get() || outside != 0 || closes != 2 {
		t.Fatal("outside click leaked or failed to dismiss")
	}
	open.Set(true)
	p.Frame()
	disabled.Set(true)
	p.Frame()
	if open.Get() {
		t.Fatal("disabling trigger did not close bound popup")
	}
	pover.Show()
	if pover.IsOpen() {
		t.Fatal("disabled Show opened popup")
	}
}

func TestPopoverKeyedRebuildAndComposition(t *testing.T) {
	t.Parallel()
	revision := ggui.State(0)
	var pop *ui.PopoverWidget
	p := ggui.ProbeBuilder(func() ggui.Widget {
		_ = revision.Get()
		pop = ui.PopoverOf(ggui.Text("Open custom"), ui.Button("Close content", func() { pop.Hide() })).Name("Custom").Key("custom").Width(240).Pad(12)
		return ggui.Column(pop)
	}, ggui.Sz(360, 320))
	defer p.Close()
	p.Tap("Custom")
	p.Advance(time.Second)
	revision.Set(1)
	p.Frame()
	if !pop.IsOpen() {
		t.Fatal("keyed rebuild lost popup state")
	}
	if got := pop.Popup().Rect().Size.W; got != 240 {
		t.Fatalf("popup width=%v", got)
	}
	p.Tap("Close content")
	if pop.IsOpen() {
		t.Fatal("composed close did not dismiss")
	}
}

func TestPopoverAccessibleActionsAndSmallViewport(t *testing.T) {
	t.Parallel()
	pop := ui.Popover("Open", ggui.Column(ui.Button("A", nil), ggui.Box().Height(500), ui.Button("Z", nil)))
	p := ggui.NewProbe(ggui.Column(ggui.Box().Height(140), pop), ggui.Sz(180, 180))
	defer p.Close()
	act(t, p, ggui.RoleButton, "Open", ggui.Action{Kind: ggui.ActionExpand})
	p.Advance(time.Second)
	r := pop.Popup().Rect()
	if r.Origin.X < 0 || r.Origin.Y < 0 || r.Origin.X+r.Size.W > 180 || r.Origin.Y+r.Size.H > 180 {
		t.Fatalf("popup outside viewport: %v", r)
	}
	act(t, p, ggui.RoleButton, "Open", ggui.Action{Kind: ggui.ActionCollapse})
	if pop.IsOpen() {
		t.Fatal("accessible collapse failed")
	}
}

func TestPopoverDefaultsAreNonmodalAndCentered(t *testing.T) {
	t.Parallel()
	clicks := 0
	pop := ui.Popover("Open", ui.Button("Inside", nil))
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(ggui.Padding(pop, 0, 200), ggui.Box().Height(150), ui.Button("Outside", func() { clicks++ }))), ggui.Sz(600, 350))
	defer p.Close()
	p.Tap("Open")
	p.Frame()
	trigger, _ := p.Find("Open")
	r := pop.Popup().Rect()
	if r.Size.W != 288 || r.Center().X != trigger.Rect.Center().X {
		t.Fatalf("default panel=%v trigger=%v", r, trigger.Rect)
	}
	p.Move(ggui.Pt(590, 340))
	p.Key("shift+tab")
	p.Frame()
	if pop.IsOpen() {
		t.Fatal("focus leaving nonmodal panel did not dismiss")
	}
	p.Tap("Open")
	p.Tap("Outside")
	if pop.IsOpen() || clicks != 1 {
		t.Fatalf("outside click: open=%v calls=%d", pop.IsOpen(), clicks)
	}
}

func TestPopoverNestedPanelsKeepParentOpen(t *testing.T) {
	t.Parallel()
	inner := ui.Popover("Inner", ui.Button("Deep action", nil))
	outer := ui.Popover("Outer", ggui.Column(inner, ui.Button("Sibling", nil)))
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(outer)), ggui.Sz(600, 500))
	defer p.Close()
	p.Tap("Outer")
	p.Tap("Inner")
	p.Move(ggui.Pt(590, 490))
	p.Frame()
	if !outer.IsOpen() || !inner.IsOpen() {
		t.Fatalf("nested focus closed a popup: outer=%v inner=%v", outer.IsOpen(), inner.IsOpen())
	}
	p.Key("escape")
	p.Frame()
	if !outer.IsOpen() || inner.IsOpen() {
		t.Fatalf("Escape should close only inner: outer=%v inner=%v", outer.IsOpen(), inner.IsOpen())
	}
}

func TestPopoverTextOnlyDialogSemanticsAndEscape(t *testing.T) {
	t.Parallel()
	pop := ui.Popover("Info", ggui.Text("Read only content")).Title("Details").Description("More information")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(pop)), ggui.Sz(450, 300))
	defer p.Close()
	p.Tap("Info")
	p.Frame()
	dialog, ok := p.Semantics().Find(ggui.RoleDialog, "Details")
	if !ok || dialog.Description != "More information" {
		t.Fatalf("dialog semantics: %+v", dialog)
	}
	p.Key("escape")
	p.Frame()
	if pop.IsOpen() {
		t.Fatal("text-only popup did not dismiss")
	}
	focused, ok := p.Semantics().Focused()
	if !ok || focused.Name != "Info" {
		t.Fatalf("text-only Escape did not return focus: %+v", focused)
	}
}
