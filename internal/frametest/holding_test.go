// Package frametest_test checks what a window's frames show, pixel by
// pixel, against what they would show painted whole. It runs inside a frame
// of the renderer, which reading an image needs, and so is a package of its
// own: the ggui package's tests run with none.
package frametest_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/gfxtest"
	"github.com/ironpark/ggui/ui"
)

func TestMain(m *testing.M) { gfxtest.MainInFrame(m) }

// refresh is a display's, the time between frames.
const refresh = 16 * time.Millisecond

// A scene is what a test runs twice: build makes the tree, with state of
// its own each time, and act changes it between frames, as a write or
// input would; it is called once, after the tree has come to rest.
type scene func() (build ggui.Builder, act func(p *ggui.WindowProbe))

// A walk is a scene that acts after each of several frames, as a pointer
// moving across a window does: act is called with how many times it was,
// and reports whether to act after the next frame too.
type walk func() (build ggui.Builder, act func(p *ggui.WindowProbe, i int) bool)

// sameAsWhole runs s in a window of size twice: once as a window paints,
// skipping, painting in part or within a boundary where it may, and once
// asked to paint every frame whole. After act, the first runs a frame each
// refresh until it comes to rest, then one at each instant a widget asked
// to be painted again at, as a delayed tooltip does, while that is within a
// second; the second runs the same frames, at the same instants. At every
// frame the two screens must be the same, pixel for pixel: what a frame
// leaves unpainted must show what painting it would have. It returns the
// first window's frames after act.
//
// The windows run one after the other, not frame by frame side by side:
// the generations a window tells a change by are its goroutine's, and the
// other's writes and layouts would reach it as its own.
func sameAsWhole(t *testing.T, size ggui.Size, s scene) []ggui.WindowFrame {
	t.Helper()
	return sameAsWholeAlong(t, size, func() (ggui.Builder, func(*ggui.WindowProbe, int) bool) {
		build, act := s()
		return build, func(p *ggui.WindowProbe, _ int) bool {
			act(p)
			return false
		}
	})
}

// sameAsWholeAlong is sameAsWhole for a walk: it acts after a frame, and
// after each frame that follows while act asks to, then runs to rest. It
// returns the first window's frames after the first act.
func sameAsWholeAlong(t *testing.T, size ggui.Size, s walk) []ggui.WindowFrame {
	t.Helper()
	build, act := s()
	kept := ggui.NewWindowProbe(build, size)

	// The frames run, as how long after the last each came and how the
	// first window painted it, with what it showed; acted are the frames
	// after which act ran.
	var (
		steps  []time.Duration
		paints []ggui.WindowFrame
		shown  [][]byte
		acted  []int
	)
	step := func(d time.Duration) ggui.WindowFrame {
		fr := kept.Step(d)
		steps, paints = append(steps, d), append(paints, fr)
		shown = append(shown, pixels(kept.Screen()))
		return fr
	}
	step(0)
	for i := 0; ; i++ {
		if fr := step(refresh); !fr.Next || i > 30 {
			break
		}
	}
	for i, more := 0, true; more; i++ {
		step(refresh)
		acted = append(acted, len(steps)-1)
		more = act(kept, i)
	}
	rested := false
	d := refresh
	for range 120 {
		fr := step(d)
		d = refresh
		if !fr.Next {
			if fr.Wake.IsZero() || fr.Wake.Sub(fr.At) > time.Second {
				rested = true
				break
			}
			d = max(fr.Wake.Sub(fr.At), time.Millisecond)
		}
	}
	frames := paints[acted[0]+1:]
	kept.Close()

	build, actWhole := s()
	whole := ggui.NewWindowProbe(build, size)
	defer whole.Close()
	n := 0
	for i, d := range steps {
		if i > 0 {
			whole.Window().Repaint()
		}
		if fr := whole.Step(d); fr.Painted != ggui.FrameWhole {
			t.Fatalf("frame %d of the window asked to paint whole was painted %v", i-acted[0]-1, fr.Painted)
		}
		if diff := screenDiff(shown[i], whole.Screen()); diff != "" {
			fr := paints[i]
			t.Fatalf("frame %d, painted %v within %v, differs from the frame painted whole: %s", i-acted[0]-1, fr.Painted, fr.Damage, diff)
		}
		if n < len(acted) && i == acted[n] {
			actWhole(whole, n)
			n++
		}
	}
	if !rested {
		t.Fatalf("the window did not come to rest in 120 frames: %v", frames)
	}
	return frames
}

