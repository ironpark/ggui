package ui

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

func TestDisclosureHeightMotionAndLeavingInput(t *testing.T) {
	for _, accordion := range []bool{false, true} {
		t.Run(pick(accordion, "accordion", "collapsible"), func(t *testing.T) {
			on := ggui.State(false)
			keys := ggui.State([]string{})
			clicked := 0
			body := ggui.Box(Button("Inside", func() { clicked++ })).Height(100)
			var w ggui.Widget
			if accordion {
				w = Accordion(keys, AccordionItem("a", "Reveal", body))
			} else {
				w = Collapsible(on, "Reveal", body)
			}
			var measured ggui.Size
			watch := ggui.FromFuncs(func(c ggui.Constraints, e ggui.Env) ggui.Size { measured = w.Layout(c, e); return measured }, func(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(w, r) })
			p := ggui.NewProbe(ggui.Column(watch), ggui.Sz(320, 400))
			height := func() float64 { p.Frame(); return measured.H }
			defer p.Close()
			p.Advance(0)
			h0 := height()
			p.Tap("Reveal")
			p.Frame()
			p.Advance(50 * time.Millisecond)
			h1 := height()
			p.Advance(time.Second)
			h2 := height()
			if !(h0 < h1 && h1 < h2) {
				t.Fatalf("height must interpolate: %v %v %v", h0, h1, h2)
			}
			p.Tap("Reveal")
			p.Frame()
			if _, ok := p.Find("Inside"); ok {
				t.Fatal("leaving content accepts input")
			}
			p.Advance(50 * time.Millisecond)
			if h := height(); !(h > h0 && h < h2) {
				t.Fatalf("close height %v", h)
			}
			p.Advance(time.Second)
			if h := height(); h != h0 {
				t.Fatalf("closed height %v want %v", h, h0)
			}
			if clicked != 0 {
				t.Fatal("child activated")
			}
		})
	}
}

func TestDisclosureReducedMotionImmediate(t *testing.T) {
	keys := ggui.State([]string{})
	a := Accordion(keys, AccordionItem("a", "Reveal", ggui.Box().Height(100)))
	var measured ggui.Size
	watch := ggui.FromFuncs(func(c ggui.Constraints, e ggui.Env) ggui.Size { measured = a.Layout(c, e); return measured }, func(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(a, r) })
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(watch)), ggui.Sz(320, 400))
	height := func() float64 { p.Frame(); return measured.H }
	defer p.Close()
	h0 := height()
	p.Tap("Reveal")
	h1 := height()
	if h1 < h0+100 {
		t.Fatal("reduced motion should open immediately")
	}
	p.Tap("Reveal")
	if height() != h0 {
		t.Fatal("reduced motion should close immediately")
	}
}

func TestDialogMotionReversalAndExit(t *testing.T) {
	open := ggui.State(false)
	d := Dialog(open, Button("Inside", nil)).Title("Motion")
	p := ggui.NewProbe(d, ggui.Sz(500, 400))
	defer p.Close()
	p.Advance(0)
	open.Set(true)
	p.Frame()
	p.Advance(60 * time.Millisecond)
	v := d.reveal.Value(ggui.Now())
	if v <= 0 || v >= 1 {
		t.Fatalf("enter progress %v", v)
	}
	effect := d.effect
	open.Set(false)
	p.Frame()
	if _, ok := p.Find("Inside"); ok {
		t.Fatal("leaving dialog accepts input")
	}
	open.Set(true)
	p.Frame()
	p.Advance(time.Second)
	if d.reveal.Value(ggui.Now()) != 1 {
		t.Fatal("reopen did not settle")
	}
	if d.effect != effect {
		t.Fatal("dialog allocated another transition buffer")
	}
	open.Set(false)
	p.Frame()
	p.Advance(time.Second)
	if d.Rect() != (ggui.Rect{}) {
		t.Fatal("closed dialog retained panel rect")
	}
}

func TestDisabledSliderAndTabsIgnoreDirectInput(t *testing.T) {
	value := ggui.State(.5)
	slider := Slider(value, 0, 1).Disabled(true)
	selected := ggui.State(0)
	tabs := Tabs(selected, Tab("One", ggui.Text("1")), Tab("Two", ggui.Text("2"))).Disabled(true)
	p := ggui.NewProbe(ggui.Column(slider, tabs), ggui.Sz(320, 200))
	defer p.Close()
	p.Frame()
	slider.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyArrowRight})
	slider.HandlePointer(ggui.PointerEvent{Kind: ggui.PointerDown, Button: ggui.MouseButtonLeft, Pos: ggui.Pt(300, 10)})
	tabs.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyArrowRight})
	if value.Get() != .5 || selected.Get() != 0 {
		t.Fatal("disabled control changed")
	}
}

