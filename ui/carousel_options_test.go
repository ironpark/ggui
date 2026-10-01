package ui

import (
	"math"
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

func TestCarouselContentFlattensIntoSlides(t *testing.T) {
	t.Parallel()
	value := ggui.State(0)
	c := Carousel(value, CarouselContent(ggui.Text("a"), CarouselItem(ggui.Text("b"))), nil, ggui.Text("c"))
	p := ggui.NewProbe(c, ggui.Sz(416, 240))
	defer p.Close()
	p.Frame()
	if c.SnapCount() != 3 {
		t.Fatalf("grouped and loose slides made %d snaps, want 3", c.SnapCount())
	}
	if _, ok := p.Semantics().Find(ggui.RoleGroup, "Slide 1 of 3"); !ok {
		t.Fatalf("first slide is not described:\n%s", p.Semantics())
	}
	// Standalone, the content is a plain row of its items.
	row := ggui.NewProbe(CarouselContent(ggui.Text("a"), ggui.Text("b")), ggui.Sz(200, 40))
	defer row.Close()
	if _, ok := row.Semantics().Find("", "b"); !ok {
		t.Fatal("standalone CarouselContent did not paint its items")
	}
}

func TestCarouselBasisWhenFollowsTheViewport(t *testing.T) {
	t.Parallel()
	var seen []ggui.Size
	slides := make([]ggui.Widget, 4)
	for i := range slides {
		slides[i] = CarouselItem(ggui.Text("Slide")).BasisWhen(func(s ggui.Size) float64 {
			seen = append(seen, s)
			if s.W >= 400 {
				return .5
			}
			return math.NaN() // not a fraction: falls back to a full slide
		})
	}
	c := Carousel(ggui.State(0), slides...).Align(0)
	p := ggui.NewProbe(c, ggui.Sz(600, 200))
	defer p.Close()
	p.Frame()
	if c.SnapCount() != 3 {
		t.Fatalf("half-width slides in a wide viewport made %d snaps, want 3", c.SnapCount())
	}
	if len(seen) == 0 || seen[len(seen)-1].W != 600-2*48 {
		t.Fatalf("BasisWhen saw %v, want the viewport inside the control gutters", seen)
	}
	p.Resize(ggui.Sz(300, 200))
	p.Frame()
	if c.SnapCount() != 4 {
		t.Fatalf("full slides in a narrow viewport made %d snaps, want 4", c.SnapCount())
	}
}

func TestCarouselWithoutControlsUsesTheFullWidth(t *testing.T) {
	t.Parallel()
	c := Carousel(ggui.State(0), carouselSlides(3, 1)...).Controls(false).Gap(-5).Height(-1)
	p := ggui.NewProbe(ggui.Column(c), ggui.Sz(416, 240))
	defer p.Close()
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleButton, "Next slide"); ok {
		t.Fatal("Controls(false) still painted the navigation buttons")
	}
	if c.view != 416 {
		t.Fatalf("viewport is %v wide, want the whole 416", c.view)
	}
	// A negative gap is none, so slides start one viewport apart.
	if c.snaps[1]-c.snaps[0] != 416 {
		t.Fatalf("slides are %v apart, want 416", c.snaps[1]-c.snaps[0])
	}
	if c.viewport.Size.H != 0 {
		t.Fatalf("Height(-1) gave a %v tall viewport, want 0", c.viewport.Size.H)
	}
}

func TestCarouselDraggableFalseIgnoresPointerDrags(t *testing.T) {
	t.Parallel()
	c := Carousel(ggui.State(0), carouselSlides(3, 1)...).Draggable(false)
	p := ggui.NewProbe(c, ggui.Sz(416, 240))
	defer p.Close()
	p.Frame()
	if c.CaptureTouchDrag() {
		t.Fatal("a carousel that cannot be dragged still captures touch drags")
	}
	p.Press(ggui.Pt(340, 120))
	p.Move(ggui.Pt(60, 120))
	p.Release(ggui.Pt(60, 120))
	p.Advance(2 * time.Second)
	if c.Selected() != 0 || c.dragging {
		t.Fatalf("drag moved a non-draggable carousel to %d", c.Selected())
	}
	c.Draggable(true)
	p.Frame()
	if !c.CaptureTouchDrag() {
		t.Fatal("a draggable carousel with several snaps must capture touch drags")
	}
	single := Carousel(ggui.State(0), carouselSlides(1, 1)...)
	q := ggui.NewProbe(single, ggui.Sz(416, 240))
	defer q.Close()
	q.Frame()
	if single.CaptureTouchDrag() {
		t.Fatal("a single slide has nowhere to drag to but captured the drag")
	}
}

func TestCarouselZeroAnimationSnapsImmediately(t *testing.T) {
	t.Parallel()
	c := Carousel(ggui.State(0), carouselSlides(3, 1)...).Animation(-time.Second)
	p := ggui.NewProbe(c, ggui.Sz(416, 240))
	defer p.Close()
	p.Frame()
	c.Next()
	if c.offset != c.snaps[1] {
		t.Fatalf("offset %v right after Next, want the snap %v with no animation", c.offset, c.snaps[1])
	}
}