// pixels reads what img shows.
func pixels(img interface {
	Bounds() image.Rectangle
	ReadPixels([]byte)
}) []byte {
	r := img.Bounds()
	p := make([]byte, 4*r.Dx()*r.Dy())
	img.ReadPixels(p)
	return p
}

// screenDiff describes where what a window showed, pa, and what b shows
// differ, or is empty where they do not.
func screenDiff(pa []byte, b interface {
	Bounds() image.Rectangle
	ReadPixels([]byte)
}) string {
	r := b.Bounds()
	pb := pixels(b)
	if len(pa) != len(pb) {
		return fmt.Sprintf("%d bytes and %d", len(pa), len(pb))
	}
	if bytes.Equal(pa, pb) {
		return ""
	}
	var where image.Rectangle
	n := 0
	for i := 0; i < len(pa); i += 4 {
		if !bytes.Equal(pa[i:i+4], pb[i:i+4]) {
			x, y := i/4%r.Dx(), i/4/r.Dx()
			where = where.Union(image.Rect(x, y, x+1, y+1))
			n++
		}
	}
	return fmt.Sprintf("%d pixels within %v", n, where)
}

// within fails t unless every frame but the first, which the write that
// started it all paints whole, was painted contained within area alone.
func within(t *testing.T, frames []ggui.WindowFrame, area ggui.Rect) {
	t.Helper()
	if len(frames) < 4 {
		t.Fatalf("%d frames, want an easing over several: %v", len(frames), frames)
	}
	for i, fr := range frames[1:] {
		if fr.Painted != ggui.FrameContained || len(fr.Damage) == 0 {
			t.Fatalf("frame %d was painted %v, want contained: %v", i+1, fr.Painted, frames)
		}
		for _, d := range fr.Damage {
			if d.Empty() || d.Intersect(area) != d {
				t.Fatalf("frame %d painted %v, want within %v", i+1, fr.Damage, area)
			}
		}
	}
}

// wholly fails t unless every frame was painted whole.
func wholly(t *testing.T, frames []ggui.WindowFrame) {
	t.Helper()
	for i, fr := range frames {
		if fr.Painted != ggui.FrameWhole {
			t.Fatalf("frame %d was painted %v, want whole: %v", i, fr.Painted, frames)
		}
	}
}

var (
	gray  = color.RGBA{0x80, 0x80, 0x80, 0xff}
	red   = color.RGBA{0xd0, 0x30, 0x30, 0xff}
	green = color.RGBA{0x30, 0xa0, 0x50, 0xff}
	blue  = color.RGBA{0x30, 0x50, 0xd0, 0xff}
	shade = color.RGBA{0, 0, 0, 0x60}
)

// disclosure is a Reveal of a block 80 by 60 above a strip that it pushes
// down as it opens, and what opens it.
func disclosure() (ggui.Widget, func(*ggui.WindowProbe)) {
	open := ggui.State(false)
	body := ggui.Box(ggui.Box().Size(60, 40).Fill(red).Shadow(ggui.ShadowStyle{Blur: 12, Spread: 4, Color: shade})).Pad(10)
	w := ggui.Column(ggui.Reveal(open, body), ggui.Box().Size(80, 20).Fill(blue))
	return w, func(*ggui.WindowProbe) { open.Set(true) }
}

func topLeft(w ggui.Widget) ggui.Widget { return ggui.Align(w).At(0, 0) }

// A disclosure easing open inside a Contain of fixed size, beside other
// content and with no Scroll anywhere, is painted within the Contain.
func TestADisclosureInAFixedContainPaintsWithinIt(t *testing.T) {
	frames := sameAsWhole(t, ggui.Sz(240, 160), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		d, act := disclosure()
		return func() ggui.Widget {
			pane := ggui.Contain(ggui.Box(d).Size(100, 120).Fill(gray))
			return topLeft(ggui.Row(pane, ggui.Box().Size(100, 120).Fill(green)))
		}, act
	})
	within(t, frames, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(100, 120)))
}

