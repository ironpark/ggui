package record

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/widget"
)

func TestGlide(t *testing.T) {
	a, b := ggui.Pt(10, 200), ggui.Pt(310, 40)
	if glide(a, b, 0) != a || glide(a, b, 1) != b {
		t.Fatal("the glide does not start and end where it is asked to")
	}
	dx, dy := b.X-a.X, b.Y-a.Y
	along := func(p ggui.Point) float64 { return ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / (dx*dx + dy*dy) }
	prev := 0.0
	bowed := false
	for i := 1; i <= 30; i++ {
		p := glide(a, b, float64(i)/30)
		f := along(p)
		if f < prev {
			t.Fatalf("step %d goes back: %g after %g", i, f, prev)
		}
		prev = f
		// Off the straight line, a little.
		off := math.Abs((p.X-a.X)*dy-(p.Y-a.Y)*dx) / math.Hypot(dx, dy)
		if off > 40 {
			t.Fatalf("step %d is %g off the line", i, off)
		}
		if off > 1 {
			bowed = true
		}
	}
	if !bowed {
		t.Error("the glide is a straight line")
	}
	// Eased: the first and last steps are shorter than the middle one.
	step := func(i int) float64 { return along(glide(a, b, float64(i+1)/30)) - along(glide(a, b, float64(i)/30)) }
	if step(0) >= step(15) || step(29) >= step(15) {
		t.Errorf("not eased: %g, %g, %g", step(0), step(15), step(29))
	}
}

// pad is a control that logs the pointer events it is given and is
// drawn red while pressed.
type pad struct {
	ggui.Interactive
	log *[]ggui.PointerKind
}

func (p *pad) Layout(c ggui.Constraints, _ ggui.Env) ggui.Size { return ggui.Sz(40, 20) }

func (p *pad) Paint(dst *ggui.Canvas, r ggui.Rect) {
	col := color.Color(blue)
	if p.Pressed {
		col = red
	}
	dst.FillRect(r, col)
	p.Hit(dst, r, p, ggui.CursorShapePointer)
}

func (p *pad) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind != ggui.PointerMove {
		*p.log = append(*p.log, ev.Kind)
	}
	return p.Pointer(ev, nil)
}

func (p *pad) HandleKey(ggui.KeyEvent) {}

func padTour(log *[]ggui.PointerKind, script func(r *Recorder)) Tour {
	return Tour{
		Size: ggui.Sz(100, 60),
		FPS:  20,
		Build: func() ggui.Widget {
			p := &pad{log: log}
			p.Role = ggui.RoleButton
			p.SetName("Pad")
			return widget.Box(p).Padding(ggui.Insets(20))
		},
		Setup: func(*ggui.Probe) {
			ggui.SetEnv(ggui.Untrack(ggui.UseEnv).With(ggui.BackgroundKey, color.Color(white)))
		},
		Script: script,
	}
}

