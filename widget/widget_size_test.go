package widget

import (
	"image/color"
	"runtime"
	"testing"
	"unsafe"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/pkg/gem"
)

// steadyTextFrame lays w out and paints it, as a frame that finds nothing
// changed does.
func steadyTextFrame(b *testing.B, w ggui.Widget, env ggui.Env) {
	img := ggfx.NewImage(200, 200)
	r := ggui.Rct(ggui.Pt(0, 0), ggui.Sz(200, 200))
	frame := func() {
		w.Layout(ggui.Loose(r.Size), env)
		w.Paint(&ggui.Canvas{Image: img}, r)
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
	key := ggui.NewEnvKey[ggui.TextStyle]("bench style")
	rtl := ggui.Env{}.With(ggui.DirectionKey, ggui.DirRTL)
	for _, tc := range []struct {
		name string
		w    func() ggui.Widget
		env  ggui.Env
	}{
		{"plain", func() ggui.Widget { return Text("hello world, a plain line of text") }, ggui.Env{}},
		{"rtl", func() ggui.Widget { return Text("שלום world") }, rtl},
		{"mixed", func() ggui.Widget { return Text("hello שלום world") }, ggui.Env{}},
		{"selectable", func() ggui.Widget { return Text("hello world").Selectable() }, ggui.Env{}},
		{"stylekey", func() ggui.Widget { return Text("hello").StyleKey(key, ggui.TextStyle{Size: 20}) }, ggui.Env{}},
		{"textf", func() ggui.Widget { return Textf("n=%d", 42) }, ggui.Env{}},
		{"box", func() ggui.Widget { return Box(Text("in a box")).Pad(4).Fill(color.White) }, ggui.Env{}},
		{"box-decorated", func() ggui.Widget {
			return Box(Text("in a box")).Pad(4).Fill(color.White).
				Gradient(ggui.LinearGradient(0, ggui.GradientStop{Color: color.White}, ggui.GradientStop{Offset: 1, Color: color.Black}))
		}, ggui.Env{}},
	} {
		b.Run(tc.name, func(b *testing.B) { steadyTextFrame(b, tc.w(), tc.env) })
	}
}

// BenchmarkTextHeap reports the heap 10k Texts hold, as built and once
// laid out and painted.
func BenchmarkTextHeap(b *testing.B) {
	const n = 10000
	img := ggfx.NewImage(200, 200)
	r := ggui.Rct(ggui.Pt(0, 0), ggui.Sz(200, 20))
	for _, painted := range []bool{false, true} {
		b.Run(gem.Pick(painted, "painted", "built"), func(b *testing.B) {
			for b.Loop() {
				var before, after runtime.MemStats
				runtime.GC()
				runtime.ReadMemStats(&before)
				ws := make([]*TextWidget, n)
				for i := range ws {
					ws[i] = Text("hello world")
					if painted {
						ws[i].Layout(ggui.Loose(r.Size), ggui.Env{})
						ws[i].Paint(&ggui.Canvas{Image: img}, r)
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
	c := ggui.Loose(ggui.Sz(300, 100))
	w := Text("hello world")
	w.Layout(c, ggui.Env{})
	w.Paint(&ggui.Canvas{}, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(300, 100)))
	if w.bidi != nil || w.opts != nil || w.rich != nil {
		t.Fatalf("a plain Text made bidi %v, opts %v, rich %v", w.bidi, w.opts, w.rich)
	}
	rtl := ggui.Env{}.With(ggui.DirectionKey, ggui.DirRTL)
	w.Layout(c, rtl)
	if w.bidi == nil || startAlign(w.dir()) != 1 {
		t.Fatal("a Text under DirRTL is not ordered right to left")
	}
	w.Layout(c, ggui.Env{})
	if startAlign(w.dir()) != 0 || w.bidi.lines[0] != nil {
		t.Fatal("a Text back under DirLTR still draws right to left")
	}

	mixed := Text("hello שלום")
	mixed.Layout(c, ggui.Env{})
	if mixed.bidi == nil || mixed.bidi.lines[0] == nil {
		t.Fatal("a line holding right-to-left text was not reordered")
	}

	f := Textf("n=%d", 7)
	f.Layout(c, ggui.Env{})
	if f.value != "n=7" {
		t.Fatalf("Textf laid out %q", f.value)
	}
	f.Content("plain")
	f.Layout(c, ggui.Env{})
	if f.value != "plain" || f.opts.format != "" || f.opts.formatArgs != nil {
		t.Fatalf("Content after Textf laid out %q, kept %+v", f.value, *f.opts)
	}

	b := Box(Text("x")).Shadow()
	b.Gradient(ggui.Gradient{})
	b.Layout(c, ggui.Env{})
	b.Paint(&ggui.Canvas{}, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(10, 10)))
	if b.deco != nil {
		t.Fatal("a Box with no shadow or gradient made a boxDeco")
	}
}
