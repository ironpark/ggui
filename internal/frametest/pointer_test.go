package frametest_test

import (
	"image/color"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/framestats"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// A widget that reads where the pointer is in its Paint, as a tooltip
// does, is painted again as the pointer moves: in part, where it drew, as
// one that reads the time is. These tests move the pointer across such
// widgets with everything around them that could make that wrong, and
// hold each frame to what painting it whole shows.

// path is n points from a to b, the last at b.
func path(a, b ggui.Point, n int) []ggui.Point {
	out := make([]ggui.Point, n)
	for i := range out {
		f := float64(i+1) / float64(n)
		out[i] = ggui.Pt(a.X+(b.X-a.X)*f, a.Y+(b.Y-a.Y)*f)
	}
	return out
}

// along moves the pointer along points, one a frame, through the tree
// build makes; then, when given, does then.
func along(build func() ggui.Widget, points []ggui.Point, then func(p *ggui.WindowProbe, i int)) walk {
	return func() (ggui.Builder, func(*ggui.WindowProbe, int) bool) {
		return build, func(p *ggui.WindowProbe, i int) bool {
			if then != nil {
				then(p, i)
			}
			p.Move(points[i])
			return i+1 < len(points)
		}
	}
}

// follower is a size-sized panel that marks where the pointer is over it,
// within its own Rect, with a dot of col: a widget whose look follows the
// pointer and nothing else.
func follower(size ggui.Size, col color.Color) ggui.Widget {
	return ggui.FromFuncs(func(c ggui.Constraints, _ ggui.Env) ggui.Size {
		return c.Constrain(size)
	}, func(dst *ggui.Canvas, r ggui.Rect) {
		dst.FillRect(r, gray)
		if p, ok := dst.Pointer(); ok && r.Grow(-6).Contains(p) {
			dst.FillCircle(p, 5, col)
		}
	})
}

// paintedIn counts the frames painted in part, and fails t unless each lies
// within area.
func paintedIn(t *testing.T, frames []ggui.WindowFrame, area ggui.Rect) int {
	t.Helper()
	n := 0
	for i, fr := range frames {
		if fr.Painted != ggui.FramePartial {
			continue
		}
		n++
		for _, d := range fr.Damage {
			if d.Intersect(area) != d {
				t.Fatalf("frame %d painted %v, want within %v", i, fr.Damage, area)
			}
		}
	}
	return n
}

// counts is how many frames were painted each way.
func counts(frames []ggui.WindowFrame) map[ggui.FramePaint]int {
	m := map[ggui.FramePaint]int{}
	for _, fr := range frames {
		m[fr.Painted]++
	}
	return m
}

// A pointer moving over a widget that follows it paints that widget alone,
// in part, and what lies beside it is left as it was.
func TestAMovingPointerPaintsWhatFollowsItInPart(t *testing.T) {
	before := framestats.Read().Pointer
	frames := sameAsWholeAlong(t, ggui.Sz(260, 120), along(func() ggui.Widget {
		return topLeft(ggui.Row(follower(ggui.Sz(100, 80), red), ggui.Box().Size(100, 80).Fill(green)))
	}, path(ggui.Pt(10, 10), ggui.Pt(90, 70), 12), nil))
	if n := paintedIn(t, frames, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(110, 90))); n < 12 {
		t.Fatalf("%d frames painted in part, want each the pointer moved in: %v", n, counts(frames))
	}
	if got := framestats.Read().Pointer - before; got < 12 {
		t.Fatalf("%d frames counted as painted in part for the pointer, want 12", got)
	}
}

// A widget that draws at the pointer wherever it is, past its own Rect,
// draws where no region of a frame painted in part reaches: such a frame
// is painted whole.
func TestAPointerFollowerDrawingPastItsBoundsPaintsWhole(t *testing.T) {
	frames := sameAsWholeAlong(t, ggui.Sz(260, 120), along(func() ggui.Widget {
		spot := ggui.FromFuncs(func(c ggui.Constraints, _ ggui.Env) ggui.Size { return ggui.Sz(40, 40) }, func(dst *ggui.Canvas, r ggui.Rect) {
			dst.FillRect(r, blue)
			if p, ok := dst.Pointer(); ok {
				dst.FillCircle(p, 6, red)
			}
		})
		return topLeft(ggui.Row(spot, ggui.Box().Size(160, 80).Fill(green)))
	}, path(ggui.Pt(20, 20), ggui.Pt(200, 60), 10), nil))
	if counts(frames)[ggui.FrameWhole] == 0 {
		t.Fatalf("no frame painted whole for a dot drawn past its widget: %v", counts(frames))
	}
}

