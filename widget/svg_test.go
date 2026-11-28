package widget

import (
	"errors"
	"image"
	"image/color"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/inspect"
	"github.com/ironpark/ggui/internal/framestats"
	"github.com/ironpark/svgx"
)

const testRectSVG = `<svg viewBox="0 0 200 100" xmlns="http://www.w3.org/2000/svg">` +
	`<rect id="box" x="10" y="10" width="180" height="80" fill="#ff0000" stroke="#0000ff" stroke-width="4"/>` +
	`</svg>`

const testSquareSVG = `<svg viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg">` +
	`<rect id="sq" x="0" y="0" width="100" height="100" fill="#00ff00"/>` +
	`</svg>`

const testCurrentColorSVG = `<svg viewBox="0 0 40 40" xmlns="http://www.w3.org/2000/svg">` +
	`<rect id="cc" x="0" y="0" width="40" height="40" fill="currentColor"/>` +
	`</svg>`

const testSMILSVG = `<svg viewBox="0 0 100 100" xmlns="http://www.w3.org/2000/svg">` +
	`<rect id="mover" x="0" y="40" width="20" height="20" fill="#ff0000">` +
	`<animate attributeName="x" from="0" to="80" dur="1s" fill="freeze"/>` +
	`</rect></svg>`

func mustParseSVG(t testing.TB, src string) *svgx.Document {
	t.Helper()
	doc, err := ParseSVG([]byte(src))
	if err != nil {
		t.Fatalf("ParseSVG failed: %v", err)
	}
	return doc
}

type testEnvColor struct {
	key ggui.EnvKey[color.Color]
	def color.Color
}

func (c testEnvColor) RGBA() (r, g, b, a uint32) { return c.def.RGBA() }
func (c testEnvColor) Resolve(env ggui.Env) color.Color {
	if v, ok := env.Get(c.key); ok && v != nil {
		return v
	}
	return c.def
}

// layoutCounter wraps a widget and counts Layout invocations.
type layoutCounter struct {
	child ggui.Widget
	count int
}

func (l *layoutCounter) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	l.count++
	return l.child.Layout(c, env)
}

func (l *layoutCounter) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(l.child, r) }

type paintFlagWidget struct {
	child    ggui.Widget
	painting *bool
}

func (p *paintFlagWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return p.child.Layout(c, env)
}

func (p *paintFlagWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	*p.painting = true
	dst.Paint(p.child, r)
	*p.painting = false
}

// clipWidget clips painting of child to clipRect (or empty when hidden) to test
// offscreen pause/resume cleanly without relying on scroll bar geometry.
type clipWidget struct {
	child  ggui.Widget
	hidden bool
}

func (c *clipWidget) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	return c.child.Layout(cs, env)
}

func (c *clipWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if c.hidden {
		// Clip to a disjoint rect so dst.Visible(r) is empty.
		off := ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W+1000, r.Origin.Y+r.Size.H+1000), ggui.Sz(10, 10))
		dst.Clip(off).Paint(c.child, r)
		return
	}
	dst.Paint(c.child, r)
}

