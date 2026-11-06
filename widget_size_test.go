package ggui

import (
	"image/color"
	"runtime"
	"testing"
	"unsafe"

	"github.com/ironpark/ggfx"
)

// steadyTextFrame lays w out and paints it, as a frame that finds nothing
// changed does.
func steadyTextFrame(b *testing.B, w Widget, env Env) {
	img := ggfx.NewImage(200, 200)
	r := Rct(Pt(0, 0), Sz(200, 200))
	frame := func() {
		w.Layout(Loose(r.Size), env)
		w.Paint(&Canvas{Image: img}, r)
	}
	for range 4 {
		frame()
	}
	b.ReportAllocs()
	for b.Loop() {
		frame()
	}
}

// A steady frame of a Text lays out and paints nothing new, and allocates
// no more for keeping its rarely used parts apart.
func BenchmarkSteadyTextFrame(b *testing.B) {
	key := NewEnvKey[TextStyle]("bench style")
	rtl := Env{}.With(DirectionKey, DirRTL)
	for _, tc := range []struct {
		name string
		w    func() Widget
		env  Env
	}{
		{"plain", func() Widget { return Text("hello world, a plain line of text") }, Env{}},
		{"rtl", func() Widget { return Text("שלום world") }, rtl},
		{"mixed", func() Widget { return Text("hello שלום world") }, Env{}},
		{"selectable", func() Widget { return Text("hello world").Selectable() }, Env{}},
		{"stylekey", func() Widget { return Text("hello").StyleKey(key, TextStyle{Size: 20}) }, Env{}},
		{"textf", func() Widget { return Textf("n=%d", 42) }, Env{}},
		{"box", func() Widget { return Box(Text("in a box")).Pad(4).Fill(color.White) }, Env{}},
		{"box-decorated", func() Widget {
			return Box(Text("in a box")).Pad(4).Fill(color.White).
				Gradient(LinearGradient(0, GradientStop{Color: color.White}, GradientStop{Offset: 1, Color: color.Black}))
		}, Env{}},
	} {
		b.Run(tc.name, func(b *testing.B) { steadyTextFrame(b, tc.w(), tc.env) })
	}
}

// BenchmarkTextHeap reports the heap 10k Texts hold, as built and once
// laid out and painted.
func BenchmarkTextHeap(b *testing.B) {
	const n = 10000
	img := ggfx.NewImage(200, 200)
	r := Rct(Pt(0, 0), Sz(200, 20))
	for _, painted := range []bool{false, true} {
		b.Run(pick(painted, "painted", "built"), func(b *testing.B) {
			for b.Loop() {
				var before, after runtime.MemStats
				runtime.GC()
				runtime.ReadMemStats(&before)
				ws := make([]*TextWidget, n)
				for i := range ws {
					ws[i] = Text("hello world")
					if painted {
						ws[i].Layout(Loose(r.Size), Env{})
						ws[i].Paint(&Canvas{Image: img}, r)
					}
				}
				runtime.GC()
				runtime.ReadMemStats(&after)
				b.ReportMetric(float64(after.HeapAlloc-before.HeapAlloc)/n, "heapB/text")
				runtime.KeepAlive(ws)
			}
		})
	}
}

// The widgets a screen holds by the hundred stay small: the parts few of
// them use are kept apart, made only when set. Each bound is the Go size
// class the widget is allocated in.
func TestWidgetSizes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		got, most uintptr
	}{
		{"TextWidget", unsafe.Sizeof(TextWidget{}), 576},
		{"BoxWidget", unsafe.Sizeof(BoxWidget{}), 288},
	} {
		if tc.got > tc.most {
			t.Errorf("%s is %d bytes, want at most %d", tc.name, tc.got, tc.most)
		}
	}
}

// A Text makes its rarely used parts only on the paths that need them, and
// keeps working when it stops needing them.
func TestTextKeepsRarePartsApart(t *testing.T) {
	t.Parallel()
	c := Loose(Sz(300, 100))
	w := Text("hello world")
	w.Layout(c, Env{})
	w.Paint(&Canvas{}, Rct(Pt(0, 0), Sz(300, 100)))
	if w.bidi != nil || w.opts != nil || w.rich != nil || w.richAt != nil {
		t.Fatalf("a plain Text made bidi %v, opts %v, rich %v", w.bidi, w.opts, w.rich)
	}
	rtl := Env{}.With(DirectionKey, DirRTL)
	w.Layout(c, rtl)
	if w.bidi == nil || startAlign(w.dir()) != 1 {
		t.Fatal("a Text under DirRTL is not ordered right to left")
	}
	w.Layout(c, Env{})
	if startAlign(w.dir()) != 0 || w.bidi.lines[0] != nil {
		t.Fatal("a Text back under DirLTR still draws right to left")
	}

	mixed := Text("hello שלום")
	mixed.Layout(c, Env{})
	if mixed.bidi == nil || mixed.bidi.lines[0] == nil {
		t.Fatal("a line holding right-to-left text was not reordered")
	}

	f := Textf("n=%d", 7)
	f.Layout(c, Env{})
	if f.value != "n=7" {
		t.Fatalf("Textf laid out %q", f.value)
	}
	f.Content("plain")
	f.Layout(c, Env{})
	if f.value != "plain" || f.opts.format != "" || f.opts.formatArgs != nil {
		t.Fatalf("Content after Textf laid out %q, kept %+v", f.value, *f.opts)
	}

	b := Box(Text("x")).Shadow()
	b.Gradient(Gradient{})
	b.Layout(c, Env{})
	b.Paint(&Canvas{}, Rct(Pt(0, 0), Sz(10, 10)))
	if b.deco != nil {
		t.Fatal("a Box with no shadow or gradient made a boxDeco")
	}
}