// A widget that reads the pointer under a Transform cannot be placed in the
// window: a move paints the whole window.
func TestAPointerReaderUnderATransformPaintsWhole(t *testing.T) {
	frames := sameAsWholeAlong(t, ggui.Sz(260, 120), along(func() ggui.Widget {
		return topLeft(ggui.Row(ggui.Transform(follower(ggui.Sz(100, 80), red)).Rotate(0.1), ggui.Box().Size(100, 80).Fill(green)))
	}, path(ggui.Pt(10, 10), ggui.Pt(90, 70), 8), nil))
	for i, fr := range frames[:8] {
		if fr.Painted != ggui.FrameWhole {
			t.Fatalf("frame %d was painted %v, want whole: %v", i, fr.Painted, counts(frames))
		}
	}
}

// A pointer reader under a shape clip cannot be placed either.
func TestAPointerReaderUnderAShapeClipPaintsWhole(t *testing.T) {
	frames := sameAsWholeAlong(t, ggui.Sz(260, 120), along(func() ggui.Widget {
		return topLeft(ggui.Row(ggui.ClipOval(follower(ggui.Sz(100, 80), red)), ggui.Box().Size(100, 80).Fill(green)))
	}, path(ggui.Pt(10, 10), ggui.Pt(90, 70), 8), nil))
	for i, fr := range frames[:8] {
		if fr.Painted != ggui.FrameWhole {
			t.Fatalf("frame %d was painted %v, want whole: %v", i, fr.Painted, counts(frames))
		}
	}
}

// Painted in part, a pointer reader under another widget stays under it,
// and one over another stays over it.
func TestAPointerReaderUnderAndOverOthersInAStack(t *testing.T) {
	sameAsWholeAlong(t, ggui.Sz(260, 160), along(func() ggui.Widget {
		under := ggui.Stack(follower(ggui.Sz(120, 100), red), ggui.Align(ggui.Box().Size(50, 50).Fill(shade)).At(0.5, 0.5))
		over := ggui.Stack(ggui.Box().Size(120, 100).Fill(blue), ggui.Align(follower(ggui.Sz(60, 60), green)).At(0.5, 0.5))
		return topLeft(ggui.Row(under, over).Gap(8))
	}, path(ggui.Pt(10, 10), ggui.Pt(240, 90), 16), nil))
}

// A pointer reader that draws a shadow around the dot, past the dot, and
// one inside a panel with a shadow of its own, show as painted whole.
func TestAPointerReaderWithShadows(t *testing.T) {
	sameAsWholeAlong(t, ggui.Sz(280, 160), along(func() ggui.Widget {
		lit := ggui.FromFuncs(func(c ggui.Constraints, _ ggui.Env) ggui.Size { return ggui.Sz(120, 100) }, func(dst *ggui.Canvas, r ggui.Rect) {
			dst.FillRect(r, gray)
			if p, ok := dst.Pointer(); ok && r.Contains(p) {
				dst.Shadow(ggui.Rct(p.Add(ggui.Pt(-8, -8)), ggui.Sz(16, 16)), 4, ggui.ShadowStyle{Blur: 10, Spread: 2, Color: shade})
			}
		})
		framed := ggui.Box(follower(ggui.Sz(100, 80), red)).Pad(4).Shadow(ggui.ShadowStyle{Blur: 14, Color: shade})
		return topLeft(ggui.Row(lit, framed).Gap(24))
	}, path(ggui.Pt(5, 5), ggui.Pt(250, 90), 18), nil))
}