func TestParseSVGAndLoadSVGFile(t *testing.T) {
	t.Parallel()

	doc, err := ParseSVG([]byte(testRectSVG))
	if err != nil {
		t.Fatalf("ParseSVG valid: %v", err)
	}
	vb := doc.ViewBox()
	if vb.X != 0 || vb.Y != 0 || vb.W != 200 || vb.H != 100 {
		t.Fatalf("ViewBox = (%v,%v,%v,%v), want (0,0,200,100)", vb.X, vb.Y, vb.W, vb.H)
	}

	for _, bad := range []string{
		"",
		"<not-svg",
		`<svg xmlns="http://www.w3.org/2000/svg"/>`,
		`<svg viewBox="0 0 0 24" xmlns="http://www.w3.org/2000/svg"/>`,
		`<svg viewBox="0 0 24 -10" xmlns="http://www.w3.org/2000/svg"/>`,
	} {
		got, err := ParseSVG([]byte(bad))
		if err == nil || got != nil {
			t.Errorf("ParseSVG(%q) = (%v, %v), want error", bad, got, err)
			continue
		}
		if !strings.HasPrefix(err.Error(), "ggui: parse SVG: ") {
			t.Errorf("ParseSVG(%q) error = %q, want prefix %q", bad, err.Error(), "ggui: parse SVG: ")
		}
	}

	dir := t.TempDir()
	goodPath := filepath.Join(dir, "good.svg")
	if err := os.WriteFile(goodPath, []byte(testRectSVG), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSVGFile(goodPath)
	if err != nil {
		t.Fatalf("LoadSVGFile valid: %v", err)
	}
	if vb := loaded.ViewBox(); vb.W != 200 || vb.H != 100 {
		t.Fatalf("loaded ViewBox = %vx%v, want 200x100", vb.W, vb.H)
	}

	if _, err := LoadSVGFile(filepath.Join(dir, "missing.svg")); !errors.Is(err, fs.ErrNotExist) || !strings.HasPrefix(err.Error(), "ggui: load SVG: ") {
		t.Fatalf("missing file err = %v, want load SVG error wrapping fs.ErrNotExist", err)
	}

	badPath := filepath.Join(dir, "bad.svg")
	if err := os.WriteFile(badPath, []byte(`<svg viewBox="0 0 0 0"/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSVGFile(badPath); err == nil || !strings.HasPrefix(err.Error(), "ggui: parse SVG: ") {
		t.Fatalf("bad file err = %v, want parse SVG error", err)
	}
}

func TestSVGLayoutAspectAndPixelCap(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testRectSVG) // 200x100 (2:1)
	cases := []struct {
		name string
		w    *SVGWidget
		c    ggui.Constraints
		want ggui.Size
	}{
		{"natural", SVG(doc), ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded)), ggui.Sz(200, 100)},
		{"shrinks to width", SVG(doc), ggui.Loose(ggui.Sz(100, ggui.Unbounded)), ggui.Sz(100, 50)},
		{"shrinks to height", SVG(doc), ggui.Loose(ggui.Sz(ggui.Unbounded, 25)), ggui.Sz(50, 25)},
		{"width follows height", SVG(doc).Height(50), ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded)), ggui.Sz(100, 50)},
		{"height follows width", SVG(doc).Width(50), ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded)), ggui.Sz(50, 25)},
		{"fixed size", SVG(doc).Size(30, 30), ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded)), ggui.Sz(30, 30)},
		{"nil doc natural", SVG(nil), ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded)), ggui.Sz(0, 0)},
		{"nil doc fixed", SVG(nil).Size(40, 20), ggui.Loose(ggui.Sz(ggui.Unbounded, ggui.Unbounded)), ggui.Sz(40, 20)},
	}
	for _, tc := range cases {
		if got := tc.w.Layout(tc.c, ggui.Env{}); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}

	// Nil doc paints nothing without panicking.
	p := ggui.NewProbe(SVG(nil).Size(40, 40), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()

	// Device pixel capping preserves aspect ratio up to svgMaxSide=1024.
	if pw, ph := svgPixelSize(ggui.Sz(2000, 1000), 1); pw != 1024 || ph != 512 {
		t.Fatalf("svgPixelSize(2000x1000, 1) = %dx%d, want 1024x512", pw, ph)
	}
	if pw, ph := svgPixelSize(ggui.Sz(600, 300), 2); pw != 1024 || ph != 512 {
		t.Fatalf("svgPixelSize(600x300, 2) = %dx%d, want 1024x512", pw, ph)
	}
	if pw, ph := svgPixelSize(ggui.Sz(100, 50), 2); pw != 200 || ph != 100 {
		t.Fatalf("svgPixelSize(100x50, 2) = %dx%d, want 200x100", pw, ph)
	}
}

func TestSVGAltSemantics(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testRectSVG)

	p1 := ggui.NewProbe(SVG(doc).Size(60, 30).Alt("Brand logo"), ggui.Sz(100, 100))
	defer p1.Close()
	sem1 := p1.Semantics().String()
	if !strings.Contains(sem1, "Brand logo") {
		t.Fatalf("semantics missing Alt label: %s", sem1)
	}

	p2 := ggui.NewProbe(SVG(doc).Size(60, 30), ggui.Sz(100, 100))
	defer p2.Close()
	sem2 := p2.Semantics().String()
	if strings.Contains(sem2, string(ggui.RoleImage)) {
		t.Fatalf("decorative SVG without Alt leaked RoleImage into semantics: %s", sem2)
	}
}

func TestSVGAutoPlayDefaultAndDisable(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testRectSVG)
	tl := &svgx.Timeline{Span: time.Second, End: time.Second}

	// Animated document auto-plays by default.
	var c1 SVGController
	w1 := SVG(doc).Controller(&c1)
	w1.testTimeline = tl
	p1 := ggui.NewProbe(w1, ggui.Sz(100, 100))
	defer p1.Close()
	p1.Frame()
	if !c1.Playing() {
		t.Fatal("animated SVG did not AutoPlay by default")
	}
	p1.Advance(300 * time.Millisecond)
	if got := c1.Time(); got < 250*time.Millisecond || got > 350*time.Millisecond {
		t.Fatalf("after 300ms: Time() = %v, want ~300ms", got)
	}

	// AutoPlay(false) keeps it paused at 0.
	var c2 SVGController
	w2 := SVG(doc).Controller(&c2).AutoPlay(false)
	w2.testTimeline = tl
	p2 := ggui.NewProbe(w2, ggui.Sz(100, 100))
	defer p2.Close()
	p2.Frame()
	if c2.Playing() {
		t.Fatal("AutoPlay(false) should not start playing")
	}
	p2.Advance(300 * time.Millisecond)
	if c2.Time() != 0 {
		t.Fatalf("AutoPlay(false) advanced to %v, want 0", c2.Time())
	}
}

func TestSVGControllerTransportLoopAlternateSpeedAndOnDone(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testRectSVG)
	tl := &svgx.Timeline{Span: time.Second, End: time.Second}

	// Zero-value controller defaults before attach.
	var ctrl SVGController
	if ctrl.Playing() || ctrl.Time() != 0 || ctrl.Progress() != 0 || ctrl.Duration() != 0 {
		t.Fatalf("zero controller state = playing:%v time:%v prog:%v dur:%v",
			ctrl.Playing(), ctrl.Time(), ctrl.Progress(), ctrl.Duration())
	}

	// SeekProgress before attach stays pending until attach.
	ctrl.SeekProgress(0.25)
	if ctrl.Progress() != 0.25 {
		t.Fatalf("pending Progress() = %v, want 0.25", ctrl.Progress())
	}

	doneCount := 0
	var doneMidPaint bool
	painting := false
	w := SVG(doc).Controller(&ctrl).AutoPlay(false).OnDone(func() {
		if painting {
			doneMidPaint = true
		}
		doneCount++
	})
	w.testTimeline = tl

	// Wrap to verify OnDone is posted (not invoked synchronously inside Paint).
	wrapper := &paintFlagWidget{child: w, painting: &painting}
	p := ggui.NewProbe(wrapper, ggui.Sz(100, 100))
	defer p.Close()

	p.Frame()
	if ctrl.Duration() != time.Second {
		t.Fatalf("Duration() = %v, want 1s", ctrl.Duration())
	}
	if ctrl.Time() != 250*time.Millisecond {
		t.Fatalf("pending SeekProgress(0.25) applied as %v, want 250ms", ctrl.Time())
	}
	if ctrl.Playing() {
		t.Fatal("SeekProgress should preserve paused state")
	}

	// Play from 250ms -> finishes after 750ms at 1s.
	ctrl.Play()
	if !ctrl.Playing() {
		t.Fatal("Play() did not set Playing()")
	}
	p.Frame() // establish anchor
	p.Advance(500 * time.Millisecond)
	if got := ctrl.Time(); got < 700*time.Millisecond || got > 800*time.Millisecond {
		t.Fatalf("mid-play Time() = %v, want ~750ms", got)
	}
	p.Advance(400 * time.Millisecond) // crosses 1s
	if ctrl.Playing() || ctrl.Time() != time.Second {
		t.Fatalf("at end: playing=%v time=%v, want false, 1s", ctrl.Playing(), ctrl.Time())
	}
	// Posted callback runs on the next frame tick, never mid-Paint.
	p.Frame()
	if doneMidPaint {
		t.Fatal("OnDone ran synchronously mid-Paint instead of via Host.Post")
	}
	if doneCount != 1 {
		t.Fatalf("doneCount = %d, want 1", doneCount)
	}
	// Extra frames do not refire OnDone.
	p.Advance(200 * time.Millisecond)
	p.Frame()
	if doneCount != 1 {
		t.Fatalf("doneCount refired: %d", doneCount)
	}

	// Pause & Seek preserve play state; Stop pauses and rewinds to 0 without OnDone.
	ctrl.Play() // restarts from 0 because it was at end!
	p.Frame()
	if ctrl.Time() != 0 || !ctrl.Playing() {
		t.Fatalf("Play() from end: time=%v playing=%v, want 0, true", ctrl.Time(), ctrl.Playing())
	}
	ctrl.Seek(400 * time.Millisecond)
	if !ctrl.Playing() || ctrl.Time() != 400*time.Millisecond {
		t.Fatalf("Seek while playing: playing=%v time=%v", ctrl.Playing(), ctrl.Time())
	}
	ctrl.Pause()
	if ctrl.Playing() {
		t.Fatal("Pause did not stop playback")
	}
	p.Advance(200 * time.Millisecond)
	if ctrl.Time() != 400*time.Millisecond {
		t.Fatalf("time moved while paused: %v", ctrl.Time())
	}
	ctrl.Stop()
	p.Frame()
	if ctrl.Playing() || ctrl.Time() != 0 || doneCount != 1 {
		t.Fatalf("Stop: playing=%v time=%v done=%d", ctrl.Playing(), ctrl.Time(), doneCount)
	}

	// Reverse from 0 starts at span (1s) and plays backward to 0.
	ctrl.Reverse()
	p.Frame()
	if !ctrl.Playing() || ctrl.Time() != time.Second {
		t.Fatalf("Reverse from 0: playing=%v time=%v, want true, 1s", ctrl.Playing(), ctrl.Time())
	}
	p.Advance(400 * time.Millisecond)
	if got := ctrl.Time(); got < 550*time.Millisecond || got > 650*time.Millisecond {
		t.Fatalf("mid-reverse Time() = %v, want ~600ms", got)
	}
	p.Advance(700 * time.Millisecond)
	p.Frame()
	if ctrl.Playing() || ctrl.Time() != 0 || doneCount != 2 {
		t.Fatalf("end of reverse: playing=%v time=%v done=%d", ctrl.Playing(), ctrl.Time(), doneCount)
	}

	// OnDone replaces single slot and nil clears.
	replaced := 0
	w.OnDone(func() { replaced++ })
	ctrl.Play()
	p.Frame()
	p.Advance(1100 * time.Millisecond)
	p.Frame()
	if doneCount != 2 || replaced != 1 {
		t.Fatalf("OnDone replace: old=%d new=%d, want 2, 1", doneCount, replaced)
	}
	w.OnDone(nil)
	ctrl.Play()
	p.Frame()
	p.Advance(1100 * time.Millisecond)
	p.Frame()
	if replaced != 1 {
		t.Fatalf("OnDone(nil) did not clear callback: replaced=%d", replaced)
	}
}

func TestSVGLoopAlternateAndSpeed(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testRectSVG)
	tl := &svgx.Timeline{Span: time.Second, End: time.Second}

	// Loop(2) + Alternate(true) + Speed(2): 2 passes at 2x = 1s wall time,
	// ends at t=0 after ping-ponging 0 -> 1s -> 0.
	var ctrl SVGController
	done := 0
	w := SVG(doc).Controller(&ctrl).Loop(2).Alternate(true).Speed(2).OnDone(func() { done++ })
	w.testTimeline = tl
	p := ggui.NewProbe(w, ggui.Sz(100, 100))
	defer p.Close()

	p.Frame() // anchor at t=0
	p.Advance(250 * time.Millisecond)
	// 250ms * 2 = 500ms forward.
	if got := ctrl.Time(); got < 450*time.Millisecond || got > 550*time.Millisecond {
		t.Fatalf("pass 1 mid: %v, want ~500ms", got)
	}
	p.Advance(400 * time.Millisecond)
	// Total 650ms * 2 = 1300ms -> bounced at 1000ms -> 700ms heading backward.
	if got := ctrl.Time(); got < 650*time.Millisecond || got > 750*time.Millisecond {
		t.Fatalf("pass 2 bouncing back: %v, want ~700ms", got)
	}
	p.Advance(400 * time.Millisecond)
	p.Frame()
	if ctrl.Playing() || ctrl.Time() != 0 || done != 1 {
		t.Fatalf("after 2 alternate passes: playing=%v time=%v done=%d", ctrl.Playing(), ctrl.Time(), done)
	}

	// Loop(-1) loops forever and never fires OnDone.
	var forever SVGController
	foreverDone := 0
	wf := SVG(doc).Controller(&forever).Loop(-1).Alternate(false).OnDone(func() { foreverDone++ })
	wf.testTimeline = tl
	pf := ggui.NewProbe(wf, ggui.Sz(100, 100))
	defer pf.Close()
	pf.Frame()
	pf.Advance(3300 * time.Millisecond)
	pf.Frame()
	if !forever.Playing() || foreverDone != 0 {
		t.Fatalf("Loop(-1): playing=%v done=%d, want true, 0", forever.Playing(), foreverDone)
	}
	if got := forever.Time(); got < 250*time.Millisecond || got > 350*time.Millisecond {
		t.Fatalf("Loop(-1) after 3.3s: time=%v, want ~300ms", got)
	}

	// Negative speed clamps to 0 and freezes playback position.
	var neg SVGController
	wn := SVG(doc).Controller(&neg).Speed(-3)
	wn.testTimeline = tl
	pn := ggui.NewProbe(wn, ggui.Sz(100, 100))
	defer pn.Close()
	pn.Frame()
	pn.Advance(500 * time.Millisecond)
	if neg.Time() != 0 {
		t.Fatalf("Speed(-3) moved playhead to %v, want 0", neg.Time())
	}
}

func TestSVGBindTimeProgressAndControllerTakeover(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testRectSVG)
	tl := &svgx.Timeline{Span: 2 * time.Second, End: 2 * time.Second}

	tSig := ggui.State(300 * time.Millisecond)
	var ctrl SVGController
	w := SVG(doc).Controller(&ctrl).BindTime(tSig)
	w.testTimeline = tl
	p := ggui.NewProbe(w, ggui.Sz(100, 100))
	defer p.Close()

	p.Frame()
	if ctrl.Time() != 300*time.Millisecond || ctrl.Playing() {
		t.Fatalf("BindTime frame 1: time=%v playing=%v, want 300ms, false", ctrl.Time(), ctrl.Playing())
	}
	tSig.Set(800 * time.Millisecond)
	p.Frame()
	if ctrl.Time() != 800*time.Millisecond {
		t.Fatalf("BindTime frame 2: time=%v, want 800ms", ctrl.Time())
	}

	// Any transport call takes playhead back from BindTime!
	ctrl.Play()
	p.Frame()
	tSig.Set(100 * time.Millisecond) // ignored now that controller took over
	p.Advance(400 * time.Millisecond)
	if got := ctrl.Time(); got < 1150*time.Millisecond || got > 1250*time.Millisecond {
		t.Fatalf("after ctrl.Play() takeover: time=%v, want ~1.2s (not 100ms)", got)
	}

	// BindProgress maps [0, 1] onto Timeline.Span.
	pSig := ggui.State(0.5)
	var ctrl2 SVGController
	w2 := SVG(doc).Controller(&ctrl2).BindProgress(pSig)
	w2.testTimeline = tl
	p2 := ggui.NewProbe(w2, ggui.Sz(100, 100))
	defer p2.Close()
	p2.Frame()
	if ctrl2.Time() != time.Second || math.Abs(ctrl2.Progress()-0.5) > 0.01 {
		t.Fatalf("BindProgress(0.5): time=%v prog=%v, want 1s, 0.5", ctrl2.Time(), ctrl2.Progress())
	}
	pSig.Set(0.75)
	p2.Frame()
	if ctrl2.Time() != 1500*time.Millisecond {
		t.Fatalf("BindProgress(0.75): time=%v, want 1.5s", ctrl2.Time())
	}
	// Seek takes over from BindProgress.
	ctrl2.Seek(200 * time.Millisecond)
	pSig.Set(0.9)
	p2.Frame()
	if ctrl2.Time() != 200*time.Millisecond {
		t.Fatalf("after ctrl.Seek takeover: time=%v, want 200ms", ctrl2.Time())
	}
}

func TestSVGOffscreenPauseAndResume(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testRectSVG)
	tl := &svgx.Timeline{Span: 10 * time.Second, End: 10 * time.Second}

	var ctrl SVGController
	w := SVG(doc).Controller(&ctrl)
	w.testTimeline = tl
	clip := &clipWidget{child: w}

	target := ggfx.NewImage(100, 100)
	defer target.Deallocate()
	p := ggui.NewProbe(clip, ggui.Sz(100, 100))
	defer p.Close()
	p.RenderTo(target)

	p.Frame() // anchor at t=0
	p.Advance(500 * time.Millisecond)
	pausedAt := ctrl.Time()
	if pausedAt < 450*time.Millisecond || pausedAt > 550*time.Millisecond {
		t.Fatalf("visible playhead = %v, want ~500ms", pausedAt)
	}
	rastersBeforeHide := w.slot.surface.rasterCount

	// Move offscreen and advance 5 seconds: playhead freezes and no rasterization occurs.
	clip.hidden = true
	p.Advance(2 * time.Second)
	p.Advance(3 * time.Second)
	if ctrl.Time() != pausedAt {
		t.Fatalf("offscreen playhead moved from %v to %v", pausedAt, ctrl.Time())
	}
	if w.slot.surface.rasterCount != rastersBeforeHide {
		t.Fatalf("rasterized offscreen: before=%d after=%d", rastersBeforeHide, w.slot.surface.rasterCount)
	}

	// Return onscreen: first visible frame resumes at pausedAt without jumping 5 seconds!
	clip.hidden = false
	p.Advance(1 * time.Second)
	if ctrl.Time() != pausedAt {
		t.Fatalf("first visible frame jumped from %v to %v", pausedAt, ctrl.Time())
	}
	// Subsequent 300ms advances smoothly from pausedAt.
	p.Advance(300 * time.Millisecond)
	want := pausedAt + 300*time.Millisecond
	if got := ctrl.Time(); got < want-50*time.Millisecond || got > want+50*time.Millisecond {
		t.Fatalf("resumed playhead = %v, want ~%v", got, want)
	}
}

func TestSVGReducedMotionAndReducedMotionAt(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testRectSVG)

	// Finite doc under ReducedMotion jumps to Span and fires OnDone once.
	var cFinite SVGController
	done := 0
	wFinite := SVG(doc).Controller(&cFinite).OnDone(func() { done++ })
	wFinite.testTimeline = &svgx.Timeline{Span: 2 * time.Second, End: 2 * time.Second}
	p1 := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, wFinite), ggui.Sz(100, 100))
	defer p1.Close()
	p1.Frame()
	p1.Frame()
	if cFinite.Playing() || cFinite.Time() != 2*time.Second || done != 1 {
		t.Fatalf("reduced finite: playing=%v time=%v done=%d, want false, 2s, 1",
			cFinite.Playing(), cFinite.Time(), done)
	}
	p1.Advance(500 * time.Millisecond)
	p1.Frame()
	if done != 1 {
		t.Fatalf("reduced finite fired OnDone more than once: %d", done)
	}

	// Infinite doc under ReducedMotion freezes at 0 without firing OnDone.
	var cInf SVGController
	infDone := 0
	wInf := SVG(doc).Controller(&cInf).OnDone(func() { infDone++ })
	wInf.testTimeline = &svgx.Timeline{Span: 2 * time.Second, Infinite: true, Loop: 2 * time.Second}
	p2 := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, wInf), ggui.Sz(100, 100))
	defer p2.Close()
	p2.Frame()
	p2.Advance(1 * time.Second)
	p2.Frame()
	if cInf.Playing() || cInf.Time() != 0 || infDone != 0 {
		t.Fatalf("reduced infinite: playing=%v time=%v done=%d, want false, 0, 0",
			cInf.Playing(), cInf.Time(), infDone)
	}

	// Explicit ReducedMotionAt sets the resting timestamp AND still fires OnDone once on finite docs.
	var cAt SVGController
	atDone := 0
	wAt := SVG(doc).Controller(&cAt).ReducedMotionAt(750 * time.Millisecond).OnDone(func() { atDone++ })
	wAt.testTimeline = &svgx.Timeline{Span: 2 * time.Second, End: 2 * time.Second}
	p3 := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, wAt), ggui.Sz(100, 100))
	defer p3.Close()
	p3.Frame()
	p3.Frame()
	if cAt.Playing() || cAt.Time() != 750*time.Millisecond || atDone != 1 {
		t.Fatalf("ReducedMotionAt finite: playing=%v time=%v done=%d, want false, 750ms, 1",
			cAt.Playing(), cAt.Time(), atDone)
	}
	p3.Advance(500 * time.Millisecond)
	p3.Frame()
	if atDone != 1 {
		t.Fatalf("ReducedMotionAt finite fired OnDone more than once: %d", atDone)
	}

	// Explicit ReducedMotionAt on an infinite doc freezes at t without firing OnDone.
	var cAtInf SVGController
	atInfDone := 0
	wAtInf := SVG(doc).Controller(&cAtInf).ReducedMotionAt(750 * time.Millisecond).OnDone(func() { atInfDone++ })
	wAtInf.testTimeline = &svgx.Timeline{Span: 2 * time.Second, Infinite: true, Loop: 2 * time.Second}
	p4 := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, wAtInf), ggui.Sz(100, 100))
	defer p4.Close()
	p4.Frame()
	p4.Frame()
	if cAtInf.Playing() || cAtInf.Time() != 750*time.Millisecond || atInfDone != 0 {
		t.Fatalf("ReducedMotionAt infinite: playing=%v time=%v done=%d, want false, 750ms, 0",
			cAtInf.Playing(), cAtInf.Time(), atInfDone)
	}
}

func TestSVGMaxFPSPacing(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testRectSVG)
	tl := &svgx.Timeline{Span: 10 * time.Second, End: 10 * time.Second}

	// Pin clock to an exact multiple of 100ms (10 FPS step).
	now := time.Unix(1000, 0)
	restore := ggui.SetClock(func() time.Time { return now })
	defer restore()

	var ctrl SVGController
	w := SVG(doc).Controller(&ctrl).MaxFPS(10)
	w.testTimeline = tl
	target := ggfx.NewImage(100, 100)
	defer target.Deallocate()

	p := ggui.NewProbe(w, ggui.Sz(100, 100))
	defer p.Close()
	p.RenderTo(target)

	p.Frame() // t=0, raster #1
	if w.slot.surface.rasterCount != 1 {
		t.Fatalf("initial rasterCount = %d, want 1", w.slot.surface.rasterCount)
	}

	// Advances within the same 100ms bucket do not advance time or re-rasterize.
	for range 4 {
		now = now.Add(20 * time.Millisecond) // 20, 40, 60, 80ms
		p.Frame()
	}
	if ctrl.Time() != 0 || w.slot.surface.rasterCount != 1 {
		t.Fatalf("within 100ms bucket: time=%v rasters=%d, want 0, 1", ctrl.Time(), w.slot.surface.rasterCount)
	}

	// Crossing 100ms advances by 100ms and rasterizes once.
	now = now.Add(20 * time.Millisecond) // 100ms
	p.Frame()
	if ctrl.Time() != 100*time.Millisecond || w.slot.surface.rasterCount != 2 {
		t.Fatalf("after 100ms boundary: time=%v rasters=%d, want 100ms, 2", ctrl.Time(), w.slot.surface.rasterCount)
	}
}

func TestSVGNodeOverridesBindingsAndZeroLayoutRemeasure(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testRectSVG)

	colorKey := ggui.NewEnvKey[color.Color]("test-svg-color")
	themeCol := testEnvColor{key: colorKey, def: color.RGBA{R: 10, G: 20, B: 30, A: 255}}

	rot := ggui.Tween(0.0, 200*time.Millisecond)
	off := ggui.State(ggui.Pt(0, 0))
	op := ggui.State(1.0)
	vis := ggui.State(true)
	fill := ggui.State[color.Color](color.RGBA{R: 255, G: 0, B: 0, A: 255})
	stroke := ggui.State[color.Color](themeCol)
	sw := ggui.State(2.0)
	tStart := ggui.State(0.0)
	tEnd := ggui.State(1.0)
	rootCol := ggui.State[color.Color](color.RGBA{R: 1, G: 2, B: 3, A: 255})
	scale := ggui.State(1.0)

	var ctrl SVGController
	node := ctrl.Node("box").
		BindRotate(rot).
		BindOffset(off).
		BindScale(scale).
		Origin(0.25, 0.75).
		BindOpacity(op).
		BindVisible(vis).
		BindFill(fill).
		BindStroke(stroke).
		BindStrokeWidth(sw).
		BindTrimStart(tStart).
		BindTrimEnd(tEnd)

	svgW := SVG(doc).Size(80, 40).Controller(&ctrl).BindColor(rootCol)
	counter := &layoutCounter{child: svgW}
	tree := ggui.Provide(colorKey, color.Color(color.RGBA{R: 40, G: 50, B: 60, A: 255}), ggui.Cached(counter))

	target := ggfx.NewImage(100, 100)
	defer target.Deallocate()
	p := ggui.NewProbe(tree, ggui.Sz(100, 100))
	defer p.Close()
	p.RenderTo(target)

	p.Frame()
	if counter.count != 1 {
		t.Fatalf("initial layout count = %d, want 1", counter.count)
	}
	rev0 := svgW.slot.nodesRev
	rasters0 := svgW.slot.surface.rasterCount

	// Unchanged frame skips rasterization and layout.
	p.Frame()
	if counter.count != 1 || svgW.slot.nodesRev != rev0 || svgW.slot.surface.rasterCount != rasters0 {
		t.Fatalf("unchanged frame: layouts=%d rev=%d/%d rasters=%d/%d",
			counter.count, svgW.slot.nodesRev, rev0, svgW.slot.surface.rasterCount, rasters0)
	}

	// Tick Tween and Signals: Paint re-renders with ZERO Layout re-measures!
	rot.Set(1.5)
	for range 5 {
		p.Advance(40 * time.Millisecond)
	}
	off.Set(ggui.Pt(5, -3))
	scale.Set(1.25)
	op.Set(0.5)
	vis.Set(false)
	fill.Set(color.RGBA{R: 0, G: 255, B: 0, A: 255})
	sw.Set(4.0)
	tStart.Set(0.1)
	tEnd.Set(0.9)
	rootCol.Set(color.RGBA{R: 9, G: 8, B: 7, A: 255})
	p.Frame()

	// Imperative node mutation also triggers zero Layout re-measure inside Cached!
	node.ScaleXY(2, 3).OriginAt(10, 20).Trim(0.2, 0.8)
	p.Frame()

	if counter.count != 1 {
		t.Fatalf("Cached Layout re-measured on Paint-time binds/node updates: count=%d, want 1", counter.count)
	}
	if svgW.slot.nodesRev <= rev0 {
		t.Fatalf("nodesRev did not bump on node changes: %d -> %d", rev0, svgW.slot.nodesRev)
	}
	if svgW.slot.surface.rasterCount <= rasters0 {
		t.Fatalf("surface did not re-rasterize on node changes")
	}

	// Verify typed-nil color.Color safely maps to svgx.NoPaint without panicking.
	var typedNil *color.RGBA
	node.Fill(typedNil).Stroke(nil)
	p.Frame()
	if ov, ok := svgW.slot.overrides["box"]; !ok || ov.IsZero() {
		t.Fatal("expected active override for box")
	}

	// ClearFill / ClearStroke / Reset / ResetNodes work cleanly.
	node.ClearFill().ClearStroke()
	p.Frame()
	ctrl.ResetNodes()
	p.Frame()
	if len(svgW.slot.overrides) != 0 {
		t.Fatalf("ResetNodes left overrides: %v", svgW.slot.overrides)
	}
	if counter.count != 1 {
		t.Fatalf("ResetNodes caused Layout re-measure: count=%d, want 1", counter.count)
	}
}

func TestSVGBindPanicsOnNilReaders(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testRectSVG)
	var ctrl SVGController
	node := ctrl.Node("box")

	var typedNilFloat *ggui.StateValue[float64]
	var typedNilColor *ggui.StateValue[color.Color]

	checks := []struct {
		name string
		fn   func()
	}{
		{"SVG.BindColor(nil)", func() { SVG(doc).BindColor(nil) }},
		{"SVG.BindColor(typedNil)", func() { SVG(doc).BindColor(typedNilColor) }},
		{"SVG.BindTime(nil)", func() { SVG(doc).BindTime(nil) }},
		{"SVG.BindProgress(nil)", func() { SVG(doc).BindProgress(nil) }},
		{"SVGNode.BindOffset(nil)", func() { node.BindOffset(nil) }},
		{"SVGNode.BindRotate(nil)", func() { node.BindRotate(nil) }},
		{"SVGNode.BindRotate(typedNil)", func() { node.BindRotate(typedNilFloat) }},
		{"SVGNode.BindScale(nil)", func() { node.BindScale(nil) }},
		{"SVGNode.BindScaleXY(nil)", func() { node.BindScaleXY(nil) }},
		{"SVGNode.BindOpacity(nil)", func() { node.BindOpacity(nil) }},
		{"SVGNode.BindVisible(nil)", func() { node.BindVisible(nil) }},
		{"SVGNode.BindFill(nil)", func() { node.BindFill(nil) }},
		{"SVGNode.BindStroke(nil)", func() { node.BindStroke(nil) }},
		{"SVGNode.BindStrokeWidth(nil)", func() { node.BindStrokeWidth(nil) }},
		{"SVGNode.BindTrimStart(nil)", func() { node.BindTrimStart(nil) }},
		{"SVGNode.BindTrimEnd(nil)", func() { node.BindTrimEnd(nil) }},
	}
	for _, tc := range checks {
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("%s did not panic", tc.name)
				}
			}()
			tc.fn()
		}()
	}
}

func TestSVGRebuildSurvivalViaRetain(t *testing.T) {
	t.Parallel()
	doc1 := mustParseSVG(t, testRectSVG)
	doc2 := mustParseSVG(t, testSquareSVG)
	tl := &svgx.Timeline{Span: 5 * time.Second, End: 5 * time.Second}

	curDoc := ggui.State(doc1)
	bump := ggui.State(0)
	var lastWidget *SVGWidget

	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			_ = bump.Get()
			w := SVG(curDoc.Get()).Key("hero").Size(60, 30)
			w.testTimeline = tl
			lastWidget = w
			return w
		})
	}, ggui.Sz(100, 100))
	defer p.Close()

	target := ggfx.NewImage(100, 100)
	defer target.Deallocate()
	p.RenderTo(target)

	p.Frame()
	firstSlot := lastWidget.slot
	p.Advance(400 * time.Millisecond)
	tBeforeRebuild := lastWidget.slot.ph.pos
	if tBeforeRebuild < 350*time.Millisecond {
		t.Fatalf("expected playhead >= 350ms, got %v", tBeforeRebuild)
	}

	// Rebuild fresh SVGWidget with the same doc and key: adopts playhead and surface!
	bump.Set(1)
	p.Advance(200 * time.Millisecond)
	if lastWidget.slot != firstSlot {
		t.Fatal("rebuilt SVGWidget did not adopt retained slot")
	}
	if lastWidget.slot.ph.pos <= tBeforeRebuild {
		t.Fatalf("adopted playhead did not continue from %v: got %v", tBeforeRebuild, lastWidget.slot.ph.pos)
	}

	// Switching to a different doc creates a fresh slot and resets playhead.
	curDoc.Set(doc2)
	p.Frame()
	if lastWidget.slot == firstSlot || lastWidget.slot.ph.pos != 0 {
		t.Fatalf("doc switch kept old slot or playhead: sameSlot=%v pos=%v",
			lastWidget.slot == firstSlot, lastWidget.slot.ph.pos)
	}
}

func TestSVGTwoWidgetsSharingOneController(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testSquareSVG)
	tl := &svgx.Timeline{Span: 4 * time.Second, End: 4 * time.Second}

	var ctrl SVGController
	ctrl.Node("sq").Opacity(0.75)
	wSmall := SVG(doc).Size(32, 32).Controller(&ctrl)
	wLarge := SVG(doc).Size(80, 80).Controller(&ctrl)
	wSmall.testTimeline = tl
	wLarge.testTimeline = tl

	target := ggfx.NewImage(160, 160)
	defer target.Deallocate()
	p := ggui.NewProbe(Row(wSmall, wLarge), ggui.Sz(160, 160))
	defer p.Close()
	p.RenderTo(target)

	// Advance, not Frame: it pins the probe clock first, so the playhead
	// anchors at that time and not at the wall time a slow Frame starts at.
	p.Advance(0)
	p.Advance(500 * time.Millisecond)

	// Shared playhead advances once per frame (no double-advance!).
	if got := ctrl.Time(); got < 450*time.Millisecond || got > 550*time.Millisecond {
		t.Fatalf("shared controller Time() = %v, want ~500ms (possible double-advance)", got)
	}
	// Each widget owns its own distinct surface at its own pixel size.
	if wSmall.slot.surface == nil || wLarge.slot.surface == nil || wSmall.slot.surface == wLarge.slot.surface {
		t.Fatal("two widgets on one controller must own separate svgSurfaces")
	}
	if wSmall.slot.surface.h.w != 32 || wLarge.slot.surface.h.w != 80 {
		t.Fatalf("surface sizes = %d and %d, want 32 and 80",
			wSmall.slot.surface.h.w, wLarge.slot.surface.h.w)
	}
}

// Not parallel: verifies process-wide svgTextures LRU budget and TextureImages gauge.
func TestSVGVramAccountingAndLRUEviction(t *testing.T) {
	svgTextures.drain()
	defer svgTextures.drain()

	base := inspect.ReadTextureBytes().Images
	gauge := func() int64 { return inspect.ReadTextureBytes().Images - base }

	target := ggfx.NewImage(128, 128)
	defer target.Deallocate()

	// 1. Static shared cache caps at 8 sizes per doc and accounts exact RGBA bytes.
	doc := mustParseSVG(t, testSquareSVG)
	var wantBytes int64
	for sz := 10; sz < 20; sz++ {
		w := SVG(doc).Size(float64(sz), float64(sz))
		p := ggui.NewProbe(&loosely{w: w}, ggui.Sz(128, 128))
		p.RenderTo(target)
		p.Frame()
		p.Close()
	}
	for sz := 12; sz < 20; sz++ {
		wantBytes += int64(sz * sz * 4)
	}
	if got := gauge(); got != wantBytes {
		t.Fatalf("10 static sizes: TextureImages gauge = %d, want %d (last 8 sizes)", got, wantBytes)
	}

	// 2. Unified LRU budget evicts least recently used textures across docs & dynamic surfaces.
	svgTextures.drain()
	if gauge() != 0 {
		t.Fatalf("after drain: gauge = %d, want 0", gauge())
	}

	one := int64(32 * 32 * 4)
	svgTextures.mu.Lock()
	svgTextures.budget = 2 * one
	svgTextures.mu.Unlock()
	defer func() {
		svgTextures.mu.Lock()
		svgTextures.budget = 0
		svgTextures.mu.Unlock()
	}()

	d1 := mustParseSVG(t, testSquareSVG)
	d2 := mustParseSVG(t, testSquareSVG)
	d3 := mustParseSVG(t, testSquareSVG)

	drawStatic := func(d *svgx.Document) {
		p := ggui.NewProbe(&loosely{w: SVG(d).Size(32, 32)}, ggui.Sz(64, 64))
		p.RenderTo(target)
		p.Frame()
		p.Close()
	}
	drawStatic(d1)
	drawStatic(d2)
	if got := gauge(); got != 2*one {
		t.Fatalf("2 static docs: gauge = %d, want %d", got, 2*one)
	}
	// Touch d1 so d2 is LRU, then draw dynamic surface (3rd texture): d2 must be evicted!
	drawStatic(d1)
	var ctrl SVGController
	ctrl.Node("sq").Opacity(0.5)
	dyn := SVG(d3).Size(32, 32).Controller(&ctrl)
	pd := ggui.NewProbe(&loosely{w: dyn}, ggui.Sz(64, 64))
	pd.RenderTo(target)
	pd.Frame()
	pd.Close()

	if got := gauge(); got != 2*one {
		t.Fatalf("after LRU eviction: gauge = %d, want %d", got, 2*one)
	}
	if got := framestats.ReadTextures().Images - base; got != 2*one {
		t.Fatalf("framestats gauge = %d, want %d", got, 2*one)
	}

	// Verify scratch pool reuses and clears buffers without retaining oversized ones.
	a := svgScratchRGBA(20, 20)
	for i := range a.Pix {
		a.Pix[i] = 0xff
	}
	releaseSVGScratch(a)
	b := svgScratchRGBA(10, 10)
	if b.Bounds() != image.Rect(0, 0, 10, 10) || b.Stride != 40 || len(b.Pix) != 400 {
		t.Fatalf("scratch bounds=%v stride=%d len=%d", b.Bounds(), b.Stride, len(b.Pix))
	}
	for _, v := range b.Pix {
		if v != 0 {
			t.Fatal("reused scratch buffer was not cleared")
		}
	}
	releaseSVGScratch(b)

	svgTextures.drain()
	if gauge() != 0 {
		t.Fatalf("final drain: gauge = %d, want 0", gauge())
	}
}

func TestSVGEndToEndSMILPlayback(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testSMILSVG)
	if !doc.Animated() {
		t.Skip("svgx animation backend not yet integrated (doc.Animated() == false)")
	}
	var ctrl SVGController
	w := SVG(doc).Size(100, 100).Controller(&ctrl)
	target := ggfx.NewImage(100, 100)
	defer target.Deallocate()
	p := ggui.NewProbe(w, ggui.Sz(100, 100))
	defer p.Close()
	p.RenderTo(target)

	p.Frame()
	if !ctrl.Playing() || ctrl.Duration() != time.Second {
		t.Fatalf("SMIL doc: playing=%v dur=%v, want true, 1s", ctrl.Playing(), ctrl.Duration())
	}
	p.Advance(500 * time.Millisecond)
	if got := ctrl.Time(); got < 450*time.Millisecond || got > 550*time.Millisecond {
		t.Fatalf("SMIL mid-playback: time=%v, want ~500ms", got)
	}
	p.Advance(600 * time.Millisecond)
	if ctrl.Playing() || ctrl.Time() != time.Second {
		t.Fatalf("SMIL completed: playing=%v time=%v, want false, 1s", ctrl.Playing(), ctrl.Time())
	}
}

func TestSVGCurrentColorDynamicInvalidation(t *testing.T) {
	t.Parallel()

	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}
	blue := color.RGBA{R: 0, G: 0, B: 255, A: 255}

	// 1. testCurrentColorSVG on the dynamic path re-rasterizes and updates pixels when BindColor changes.
	ccDoc := mustParseSVG(t, testCurrentColorSVG)
	colSig := ggui.State[color.Color](red)
	var ctrl SVGController
	ctrl.Node("cc").Opacity(1) // force dynamic per-widget svgSurface path

	w := SVG(ccDoc).Size(40, 40).Controller(&ctrl).BindColor(colSig)
	target := ggfx.NewImage(40, 40)
	defer target.Deallocate()
	p := ggui.NewProbe(&loosely{w: w}, ggui.Sz(40, 40))
	defer p.Close()
	p.RenderTo(target)

	p.Frame()
	if w.slot.surface == nil || w.slot.surface.rasterCount != 1 {
		t.Fatalf("initial dynamic currentColor surface = %v, want rasterCount=1", w.slot.surface)
	}
	if w.slot.surface.h.key.color != packRGBA8(red) {
		t.Fatalf("initial svgKey.color = %#x, want %#x", w.slot.surface.h.key.color, packRGBA8(red))
	}

	var gotPix color.RGBA
	w.slot.surface.onRaster = func(buf *image.RGBA) {
		gotPix = buf.RGBAAt(20, 20)
	}

	// Unchanged frame hits svgSurface cache without re-rasterizing.
	p.Frame()
	if w.slot.surface.rasterCount != 1 {
		t.Fatalf("unchanged frame re-rasterized: count=%d, want 1", w.slot.surface.rasterCount)
	}

	// Changing BindColor invalidates svgSurface and updates rasterized pixels to blue.
	colSig.Set(blue)
	p.Frame()
	if w.slot.surface.rasterCount != 2 {
		t.Fatalf("color change did not invalidate svgSurface: count=%d, want 2", w.slot.surface.rasterCount)
	}
	if gotPix != blue {
		t.Fatalf("updated currentColor pixel = %v, want %v", gotPix, blue)
	}

	// Changing BindColor back to red updates rasterized pixels to red.
	colSig.Set(red)
	p.Frame()
	if w.slot.surface.rasterCount != 3 {
		t.Fatalf("second color change did not invalidate svgSurface: count=%d, want 3", w.slot.surface.rasterCount)
	}
	if gotPix != red {
		t.Fatalf("restored currentColor pixel = %v, want %v", gotPix, red)
	}

	// 2. Doc whose XML does NOT use currentColor (UsesCurrentColor() == false) STILL
	//    invalidates svgSurface when root Color changes (covers dynamic CurrentColorPaint()
	//    overrides and animated currentColor).
	sqDoc := mustParseSVG(t, testSquareSVG)
	if sqDoc.UsesCurrentColor() {
		t.Fatal("expected testSquareSVG.UsesCurrentColor() == false")
	}
	colSig2 := ggui.State[color.Color](red)
	var ctrl2 SVGController
	ctrl2.Node("sq").Opacity(1)
	w2 := SVG(sqDoc).Size(40, 40).Controller(&ctrl2).BindColor(colSig2)

	target2 := ggfx.NewImage(40, 40)
	defer target2.Deallocate()
	p2 := ggui.NewProbe(&loosely{w: w2}, ggui.Sz(40, 40))
	defer p2.Close()
	p2.RenderTo(target2)

	p2.Frame()
	if w2.slot.surface == nil || w2.slot.surface.rasterCount != 1 {
		t.Fatalf("initial dynamic non-currentColor surface = %v, want rasterCount=1", w2.slot.surface)
	}
	if w2.slot.surface.h.key.color != packRGBA8(red) {
		t.Fatalf("initial non-currentColor svgKey.color = %#x, want %#x", w2.slot.surface.h.key.color, packRGBA8(red))
	}
	colSig2.Set(blue)
	p2.Frame()
	if w2.slot.surface.rasterCount != 2 || w2.slot.surface.h.key.color != packRGBA8(blue) {
		t.Fatalf("root color change on dynamic doc with UsesCurrentColor()==false did not invalidate svgSurface: count=%d color=%#x",
			w2.slot.surface.rasterCount, w2.slot.surface.h.key.color)
	}

	// 3. Direct CurrentColorPaint() override on svgSurface updates rasterized pixels when key.color changes.
	surf := newSVGSurface(sqDoc)
	var surfPix color.RGBA
	surf.onRaster = func(buf *image.RGBA) {
		surfPix = buf.RGBAAt(20, 20)
	}
	var m svgx.Matrix
	m.Scale(40.0/100.0, 40.0/100.0)
	ov := svgx.Overrides{"sq": svgx.NodeOverride{}.Fill(svgx.CurrentColorPaint())}

	surf.render(
		svgKey{doc: sqDoc, w: 40, h: 40, color: packRGBA8(red), nodes: 1},
		m,
		&svgx.DrawOptions{CurrentColor: red, Overrides: ov},
	)
	if surfPix != red {
		t.Fatalf("CurrentColorPaint red pixel = %v, want %v", surfPix, red)
	}
	surf.render(
		svgKey{doc: sqDoc, w: 40, h: 40, color: packRGBA8(blue), nodes: 1},
		m,
		&svgx.DrawOptions{CurrentColor: blue, Overrides: ov},
	)
	if surf.rasterCount != 2 {
		t.Fatalf("CurrentColorPaint color change rasterCount = %d, want 2", surf.rasterCount)
	}
	if surfPix != blue {
		t.Fatalf("CurrentColorPaint blue pixel = %v, want %v", surfPix, blue)
	}
	surf.releaseTexture()
}

func TestSVGSharedControllerOnDoneNotSwallowed(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testSquareSVG)
	tl := &svgx.Timeline{Span: time.Second, End: time.Second}

	var ctrl SVGController
	doneB := 0
	// Widget A (onDone == nil) paints first; Widget B (onDone != nil) paints second.
	wA := SVG(doc).Size(40, 40).Controller(&ctrl)
	wB := SVG(doc).Size(40, 40).Controller(&ctrl).OnDone(func() { doneB++ })
	wA.testTimeline = tl
	wB.testTimeline = tl

	p := ggui.NewProbe(Row(wA, wB), ggui.Sz(120, 60))
	defer p.Close()

	p.Frame()
	p.Advance(1100 * time.Millisecond)
	p.Frame()
	if ctrl.Playing() || ctrl.Time() != time.Second {
		t.Fatalf("shared playback end: playing=%v time=%v, want false, 1s", ctrl.Playing(), ctrl.Time())
	}
	if doneB != 1 {
		t.Fatalf("Widget B OnDone count = %d, want 1 (swallowed by Widget A?)", doneB)
	}

	// Extra frames do not refire OnDone.
	p.Advance(300 * time.Millisecond)
	p.Frame()
	if doneB != 1 {
		t.Fatalf("Widget B OnDone refired: count=%d, want 1", doneB)
	}

	// Second run fires Widget B again, and if Widget A also registers OnDone, both fire once.
	doneA := 0
	wA.OnDone(func() { doneA++ })
	ctrl.Play()
	p.Frame()
	p.Advance(1100 * time.Millisecond)
	p.Frame()
	if doneA != 1 || doneB != 2 {
		t.Fatalf("second run OnDone: doneA=%d doneB=%d, want 1, 2", doneA, doneB)
	}
}

func TestSVGControllerDocSwitchResetsPlayhead(t *testing.T) {
	t.Parallel()
	doc1 := mustParseSVG(t, testRectSVG)
	doc2 := mustParseSVG(t, testSquareSVG)
	tl1 := &svgx.Timeline{Span: time.Second, End: time.Second}
	tl2 := &svgx.Timeline{Span: 2 * time.Second, End: 2 * time.Second}

	var ctrl SVGController
	curDoc := ggui.State(doc1)
	done1, done2 := 0, 0

	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Reactive(func() ggui.Widget {
			d := curDoc.Get()
			w := SVG(d).Key("swap").Size(60, 30).Controller(&ctrl)
			if d == doc1 {
				w.testTimeline = tl1
				w.OnDone(func() { done1++ })
			} else {
				w.testTimeline = tl2
				w.OnDone(func() { done2++ })
			}
			return w
		})
	}, ggui.Sz(100, 100))
	defer p.Close()

	// Run doc1 to completion so pos=1s, playing=false, doneEpoch=1.
	p.Frame()
	p.Advance(1100 * time.Millisecond)
	p.Frame()
	if ctrl.Playing() || ctrl.Time() != time.Second || done1 != 1 {
		t.Fatalf("doc1 finished: playing=%v time=%v done1=%d, want false, 1s, 1",
			ctrl.Playing(), ctrl.Time(), done1)
	}

	// Switch to doc2 reusing the same controller: playhead must reset to 0 and AutoPlay must restart.
	curDoc.Set(doc2)
	p.Frame()
	if ctrl.Time() != 0 || !ctrl.Playing() || ctrl.Duration() != 2*time.Second || done2 != 0 {
		t.Fatalf("after doc switch: time=%v playing=%v dur=%v done2=%d, want 0, true, 2s, 0",
			ctrl.Time(), ctrl.Playing(), ctrl.Duration(), done2)
	}

	// doc2 plays smoothly from 0 and fires its own OnDone on completion.
	p.Advance(500 * time.Millisecond)
	if got := ctrl.Time(); got < 450*time.Millisecond || got > 550*time.Millisecond {
		t.Fatalf("doc2 mid-play Time() = %v, want ~500ms", got)
	}
	p.Advance(1600 * time.Millisecond)
	p.Frame()
	if ctrl.Playing() || ctrl.Time() != 2*time.Second || done2 != 1 {
		t.Fatalf("doc2 finished: playing=%v time=%v done2=%d, want false, 2s, 1",
			ctrl.Playing(), ctrl.Time(), done2)
	}
}

func TestSVGNonAnimatedWithTimeUsesStaticCache(t *testing.T) {
	t.Parallel()
	doc := mustParseSVG(t, testSquareSVG)

	w1 := SVG(doc).Size(40, 40).Time(500 * time.Millisecond)
	w2 := SVG(doc).Size(40, 40).Time(900 * time.Millisecond)

	target := ggfx.NewImage(100, 100)
	defer target.Deallocate()
	p := ggui.NewProbe(Row(w1, w2), ggui.Sz(100, 100))
	defer p.Close()
	p.RenderTo(target)

	p.Frame()
	if w1.slot.surface != nil || w2.slot.surface != nil {
		t.Fatalf("non-animated SVG with Time() allocated private dynamic surface: w1=%v w2=%v",
			w1.slot.surface, w2.slot.surface)
	}
	var m svgx.Matrix
	m.Scale(40.0/100.0, 40.0/100.0)
	if img, hit := svgTextures.getStatic(doc, 40, 40, 0, color.Black, m); img == nil || !hit {
		t.Fatalf("expected shared static cache hit for non-animated SVG with Time(): img=%v hit=%v", img, hit)
	}
}