// A page whose Scroll covers the whole window holds nothing a frame can
// save painting; a fixed card inside it holds a disclosure in the card.
func TestADisclosureInAFixedCardOfAPageScrollPaintsWithinTheCard(t *testing.T) {
	frames := sameAsWhole(t, ggui.Sz(240, 200), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		d, act := disclosure()
		return func() ggui.Widget {
			card := ggui.Contain(ggui.Box(d).Size(150, 100).Fill(gray))
			// The card's own shadow lies outside it and outside what the
			// frames paint; it does not change.
			framed := ggui.Box(card).Shadow(ggui.ShadowStyle{Blur: 16, Color: shade})
			return ggui.Scroll(ggui.Column(ggui.Box().Size(200, 40).Fill(green), framed, ggui.Box().Size(200, 400).Fill(blue)))
		}, act
	})
	within(t, frames, ggui.Rct(ggui.Pt(0, 40), ggui.Sz(150, 100)))
}

// A Contain whose size follows its content holds nothing: the frame it
// was to hold is painted whole, and so is every one after, or, with a
// fixed Contain around it, they are held there. Either way the disclosure
// eases all the way open and the window comes to rest, in no more frames
// than painting all of them whole takes, but the one the first frame
// asked for to paint it whole.
func TestAContainSizedByItsContentDefersToTheOneAroundIt(t *testing.T) {
	loose := func(d ggui.Widget) ggui.Widget {
		return ggui.Column(ggui.Contain(ggui.Box(d).Fill(gray)), ggui.Box().Size(100, 30).Fill(green))
	}
	alone := sameAsWhole(t, ggui.Sz(240, 200), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		d, act := disclosure()
		return func() ggui.Widget { return topLeft(loose(d)) }, act
	})
	wholly(t, alone)

	var whole int
	held := sameAsWhole(t, ggui.Sz(240, 200), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		d, act := disclosure()
		return func() ggui.Widget {
			return topLeft(ggui.Row(ggui.Contain(ggui.Box(loose(d)).Size(120, 180)), ggui.Box().Size(100, 100).Fill(red)))
		}, act
	})
	for _, fr := range held {
		if fr.Painted == ggui.FrameWhole {
			whole++
		}
	}
	// The write, and the frame that finds the inner one sized by its
	// content.
	if whole > 2 {
		t.Fatalf("%d frames painted whole, want the inner Contain to give up once: %v", whole, held)
	}
	within(t, held[whole:], ggui.Rct(ggui.Pt(0, 0), ggui.Sz(120, 180)))
	if len(held) > len(alone)+1 {
		t.Fatalf("%d frames held, %d painted whole: giving up cost more than a frame", len(held), len(alone))
	}
}

// What a boundary holds inside it stays inside: a child running past it
// and a shadow at its edge are clipped there, and a frame it holds leaves
// what lies around it, the boundary's own frame's shadow too, as painting
// it whole would.
func TestWhatABoundaryHoldsIsClippedToIt(t *testing.T) {
	frames := sameAsWhole(t, ggui.Sz(260, 200), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		d, act := disclosure()
		return func() ggui.Widget {
			// Wider than the Contain, and shadowed at its edge.
			over := ggui.Column(d, ggui.Box().Size(160, 20).Fill(red).Shadow(ggui.ShadowStyle{Blur: 20, Spread: 8, Color: shade}))
			pane := ggui.Box(ggui.Contain(ggui.Box(over).Size(110, 140).Fill(gray))).Pad(20).Shadow(ggui.ShadowStyle{Offset: ggui.Pt(6, 6), Blur: 18, Color: shade})
			return topLeft(ggui.Row(pane, ggui.Box().Size(60, 60).Fill(green)))
		}, act
	})
	within(t, frames, ggui.Rct(ggui.Pt(20, 20), ggui.Sz(110, 140)))
}