// A pointer reader painted through a layer, at half opacity, and one under
// a BackdropBlur that reads what lies behind it.
func TestAPointerReaderInALayerAndUnderABlur(t *testing.T) {
	sameAsWholeAlong(t, ggui.Sz(280, 140), along(func() ggui.Widget {
		faded := ggui.Opacity(follower(ggui.Sz(100, 80), red), 0.5)
		glass := ggui.Box(ggui.BackdropBlur(ggui.Box().Size(60, 40), 4)).Size(60, 40)
		blurred := ggui.Stack(follower(ggui.Sz(120, 100), blue), ggui.Align(glass).At(0.5, 0.5))
		return topLeft(ggui.Row(faded, blurred).Gap(12))
	}, path(ggui.Pt(10, 10), ggui.Pt(220, 90), 16), nil))
}

// Rows that fade a highlight in as the pointer crosses them, buttons, each
// painted again as it is touched and as it fades.
func TestHoverHighlightsFadingAsThePointerCrossesRows(t *testing.T) {
	sameAsWholeAlong(t, ggui.Sz(240, 200), along(func() ggui.Widget {
		return topLeft(ggui.Column(
			ui.Tooltip(ui.Button("First", nil).Ghost(), "the first"),
			ui.Button("Second", nil).Ghost(),
			ui.Tooltip(ui.Button("Third", nil).Ghost(), "the third"),
			ui.Button("Fourth", nil).Ghost(),
		).Gap(4))
	}, path(ggui.Pt(20, 4), ggui.Pt(30, 150), 14), nil))
}

// A tooltip is painted again as the pointer moves over it, in part, until
// it opens at its delay; the frame it opens in is painted whole, as it
// opens past its anchor.
func TestATooltipFollowsThePointerInPartAndOpensWhole(t *testing.T) {
	frames := sameAsWholeAlong(t, ggui.Sz(260, 160), along(func() ggui.Widget {
		anchor := ggui.Box().Size(60, 24).Fill(blue)
		return topLeft(ggui.Column(ui.Tooltip(anchor, "A tip that runs past its anchor").Delay(500*time.Millisecond), ggui.Box().Size(200, 80).Fill(green)))
	}, path(ggui.Pt(150, 100), ggui.Pt(30, 12), 8), nil))
	if counts(frames)[ggui.FramePartial] < 6 {
		t.Fatalf("want the moves before the tooltip opened painted in part: %v", counts(frames))
	}
}

// A hover card opens an overlay as the pointer rests on its anchor: the
// frame is painted whole.
func TestAHoverCardOpeningPaintsWhole(t *testing.T) {
	sameAsWholeAlong(t, ggui.Sz(300, 220), along(func() ggui.Widget {
		card := ui.HoverCard(ggui.Box().Size(80, 24).Fill(blue), ggui.Box().Size(120, 60).Fill(red))
		return topLeft(ggui.Column(card, ggui.Box().Size(200, 60).Fill(green)).Gap(8))
	}, path(ggui.Pt(200, 150), ggui.Pt(40, 12), 6), nil))
}

// A pointer reader that also paints a signal it reads, written as the
// pointer moves, is painted whole for the write.
func TestAPointerReaderReadingASignalWrittenAsItMoves(t *testing.T) {
	var level *ggui.StateValue[float64]
	frames := sameAsWholeAlong(t, ggui.Sz(260, 120), func() (ggui.Builder, func(*ggui.WindowProbe, int) bool) {
		level = ggui.State(0.0)
		lv := level
		build, act := along(func() ggui.Widget {
			bar := ggui.FromFuncs(func(c ggui.Constraints, _ ggui.Env) ggui.Size { return ggui.Sz(100, 80) }, func(dst *ggui.Canvas, r ggui.Rect) {
				dst.FillRect(r, gray)
				dst.FillRect(ggui.Rct(r.Origin, ggui.Sz(r.Size.W*ggui.Untrack(lv.Get), 10)), blue)
				if p, ok := dst.Pointer(); ok && r.Contains(p) {
					dst.FillCircle(p, 4, red)
				}
			})
			return topLeft(ggui.Row(bar, ggui.Box().Size(100, 80).Fill(green)))
		}, path(ggui.Pt(10, 20), ggui.Pt(90, 70), 8), func(p *ggui.WindowProbe, i int) {
			if i == 3 {
				lv.Set(0.7)
			}
		})()
		return build, act
	})
	if frames[3].Painted != ggui.FrameWhole {
		t.Fatalf("the frame after the write was painted %v, want whole", frames[3].Painted)
	}
}

