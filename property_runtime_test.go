package ggui

import (
	"image/color"
	"strings"
	"testing"
	"time"
)

// This reader is intentionally not comparable, but its underlying signal is
// still a measurement dependency.
type propertyNumberReader struct{ get func() float64 }

func (r propertyNumberReader) Get() float64 { return r.get() }

func measuredPropertyProbe(w Widget) (*Probe, *Size, *int) {
	size := new(Size)
	layouts := new(int)
	root := Cached(FromFuncs(func(cs Constraints, env Env) Size {
		*layouts++
		*size = w.Layout(Loose(cs.Max()), env)
		return cs.Constrain(*size)
	}, func(dst *Canvas, rc Rect) { dst.Paint(w, Rct(rc.Origin, *size)) }))
	return NewProbe(root, Sz(600, 400)), size, layouts
}

func TestPropertyDimensionsReplaceSourcesUnderNestedCaches(t *testing.T) {
	a, b := State(80.0), State(80.0)
	height := State(20.0)
	box := Box().BindWidth(a).BindHeight(height)
	p, size, _ := measuredPropertyProbe(Cached(box))
	defer p.Close()
	p.Frame()
	if *size != Sz(80, 20) {
		t.Fatal(*size)
	}
	box.BindWidth(b) // Same result, different dependencies.
	p.Frame()
	a.Set(90)
	p.Frame()
	if size.W != 80 {
		t.Fatal("old reader remained connected", *size)
	}
	b.Set(140)
	height.Set(30)
	p.Frame()
	if *size != Sz(140, 30) {
		t.Fatal(*size)
	}
	box.Size(140, 30) // Same values still detach both sources.
	b.Set(160)
	height.Set(60)
	p.Frame()
	if *size != Sz(140, 30) {
		t.Fatal("Size did not detach", *size)
	}
	box.Size(100, 40).BindWidth(propertyNumberReader{a.Get})
	a.Set(110)
	p.Frame()
	if *size != Sz(110, 40) {
		t.Fatal("non-comparable width reader", *size)
	}
}

func TestPropertyLiteralGeometryUpdatesMountedCaches(t *testing.T) {
	box := Box(Text("x")).Pad(1)
	p, size, layouts := measuredPropertyProbe(box)
	defer p.Close()
	p.Frame()
	before := *size
	box.Padding(Insets(12))
	p.Frame()
	if size.W != before.W+22 || size.H != before.H+22 {
		t.Fatalf("padding did not invalidate: %v -> %v", before, *size)
	}
	n := *layouts
	box.Pad(12)
	p.Frame()
	if *layouts != n {
		t.Fatal("equivalent shorthand caused layout")
	}
	box.Fill(color.NRGBA{R: 255, A: 255}).Border(2, color.White)
	p.Frame()
	if *layouts != n {
		t.Fatal("paint-only properties invalidated measurement")
	}
}

func TestPropertyParentConfigurationConverges(t *testing.T) {
	text := Text("unchanged")
	box := Box(text)
	layouts := 0
	root := FromFuncs(func(cs Constraints, env Env) Size {
		layouts++
		// These intermediate styles differ each pass. The final configuration is
		// consumed in this layout, so it must not force another frame's layout.
		text.Style(TextStyle{Size: 12, Color: color.Black}).Color(color.White)
		box.Pad(2).Padding(Insets(3))
		return Cached(box).Layout(cs, env)
	}, box.Paint)
	p := NewProbe(root, Sz(200, 100))
	defer p.Close()
	p.Advance(0)
	p.Frame()
	p.Frame()
	if layouts != 1 {
		t.Fatalf("configuration did not settle: %d", layouts)
	}
}

func TestTextPropertiesPullWithoutEffectsAndDetach(t *testing.T) {
	start := effects.count
	source := State("a")
	args := []any{source, Const(3)}
	a := TextOf(source)
	b := Textf("%s %d", args...)
	args[0] = "caller mutation"
	if effects.count != start {
		t.Fatal("text construction registered an effect")
	}
	p := NewProbe(Cached(Column(a, b)), Sz(300, 100))
	p.Frame()
	if b.value != "a 3" {
		t.Fatal("Textf did not own its argument list", b.value)
	}
	source.Set("b")
	p.Frame()
	if a.value != "b" || b.value != "b 3" {
		t.Fatal(a.value, b.value)
	}
	a.Content("b")
	b.Content("fixed")
	source.Set("c")
	p.Frame()
	if a.value != "b" || b.value != "fixed" {
		t.Fatal("Content did not detach")
	}
	b.BindContent(source)
	p.Frame()
	if b.value != "c" {
		t.Fatal("BindContent did not replace formatting")
	}
	p.Close()
	if effects.count != start {
		t.Fatal("text leaked computations")
	}
	// Disposing a widget does not dispose a borrowed memo used elsewhere.
	memo := Derived(func() string { return source.Get() + "!" })
	defer memo.Dispose()
	q := NewProbe(TextOf(memo), Sz(100, 40))
	q.Frame()
	q.Close()
	source.Set("d")
	if memo.Get() != "d!" {
		t.Fatal("widget disposed borrowed memo")
	}
}

func TestPropertyBindingsRejectNilReaders(t *testing.T) {
	var width *StateValue[float64]
	var text *StateValue[string]
	var flag *StateValue[bool]
	cases := map[string]func(){
		"width nil":       func() { Box().BindWidth(nil) },
		"width typed nil": func() { Box().BindWidth(width) },
		"height":          func() { Box().BindHeight(width) },
		"content":         func() { TextOf(text) },
		"name":            func() { new(Interactive).BindName(text) },
		"disabled":        func() { new(Interactive).BindInert(flag) },
		"offset":          func() { Scroll(Box()).BindOffset(width) },
		"popup":           func() { Popup(Box(), Box()).BindOpen(flag) },
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			defer func() {
				v := recover()
				if v == nil || !strings.Contains(v.(string), "non-nil reader") {
					t.Fatalf("unexpected panic: %v", v)
				}
			}()
			run()
		})
	}
}

func TestBoundDimensionsFollowAnimationWithoutRebuilding(t *testing.T) {
	tween := Tween(10.0, 100*time.Millisecond)
	box := Box().Height(10).BindWidth(tween)
	q, size, _ := measuredPropertyProbe(box)
	defer q.Close()
	q.Advance(0)
	tween.Set(100)
	q.Advance(0)
	q.Advance(200 * time.Millisecond)
	if size.W != 100 {
		t.Fatalf("animation width=%v", size.W)
	}
}

func TestScrollOffsetDetachesAndClampsLocalPosition(t *testing.T) {
	source := State(10.0)
	scroll := Scroll(Box().Size(20, 600)).BindOffset(source)
	p := NewProbe(scroll, Sz(100, 100))
	defer p.Close()
	p.Frame()
	scroll.Offset(200)
	p.Frame()
	source.Set(30)
	p.Frame()
	if scroll.position() != 200 {
		t.Fatal("old scroll source remained attached")
	}
	scroll.scrollTo(250)
	if source.Get() != 30 {
		t.Fatal("local scrolling wrote detached source")
	}
	scroll.BindOffset(source)
	p.Frame()
	scroll.scrollTo(70)
	if source.Get() != 70 {
		t.Fatal("scroll did not write back")
	}
	scroll.Offset(1000)
	p.Frame()
	if scroll.position() != 500 {
		t.Fatal("local scroll not clamped", scroll.position())
	}
}