// Without a boundary, a disclosure in a box with a shadow and a child
// running past it is painted whole, and shows as such.
func TestADisclosureWithNoBoundaryPaintsWhole(t *testing.T) {
	frames := sameAsWhole(t, ggui.Sz(260, 200), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		d, act := disclosure()
		return func() ggui.Widget {
			over := ggui.Column(d, ggui.Box().Size(160, 20).Fill(red))
			return topLeft(ggui.Box(ggui.Box(over).Size(110, 140).Fill(gray)).Pad(20).Shadow(ggui.ShadowStyle{Blur: 18, Color: shade}))
		}, act
	})
	wholly(t, frames)
}

// A Scroll inside a Contain holds a disclosure in its viewport, the
// nearest boundary.
func TestAScrollInAContainHoldsWithinItsViewport(t *testing.T) {
	frames := sameAsWhole(t, ggui.Sz(240, 200), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		d, act := disclosure()
		return func() ggui.Widget {
			inner := ggui.Box(ggui.Scroll(ggui.Column(d, ggui.Box().Size(80, 300).Fill(green)))).Size(100, 80)
			pane := ggui.Contain(ggui.Box(ggui.Column(ggui.Box().Size(100, 40).Fill(blue), inner)).Size(120, 160).Fill(gray))
			return topLeft(ggui.Row(pane, ggui.Box().Size(100, 100).Fill(red)))
		}, act
	})
	within(t, frames, ggui.Rct(ggui.Pt(0, 40), ggui.Sz(100, 80)))
}

// A Contain nested in another holds what changes inside it; one inside a
// Contain sized by its content passes it to the one around.
func TestNestedContainsHoldAtTheNearest(t *testing.T) {
	frames := sameAsWhole(t, ggui.Sz(240, 200), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		d, act := disclosure()
		return func() ggui.Widget {
			inner := ggui.Contain(ggui.Box(d).Size(100, 110).Fill(gray))
			outer := ggui.Contain(ggui.Box(ggui.Column(ggui.Box().Size(100, 30).Fill(blue), inner)).Pad(10).Size(140, 180).Fill(green))
			return topLeft(ggui.Row(outer, ggui.Box().Size(60, 60).Fill(red)))
		}, act
	})
	within(t, frames, ggui.Rct(ggui.Pt(10, 40), ggui.Sz(100, 110)))
}

// A Contain under a Transform cannot tell where it is in the window, and
// holds nothing.
func TestAContainUnderATransformHoldsNothing(t *testing.T) {
	frames := sameAsWhole(t, ggui.Sz(240, 200), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		d, act := disclosure()
		return func() ggui.Widget {
			pane := ggui.Transform(ggui.Contain(ggui.Box(d).Size(100, 120).Fill(gray))).Translate(7, 5)
			return topLeft(ggui.Row(pane, ggui.Box().Size(60, 60).Fill(red)))
		}, act
	})
	wholly(t, frames)
}

// A Collapsible in a pane of a Resizable eases open within the pane.
func TestACollapsibleInAResizablePanePaintsWithinIt(t *testing.T) {
	frames := sameAsWhole(t, ggui.Sz(300, 200), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		open := ggui.State(false)
		fraction := ggui.State(0.5)
		return func() ggui.Widget {
				pane := ui.Collapsible(open, "More", ggui.Box().Size(100, 60).Fill(red))
				return ui.Resizable(fraction, pane, ggui.Box().Fill(green))
			}, func(*ggui.WindowProbe) {
				open.Set(true)
			}
	})
	// The first pane, less the divider's half.
	within(t, frames, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(146, 200)))
}