func TestCarouselAssistiveActionsClampAndRespectDisabled(t *testing.T) {
	t.Parallel()
	value := ggui.State(0)
	c := Carousel(value, carouselSlides(5, 1)...).Name("Gallery")
	p := ggui.NewProbe(c, ggui.Sz(416, 240))
	defer p.Close()
	p.Frame()
	steps := []struct {
		act  ggui.Action
		ok   bool
		want int
	}{
		{ggui.Action{Kind: ggui.ActionIncrement}, true, 1},
		{ggui.Action{Kind: ggui.ActionSetValue, Num: 99}, true, 4},
		{ggui.Action{Kind: ggui.ActionDecrement}, true, 3},
		{ggui.Action{Kind: ggui.ActionSetValue, Num: -7}, true, 0},
		{ggui.Action{Kind: ggui.ActionSetValue, Num: math.Inf(1)}, false, 0},
		{ggui.Action{Kind: ggui.ActionPress}, false, 0},
	}
	for _, s := range steps {
		if got := c.Act(s.act); got != s.ok || ggui.Untrack(value.Get) != s.want {
			t.Fatalf("%+v: handled %v, slide %d; want %v, %d", s.act, got, ggui.Untrack(value.Get), s.ok, s.want)
		}
	}
	value.Set(2)
	node, ok := p.Semantics().Find(ggui.RoleGroup, "Gallery")
	if !ok || node.Value != "Slide 3 of 5" || node.Now != 2 || node.Max != 4 {
		t.Fatalf("carousel node = %+v, want slide 3 of 5", node.Node)
	}
	c.Disabled(true)
	p.Frame()
	if c.Act(ggui.Action{Kind: ggui.ActionIncrement}) || ggui.Untrack(value.Get) != 2 {
		t.Fatal("a disabled carousel accepted an assistive action")
	}
}

func TestCarouselConsumesOnlyItsAxisKeys(t *testing.T) {
	t.Parallel()
	press := func(k ggui.KeyboardKey) ggui.KeyEvent { return ggui.KeyEvent{Kind: ggui.KeyPress, Key: k} }
	horizontal := Carousel(ggui.State(0), carouselSlides(2, 1)...)
	vertical := Carousel(ggui.State(0), carouselSlides(2, 1)...).Vertical()
	for _, tc := range []struct {
		key        ggui.KeyboardKey
		horizontal bool
		vertical   bool
	}{
		{ggui.KeyArrowLeft, true, false},
		{ggui.KeyArrowRight, true, false},
		{ggui.KeyArrowUp, false, true},
		{ggui.KeyArrowDown, false, true},
		{ggui.KeyHome, true, true},
		{ggui.KeyEnd, true, true},
		{ggui.KeyEnter, false, false},
	} {
		if got := horizontal.ConsumesKey(press(tc.key)); got != tc.horizontal {
			t.Errorf("horizontal carousel consumes %v = %v, want %v", tc.key, got, tc.horizontal)
		}
		if got := vertical.ConsumesKey(press(tc.key)); got != tc.vertical {
			t.Errorf("vertical carousel consumes %v = %v, want %v", tc.key, got, tc.vertical)
		}
	}
	if horizontal.ConsumesKey(ggui.KeyEvent{Kind: ggui.KeyText, Text: "a"}) {
		t.Error("typed text was consumed as navigation")
	}
}

func TestCarouselStandaloneNavigationButtons(t *testing.T) {
	t.Parallel()
	value := ggui.State(0)
	c := Carousel(value, carouselSlides(4, 1)...).Controls(false)
	p := ggui.NewProbe(ggui.Column(c, ggui.Row(CarouselPrevious(c), CarouselNext(c))), ggui.Sz(416, 320))
	defer p.Close()
	prev, _ := p.Semantics().Find(ggui.RoleButton, "Previous slide")
	if !prev.Disabled {
		t.Fatal("Previous slide is enabled on the first slide")
	}
	next, ok := p.Semantics().Find(ggui.RoleButton, "Next slide")
	if !ok || next.Disabled {
		t.Fatalf("Next slide missing or disabled:\n%s", p.Semantics())
	}
	p.Perform(next.ID, ggui.Action{Kind: ggui.ActionPress})
	if ggui.Untrack(value.Get) != 1 {
		t.Fatalf("pressing Next slide selected %d, want 1", ggui.Untrack(value.Get))
	}
	// Focused, a navigation button also forwards the carousel's arrow keys.
	p.Tap("Next slide")
	p.Type(ggui.Mods{}, ggui.KeyArrowLeft)
	if got := ggui.Untrack(value.Get); got != 1 {
		t.Fatalf("Tap then ArrowLeft left slide %d, want 1 (2 then back)", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	if got := ggui.Untrack(value.Get); got != 3 {
		t.Fatalf("End on the navigation button selected %d, want the last slide", got)
	}
	if n, _ := p.Semantics().Find(ggui.RoleButton, "Next slide"); !n.Disabled {
		t.Fatal("Next slide is still enabled on the last slide")
	}
}