// The theme changing as the pointer moves paints the frame whole.
func TestAThemeChangeAsThePointerMovesPaintsWhole(t *testing.T) {
	frames := sameAsWholeAlong(t, ggui.Sz(260, 160), along(func() ggui.Widget {
		return topLeft(ggui.Column(ui.Tooltip(ui.Button("Themed", nil), "tip"), follower(ggui.Sz(100, 60), red)).Gap(8))
	}, path(ggui.Pt(10, 60), ggui.Pt(90, 100), 6), func(p *ggui.WindowProbe, i int) {
		if i == 2 {
			uitheme.Set(uitheme.Dark())
		}
	}))
	if frames[2].Painted != ggui.FrameWhole {
		t.Fatalf("the frame after the theme changed was painted %v, want whole", frames[2].Painted)
	}
}

// A signal two windows share, written in the other's frame, paints whole
// the frame of this one that a pointer move would have painted in part,
// as the windows of an App run their frames on the one goroutine.
func TestAWriteInAnotherWindowPaintsThePointersFrameWhole(t *testing.T) {
	shared := ggui.State(0.2)
	reader := ggui.FromFuncs(func(c ggui.Constraints, _ ggui.Env) ggui.Size { return ggui.Sz(100, 80) }, func(dst *ggui.Canvas, r ggui.Rect) {
		dst.FillRect(ggui.Rct(r.Origin, ggui.Sz(r.Size.W*ggui.Untrack(shared.Get), r.Size.H)), blue)
		if p, ok := dst.Pointer(); ok && r.Contains(p) {
			dst.FillCircle(p, 4, red)
		}
	})
	a := ggui.NewWindowProbe(func() ggui.Widget { return topLeft(ggui.Row(reader, ggui.Box().Size(100, 80).Fill(green))) }, ggui.Sz(220, 100))
	defer a.Close()
	b := ggui.NewWindowProbe(func() ggui.Widget { return ggui.Box().Fill(gray) }, ggui.Sz(50, 50))
	defer b.Close()
	a.Run(refresh, 30)
	b.Run(refresh, 30)
	a.Move(ggui.Pt(20, 20))
	if fr := a.Step(refresh); fr.Painted != ggui.FramePartial {
		t.Fatalf("a move alone was painted %v, want in part", fr.Painted)
	}
	b.Step(refresh)
	shared.Set(0.9)
	b.Step(refresh)
	a.Move(ggui.Pt(30, 30))
	if fr := a.Step(refresh); fr.Painted != ggui.FrameWhole {
		t.Fatalf("a move after the other window wrote what this one paints was painted %v, want whole", fr.Painted)
	}
}

// A property set between frames, as posted work or an image that finished
// loading sets one, paints the move's frame whole: one that changes the
// size asks for a layout, and one that changes only how the widget looks,
// set outside a rebuild, comes with the Repaint its documentation asks for.
func TestAPropertySetAsThePointerMovesPaintsWhole(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(b *ggui.BoxWidget, p *ggui.WindowProbe)
	}{
		{"a size", func(b *ggui.BoxWidget, _ *ggui.WindowProbe) { b.Size(80, 60) }},
		{"a color, repainted", func(b *ggui.BoxWidget, p *ggui.WindowProbe) {
			b.Fill(blue)
			p.Window().Repaint()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frames := sameAsWholeAlong(t, ggui.Sz(260, 120), func() (ggui.Builder, func(*ggui.WindowProbe, int) bool) {
				beside := ggui.Box().Size(100, 80).Fill(green)
				return along(func() ggui.Widget {
					return topLeft(ggui.Row(follower(ggui.Sz(100, 80), red), beside))
				}, path(ggui.Pt(10, 10), ggui.Pt(90, 70), 6), func(p *ggui.WindowProbe, i int) {
					if i == 2 {
						tc.set(beside, p)
					}
				})()
			})
			if frames[2].Painted != ggui.FrameWhole {
				t.Fatalf("the frame after the property was set was painted %v, want whole", frames[2].Painted)
			}
		})
	}
}