// ClickLabel glides to the control, enters it, presses and releases it
// in order, and a frame shows it pressed and the cursor as a hand.
func TestClickLabel(t *testing.T) {
	var log []ggui.PointerKind
	var r *Recorder
	tour := padTour(&log, func(rec *Recorder) {
		r = rec
		rec.ClickLabel("Pad")
		rec.Hold(100 * time.Millisecond)
	})
	pressed, frames := 0, 0
	var hand bool
	err := tour.play(resolve(nil), func(img *image.RGBA) error {
		frames++
		if img.RGBAAt(25, 22) == red {
			pressed++
		}
		if r.p.Cursor() == ggui.CursorShapePointer && r.mouse.placed {
			hand = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var kinds []ggui.PointerKind
	for _, k := range log {
		if k == ggui.PointerEnter || k == ggui.PointerDown || k == ggui.PointerUp || k == ggui.PointerTap {
			kinds = append(kinds, k)
		}
	}
	want := []ggui.PointerKind{ggui.PointerEnter, ggui.PointerDown, ggui.PointerUp, ggui.PointerTap}
	if len(kinds) < 4 || kinds[0] != want[0] || kinds[1] != want[1] || kinds[2] != want[2] || kinds[3] != want[3] {
		t.Errorf("the pad heard %v, want %v", kinds, want)
	}
	if pressed == 0 {
		t.Error("no frame shows the pad pressed")
	}
	if !hand {
		t.Error("the cursor over the pad is not a hand")
	}
	if r.Pointer() != ggui.Pt(40, 30) {
		t.Errorf("the pointer ended at %v, not the pad's middle", r.Pointer())
	}
	// A glide of 400ms or more, the click's 100ms held and 100ms after,
	// and the Hold.
	if frames < 8+2+2+2 {
		t.Errorf("%d frames drawn", frames)
	}
}

func TestClickLabelMissing(t *testing.T) {
	tour := padTour(new([]ggui.PointerKind), func(r *Recorder) {
		r.Hold(50 * time.Millisecond)
		r.ClickLabel("Save as")
	})
	err := tour.play(resolve(nil), func(*image.RGBA) error { return nil })
	if err == nil || !strings.Contains(err.Error(), `"Save as"`) {
		t.Fatalf("err = %v, want one naming the label", err)
	}
}

// The cursor is drawn where the pointer is, from the first pointer step,
// and not at all with Cursor(false).
func TestCursorDrawn(t *testing.T) {
	var last *image.RGBA
	var firstDrawn int
	run := func(o ...Option) {
		last, firstDrawn = nil, -1
		blank := fill(100, 60, white)
		n := 0
		tour := padTour(new([]ggui.PointerKind), func(r *Recorder) {
			r.Hold(100 * time.Millisecond)
			r.MoveTo(ggui.Pt(80, 50), 0)
			r.Hold(100 * time.Millisecond)
		})
		err := tour.play(resolve(o), func(img *image.RGBA) error {
			// Around (80, 50), clear of the pad.
			if firstDrawn < 0 && !changed(blank.SubImage(image.Rect(70, 45, 100, 60)).(*image.RGBA), sub(img, image.Rect(70, 45, 100, 60))).Empty() {
				firstDrawn = n
			}
			n++
			last = img
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	run()
	if firstDrawn != 2 {
		t.Errorf("the cursor first shows in frame %d, want 2", firstDrawn)
	}
	// The arrow's tip is at the pointer: black there, white just left.
	if c := last.RGBAAt(80, 52); luma(c) > 60 {
		t.Errorf("the arrow's body under the pointer is %v", c)
	}
	run(Cursor(false))
	if firstDrawn != -1 {
		t.Errorf("Cursor(false) drew a cursor in frame %d", firstDrawn)
	}
}

// sub returns the part r of img, its pixels moved to start at (0, 0).
func sub(img *image.RGBA, r image.Rectangle) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := range r.Dy() {
		copy(out.Pix[y*out.Stride:], img.Pix[img.PixOffset(r.Min.X, r.Min.Y+y):img.PixOffset(r.Max.X, r.Min.Y+y)])
	}
	return out
}

// Type types a character a frame at 50ms apart and 20 frames a second.
func TestTypeProgress(t *testing.T) {
	text := ggui.State("")
	var r *Recorder
	tour := Tour{
		Size: ggui.Sz(200, 40),
		FPS:  20,
		Build: func() ggui.Widget {
			return widget.TextInput(text).Placeholder("Name")
		},
		Script: func(rec *Recorder) {
			r = rec
			rec.ClickLabel("Name")
			rec.Type("hello", 50*time.Millisecond)
		},
	}
	var seen []string
	err := tour.play(resolve(nil), func(*image.RGBA) error {
		seen = append(seen, ggui.Untrack(text.Get))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = r
	got := seen[len(seen)-5:]
	want := []string{"h", "he", "hel", "hell", "hello"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the last five frames show %q, want %q", got, want)
		}
	}
}

// While only the cursor moves over a still app, each GIF frame holds a
// small rectangle around it.
func TestCursorSmallRects(t *testing.T) {
	tour := Tour{
		Size:  ggui.Sz(300, 200),
		FPS:   20,
		Frame: FrameMac,
		Build: func() ggui.Widget { return widget.Text("still") },
		Script: func(r *Recorder) {
			r.MoveTo(ggui.Pt(20, 20), 0)
			r.Hold(100 * time.Millisecond)
			r.MoveTo(ggui.Pt(280, 180), time.Second)
			r.Hold(100 * time.Millisecond)
		},
	}
	o := resolve(nil)
	o.flatten = true
	size := tour.layout(o).size
	s, err := newGIFSink(size.X, size.Y, o, tour.fps())
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	if err := tour.play(o, s.add); err != nil {
		t.Fatal(err)
	}
	if len(s.frames) < 15 {
		t.Fatalf("%d frames spooled", len(s.frames))
	}
	for i, f := range s.frames[1:] {
		// A step of the glide is at most about 25 pixels, and the
		// cursor's sprite 24.
		if f.rect.Dx() > 80 || f.rect.Dy() > 80 {
			t.Errorf("frame %d changed %v", i+1, f.rect)
		}
	}
}