// A horizontal scroll as tall as its content, which grows as it moves,
// moves what lies below it: the frames it was to hold are painted whole,
// and those that leave its height as it was within the scroll around it.
func TestAScrollSizedByItsContentShowsAsPaintedWhole(t *testing.T) {
	frames := sameAsWhole(t, ggui.Sz(200, 100), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		var sc ggui.ScrollController
		content := ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
			vp, _ := ggui.ScrollViewport(env)
			return ggui.Sz(400, 20+vp.Offset/10)
		}, func(dst *ggui.Canvas, r ggui.Rect) {
			dst.FillRect(r, red)
			dst.FillRect(ggui.Rct(r.Origin.Add(ggui.Pt(r.Size.W/2, 0)), ggui.Sz(r.Size.W/2, r.Size.H)), blue)
		})
		return func() ggui.Widget {
			inner := ggui.Scroll(content).Horizontal().Controller(&sc)
			page := ggui.Scroll(ggui.Column(inner, ggui.Box().Size(100, 400).Fill(green)))
			return ggui.Row(ggui.Box(page).Size(100, 100), ggui.Box().Size(100, 100).Fill(gray))
		}, func(*ggui.WindowProbe) { sc.ScrollTo(300) }
	})
	if len(frames) < 4 {
		t.Fatalf("%d frames, want an eased scroll over several", len(frames))
	}
}

// A write that opens a disclosure in a Contain and moves the Contain as
// well is painted whole. The disclosure asks for the next frame as it is
// laid out, before the Contain is painted where it moved to: that frame is
// held where the Contain was and where it is now, and those after where
// it is.
func TestAContainMovedAsItsDisclosureOpensHoldsWhereItIsNow(t *testing.T) {
	frames := sameAsWhole(t, ggui.Sz(240, 320), func() (ggui.Builder, func(*ggui.WindowProbe)) {
		d, open := disclosure()
		above := ggui.State(10.0)
		return func() ggui.Widget {
				pane := ggui.Contain(ggui.Box(d).Size(100, 120).Fill(gray))
				return topLeft(ggui.Column(ggui.Box().Width(100).BindHeight(above).Fill(green), pane))
			}, func(p *ggui.WindowProbe) {
				above.Set(150)
				open(p)
			}
	})
	was, now := ggui.Rct(ggui.Pt(0, 10), ggui.Sz(100, 120)), ggui.Rct(ggui.Pt(0, 150), ggui.Sz(100, 120))
	if fr := frames[1]; fr.Painted != ggui.FrameContained || len(fr.Damage) != 1 || fr.Damage[0] != was.Cover(now) {
		t.Fatalf("the frame after the move was painted %v within %v, want contained where the Contain was and is", fr.Painted, fr.Damage)
	}
	within(t, frames[1:], now)
}

// Boundaries nested three deep, a Scroll and Contains in either order,
// hold what changes at the innermost.
func TestBoundariesNestedThreeDeepHoldAtTheInnermost(t *testing.T) {
	t.Run("a Contain in a Scroll in a Contain", func(t *testing.T) {
		frames := sameAsWhole(t, ggui.Sz(240, 240), func() (ggui.Builder, func(*ggui.WindowProbe)) {
			d, act := disclosure()
			return func() ggui.Widget {
				inner := ggui.Contain(ggui.Box(d).Size(100, 110).Fill(gray))
				scroll := ggui.Box(ggui.Scroll(ggui.Column(ggui.Box().Size(120, 30).Fill(blue), inner, ggui.Box().Size(120, 300).Fill(green)))).Size(120, 180)
				return topLeft(ggui.Row(ggui.Contain(ggui.Box(scroll).Pad(10).Fill(red)), ggui.Box().Size(60, 60).Fill(blue)))
			}, act
		})
		within(t, frames, ggui.Rct(ggui.Pt(10, 40), ggui.Sz(100, 110)))
	})
	t.Run("a Scroll in a Contain in a Scroll", func(t *testing.T) {
		frames := sameAsWhole(t, ggui.Sz(240, 240), func() (ggui.Builder, func(*ggui.WindowProbe)) {
			d, act := disclosure()
			return func() ggui.Widget {
				inner := ggui.Box(ggui.Scroll(ggui.Column(d, ggui.Box().Size(80, 300).Fill(green)))).Size(100, 80)
				card := ggui.Contain(ggui.Box(ggui.Column(ggui.Box().Size(100, 20).Fill(blue), inner)).Size(140, 120).Fill(gray))
				return ggui.Scroll(ggui.Column(ggui.Box().Size(200, 30).Fill(red), card, ggui.Box().Size(200, 400).Fill(blue)))
			}, act
		})
		within(t, frames, ggui.Rct(ggui.Pt(0, 50), ggui.Sz(100, 80)))
	})
}
