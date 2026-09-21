package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestFluentButtonKeyKeepsFocusAfterMovingRebuild(t *testing.T) {
	offset := ggui.State(0.0)
	clicks := 0
	var button *ui.ButtonWidget
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			button = ui.Button("Action", func() { clicks++ }).Key("action").Outline()
			return ggui.Column(ggui.Box().Height(offset.Get()), button)
		})
	}, ggui.Sz(350, 300))
	defer p.Close()
	p.Tap("Action")
	offset.Set(50)
	p.Frame()
	if button.HitID() != "action" {
		t.Fatal("fluent Key did not reach the input identity")
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if clicks != 2 {
		t.Fatal("button lost keyboard focus after moving")
	}
}

func TestFluentSelectKeyKeepsOpenPopupAfterMovingRebuild(t *testing.T) {
	offset := ggui.State(0.0)
	value := ggui.State("Alpha")
	var control *ui.SelectWidget[string]
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			control = ui.Select(value, []string{"Alpha", "Beta"}).Key("select").Named("Choice")
			return ggui.Column(ggui.Box().Height(offset.Get()), control)
		})
	}, ggui.Sz(350, 350))
	defer p.Close()
	p.Tap("Choice")
	offset.Set(50)
	p.Frame()
	if control.HitID() != "select" || !control.Popup().IsOpen() {
		t.Fatal("select lost identity or popup state")
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if value.Get() != "Beta" {
		t.Fatal("select lost keyboard navigation after moving")
	}
}

func TestComboboxKeyKeepsPopupAndSearchFocusAfterMovingRebuild(t *testing.T) {
	offset := ggui.State(0.0)
	value := ggui.State("Alpha")
	var control *ui.ComboboxWidget[string]
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			control = ui.Combobox(value, []string{"Alpha", "Beta"}).Key("combobox").Named("Choice")
			return ggui.Column(ggui.Box().Height(offset.Get()), control)
		})
	}, ggui.Sz(350, 400))
	defer p.Close()
	p.Tap("Choice")
	firstID := control.Popup().HitID()
	offset.Set(50)
	p.Frame()
	if firstID != control.Popup().HitID() || !control.Popup().IsOpen() {
		t.Fatal("combobox lost popup identity or open state")
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if value.Get() != "Beta" || control.Popup().IsOpen() {
		t.Fatal("search lost focus or selected the wrong option after moving")
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if !control.Popup().IsOpen() {
		t.Fatal("closing did not restore focus to the keyed trigger")
	}
}

type layoutCounter struct{ layouts int }

func (w *layoutCounter) Layout(c ggui.Constraints, _ ggui.Env) ggui.Size {
	w.layouts++
	return c.Constrain(ggui.Sz(10, 10))
}
func (*layoutCounter) Paint(*ggui.Canvas, ggui.Rect) {}

func TestControlConfigurationDoesNotCauseIdleLayouts(t *testing.T) {
	leaf := &layoutCounter{}
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Column(ui.Button("Action", nil), ui.Select(ggui.State("a"), []string{"a", "b"}), leaf)
	}, ggui.Sz(350, 300))
	defer p.Close()
	p.Frame()
	p.Frame()
	if leaf.layouts != 1 {
		t.Fatalf("idle controls caused %d layouts, want 1", leaf.layouts)
	}
}