func TestHoverCardBridgeAndNarrowViewport(t *testing.T) {
	h := HoverCard(Button("Preview", nil), Button("Preview action", nil)).Width(400)
	p := ggui.NewProbe(ggui.Column(h), ggui.Sz(180, 300))
	defer p.Close()
	p.Advance(0)
	anchor, _ := p.Find("Preview")
	p.Move(anchor.Center())
	p.Advance(time.Second)
	p.Advance(time.Second)
	item, ok := p.Find("Preview action")
	if !ok {
		t.Fatal("hover card did not open")
	}
	if h.size.W > 180 {
		t.Fatal("hover card exceeds viewport")
	}
	// Move through the vertical gap; both anchor and panel remain connected.
	p.Move(ggui.Pt(anchor.Center().X, anchor.Rect.Origin.Y+anchor.Rect.Size.H+1))
	p.Advance(time.Second)
	if _, ok := p.Find("Preview action"); !ok {
		t.Fatal("hover card closed while crossing the gap")
	}
	p.Move(item.Center())
	p.Advance(time.Second)
	if _, ok := p.Find("Preview action"); !ok {
		t.Fatal("hover card closed over content")
	}
	p.Move(ggui.Pt(-1, -1))
	p.Frame()
	if _, ok := p.Find("Preview action"); ok {
		t.Fatal("leaving hover card accepts input")
	}
}

func TestFieldErrorPropagatesToInputChrome(t *testing.T) {
	err := ggui.State("")
	input := TextField(ggui.State(""))
	group := InputGroup(ggui.TextInput(ggui.State("")))
	p := ggui.NewProbe(ggui.Column(Field("Email", input).BindError(err), Field("Website", group).BindError(err)), ggui.Sz(320, 240))
	defer p.Close()
	p.Frame()
	if input.invalid || group.invalid {
		t.Fatal("valid fields marked invalid")
	}
	err.Set("Required")
	p.Frame()
	if !input.invalid || !group.invalid {
		t.Fatal("error not propagated")
	}
	err.Set("")
	p.Frame()
	if input.invalid || group.invalid {
		t.Fatal("error chrome retained after correction")
	}
}

func TestDisclosureStopsLayoutAfterSettling(t *testing.T) {
	open := ggui.State([]string{})
	layouts := 0
	body := ggui.FromFuncs(func(c ggui.Constraints, _ ggui.Env) ggui.Size { layouts++; return c.Constrain(ggui.Sz(100, 100)) }, func(*ggui.Canvas, ggui.Rect) {})
	a := Accordion(open, AccordionItem("a", "Reveal", body))
	p := ggui.NewProbe(ggui.Column(a), ggui.Sz(320, 300))
	defer p.Close()
	p.Advance(0)
	for range 20 {
		p.Frame()
	}
	if layouts != 0 {
		t.Fatal("closed content was measured")
	}
	p.Tap("Reveal")
	p.Frame()
	p.Advance(time.Second)
	p.Frame()
	settled := layouts
	for range 20 {
		p.Frame()
	}
	if layouts != settled {
		t.Fatalf("idle animation keeps invalidating layout: %d -> %d", settled, layouts)
	}
}

func TestTableRowSelectionMotion(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		selected := ggui.State(0)
		tbl := Table(ggui.State([]int{1}), func(v int) int { return v }, TextCol("ID", func(int) string { return "one" })).BindSelected(selected)
		row := tbl.row(ggui.EachItem[int]{Value: ggui.State(1), Index: ggui.State(0)}).(*tableRow[int, int])
		var amount float64
		watch := ggui.FromFuncs(row.Layout, func(dst *ggui.Canvas, rc ggui.Rect) {
			dst.Paint(row, rc)
			if m, ok := dst.Retained(row.Anchor(rc), tableRowFillSlot); ok {
				amount = m.Value(ggui.Now())
			}
		})
		p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, reduced, watch), ggui.Sz(200, 40))
		p.Advance(0)
		p.Frame()
		selected.Set(1)
		p.Frame()
		p.Advance(30 * time.Millisecond)
		p.Frame()
		if reduced && amount != 1 {
			t.Errorf("reduced motion: fill = %v", amount)
		}
		if !reduced && !(amount > 0 && amount < 1) {
			t.Errorf("selection should interpolate: %v", amount)
		}
		p.Advance(time.Second)
		p.Frame()
		if amount != 1 {
			t.Errorf("settled fill = %v", amount)
		}
		row.SetInert(true)
		selected.Set(0)
		row.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyEnter})
		row.HandlePointer(ggui.PointerEvent{Kind: ggui.PointerTap, Button: ggui.MouseButtonLeft})
		if ggui.Untrack(selected.Get) != 0 {
			t.Error("disabled row accepted input")
		}
		p.Close()
	}
}
