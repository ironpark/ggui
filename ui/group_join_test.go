package ui_test

import (
	"strconv"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/icons"
)

// sidesRecorder is a child of a group that keeps the JoinedSides it was
// laid out under.
type sidesRecorder struct {
	size  ggui.Size
	sides ui.JoinedSides
}

func (w *sidesRecorder) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	w.sides, _ = env.Get(ui.JoinedSidesKey)
	return c.Constrain(w.size)
}
func (*sidesRecorder) Paint(*ggui.Canvas, ggui.Rect) {}

func TestButtonGroupTellsEachChildWhichSidesAreJoined(t *testing.T) {
	t.Parallel()
	make3 := func() []*sidesRecorder {
		return []*sidesRecorder{{size: ggui.Sz(30, 20)}, {size: ggui.Sz(30, 20)}, {size: ggui.Sz(30, 20)}}
	}
	group := func(rs []*sidesRecorder) *ui.ButtonGroupWidget { return ui.ButtonGroup(rs[0], rs[1], rs[2]) }
	type sides = ui.JoinedSides
	for _, tc := range []struct {
		name  string
		build func(rs []*sidesRecorder) ggui.Widget
		want  [3]sides
	}{
		{"row", func(rs []*sidesRecorder) ggui.Widget { return group(rs) }, [3]sides{{Right: true}, {Left: true, Right: true}, {Left: true}}},
		{"column", func(rs []*sidesRecorder) ggui.Widget { return group(rs).Vertical() }, [3]sides{{Bottom: true}, {Top: true, Bottom: true}, {Top: true}}},
		{"right to left", func(rs []*sidesRecorder) ggui.Widget { return ggui.Directionality(ggui.DirRTL, group(rs)) }, [3]sides{{Left: true}, {Left: true, Right: true}, {Right: true}}},
		{"beside a nested group", func(rs []*sidesRecorder) ggui.Widget {
			return ui.ButtonGroup(rs[0], rs[1], rs[2], ui.ButtonGroup(ui.Button("x", nil)))
		}, [3]sides{}},
	} {
		rs := make3()
		p := ggui.NewProbe(ggui.Row(tc.build(rs)), ggui.Sz(400, 200))
		p.Frame()
		for i, r := range rs {
			if r.sides != tc.want[i] {
				t.Errorf("%s: child %d is joined on %+v, want %+v", tc.name, i, r.sides, tc.want[i])
			}
		}
		p.Close()
	}
	lone := &sidesRecorder{size: ggui.Sz(30, 20)}
	p := ggui.NewProbe(ggui.Row(ui.ButtonGroup(lone)), ggui.Sz(400, 200))
	defer p.Close()
	p.Frame()
	if lone.sides.Any() {
		t.Errorf("an only child is joined on %+v", lone.sides)
	}
}

func TestButtonGroupJoinsWithoutAGapAndStretchesItsChildren(t *testing.T) {
	t.Parallel()
	g := ui.ButtonGroup(
		ui.Button("Apply", nil).Size(ui.ButtonLarge),
		ui.ButtonGroupSeparator(),
		ui.Button("More", nil).Size(ui.ButtonSmall),
		ui.ButtonGroupText("items"),
	)
	p := ggui.NewProbe(ggui.Row(g, ggui.Spacer()), ggui.Sz(500, 100))
	defer p.Close()
	apply, more := find(t, p, ggui.RoleButton, "Apply").Rect, find(t, p, ggui.RoleButton, "More").Rect
	strip := node(t, p.Semantics(), ggui.RoleToolbar, "Actions").Rect
	if apply.Size.H != 36 || more.Size.H != 36 || strip.Size.H != 36 {
		t.Fatalf("heights: Apply %v, More %v, strip %v; want every child stretched to the tallest, 36", apply.Size.H, more.Size.H, strip.Size.H)
	}
	// The separator's line is all that lies between the two buttons.
	if gap := more.Origin.X - (apply.Origin.X + apply.Size.W); gap != 1 {
		t.Fatalf("gap across the separator = %v, want its one pixel", gap)
	}
	if text := strip.Origin.X + strip.Size.W - (more.Origin.X + more.Size.W); text <= 20 {
		t.Fatalf("the caption got %v of the strip", text)
	}
	if apply.Origin.X != strip.Origin.X {
		t.Fatalf("the strip starts at %v, its first child at %v", strip.Origin.X, apply.Origin.X)
	}
}

func TestButtonGroupOfGroupsKeepsThemApart(t *testing.T) {
	t.Parallel()
	g := ui.ButtonGroup(
		ui.ButtonGroup(ui.Button("One", nil).Outline(), ui.Button("Two", nil).Outline()).Name("Pages"),
		ui.ButtonGroup(ui.Button("Back", nil).Outline(), ui.Button("Next", nil).Outline()).Name("Steps"),
	)
	p := ggui.NewProbe(ggui.Row(g, ggui.Spacer()), ggui.Sz(500, 100))
	defer p.Close()
	one, two := find(t, p, ggui.RoleButton, "One").Rect, find(t, p, ggui.RoleButton, "Two").Rect
	back := find(t, p, ggui.RoleButton, "Back").Rect
	if two.Origin.X != one.Origin.X+one.Size.W {
		t.Fatalf("One ends at %v and Two starts at %v; want them joined", one.Origin.X+one.Size.W, two.Origin.X)
	}
	if gap := back.Origin.X - (two.Origin.X + two.Size.W); gap != 8 {
		t.Fatalf("gap between the nested groups = %v, want a Space", gap)
	}
}

func TestButtonGroupGivesATextFieldTheSpareWidth(t *testing.T) {
	t.Parallel()
	g := ui.ButtonGroup(ui.ButtonGroupText("https://"), ui.TextField(ggui.State("")).Name("Host"), ui.Button("Open", nil).Outline())
	p := ggui.NewProbe(ggui.Row(ggui.Box(g).Width(400), ggui.Spacer()), ggui.Sz(500, 100))
	defer p.Close()
	open := find(t, p, ggui.RoleButton, "Open").Rect
	if end := open.Origin.X + open.Size.W; end != 400 {
		t.Fatalf("the strip ends at %v, want the 400 its parent gave it", end)
	}
}

// A focused child's ring is the group's to draw, over the children painted
// after it; with the keyboard on it nothing else may change, and the
// children keep their own tab stops.
func TestButtonGroupChildrenKeepTheirOwnFocus(t *testing.T) {
	t.Parallel()
	taps := 0
	g := ui.ButtonGroup(ui.Button("One", nil).Outline(), ui.Button("Two", func() { taps++ }).Outline(), ui.Button("Three", nil).Outline())
	p := ggui.NewProbe(ggui.Row(g, ggui.Spacer()), ggui.Sz(500, 100))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyTab)
	p.Frame()
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if taps != 1 {
		t.Fatalf("Space on the second child pressed it %d times", taps)
	}
}

func TestToggleGroupSegmentsAreTogglesWithSpacing(t *testing.T) {
	t.Parallel()
	opts := []string{"a", "bb", "c"}
	rects := func(g ggui.Widget) (group ggui.Rect, segs []ggui.Rect) {
		p := ggui.NewProbe(ggui.Row(g, ggui.Spacer()), ggui.Sz(500, 300))
		defer p.Close()
		p.Frame()
		for _, f := range p.FindAll(ggui.RoleRadio) {
			segs = append(segs, f.Rect)
		}
		return node(t, p.Semantics(), ggui.RoleGroup, "Options").Rect, segs
	}
	for _, tc := range []struct {
		name      string
		group     *ui.ToggleGroupWidget[string]
		side, gap float64
	}{
		{"default", ui.ToggleGroup(ggui.State("a")).Options(opts), 32, 8},
		{"small", ui.ToggleGroup(ggui.State("a")).Options(opts).Size(ui.ButtonSmall), 28, 8},
		{"large outline", ui.ToggleGroup(ggui.State("a")).Options(opts).Size(ui.ButtonLarge).Outline(), 36, 8},
		{"spacing 1", ui.ToggleGroup(ggui.State("a")).Options(opts).Spacing(1), 32, 4},
		{"joined", ui.ToggleGroup(ggui.State("a")).Options(opts).Outline().Spacing(0), 32, 0},
	} {
		group, segs := rects(tc.group)
		if len(segs) != 3 {
			t.Fatalf("%s: %d segments", tc.name, len(segs))
		}
		// No track around the segments: the group is exactly as big as they are.
		if group.Origin != segs[0].Origin || group.Size.H != tc.side || group.Origin.X+group.Size.W != segs[2].Origin.X+segs[2].Size.W {
			t.Errorf("%s: group %+v around segments %+v", tc.name, group, segs)
		}
		for i, s := range segs {
			if s.Size.H != tc.side || s.Size.W < tc.side {
				t.Errorf("%s: segment %d is %v, want %v tall and no narrower", tc.name, i, s.Size, tc.side)
			}
			if i > 0 {
				if gap := s.Origin.X - (segs[i-1].Origin.X + segs[i-1].Size.W); gap != tc.gap {
					t.Errorf("%s: gap before segment %d = %v, want %v", tc.name, i, gap, tc.gap)
				}
			}
		}
		if segs[0].Size.W != tc.side {
			t.Errorf("%s: a one-letter segment is %v wide, want the minimum %v", tc.name, segs[0].Size.W, tc.side)
		}
	}
	// Joined segments are padded less, as shadcn's px-2 for spacing 0.
	_, apart := rects(ui.ToggleGroup(ggui.State("a")).Options([]string{"alignment"}))
	_, joined := rects(ui.ToggleGroup(ggui.State("a")).Options([]string{"alignment"}).Spacing(0))
	if d := apart[0].Size.W - joined[0].Size.W; d != 4 {
		t.Errorf("a joined segment is %v narrower, want 4", d)
	}
	_, stacked := rects(ui.ToggleGroup(ggui.State("a")).Options(opts).Vertical().Spacing(0))
	if stacked[1].Origin.Y != stacked[0].Origin.Y+stacked[0].Size.H || stacked[0].Size.W != stacked[1].Size.W {
		t.Errorf("vertical joined segments %+v", stacked)
	}
}

// As TestControlsLeaveACachedParentHeld, for the controls that join: none
// may write a property after measuring with it, or no Cached above holds.
func TestJoinedControlsLeaveACachedParentHeld(t *testing.T) {
	t.Parallel()
	options := []string{"a", "b", "c"}
	for name, w := range map[string]ggui.Widget{
		"Button with icons":  ui.Button("b", nil).Icon(ui.Icon(icons.Plus)).TrailingIcon(ui.Icon(icons.ChevronDown)),
		"ToggleOf":           ui.ToggleOf(ggui.State(true), ui.Icon(icons.Search)).Outline(),
		"ButtonGroup":        ui.ButtonGroup(ui.Button("a", nil).Outline(), ui.ButtonGroupSeparator(), ui.Button("b", nil).Size(ui.ButtonSmall), ui.ButtonGroupText("c")),
		"ButtonGroup nested": ui.ButtonGroup(ui.ButtonGroup(ui.Button("a", nil), ui.Button("b", nil)), ui.ButtonGroup(ui.Button("c", nil))).Vertical(),
		"ToggleGroup":        ui.ToggleGroup(ggui.State("a")).Options(options),
		"ToggleGroup joined": ui.ToggleGroup(ggui.State("a")).Options(options).Outline().Spacing(0).Size(ui.ButtonSmall),
		"KbdGroup":           ui.KbdGroup(ui.Kbd("Ctrl"), ui.Kbd("K")),
		"ButtonGroup fields": ui.ButtonGroup(ui.ButtonGroupText("https://"), ui.TextField(ggui.State("")).Name("Host"), ui.NumberInput(ggui.State(1.0)), ui.Button("Go", nil).Outline()),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sibling := &layoutCounter{}
			tick := ggui.State(0)
			root := ggui.Column(ggui.Cached(ggui.Column(w, sibling)), ggui.TextOf(ggui.Map[int, string](tick, strconv.Itoa)))
			p := ggui.NewProbe(root, ggui.Sz(600, 600))
			defer p.Close()
			for range 3 {
				p.Frame()
			}
			before := sibling.layouts
			for i := range 5 {
				tick.Set(i + 1)
				p.Frame()
			}
			if got := sibling.layouts - before; got != 0 {
				t.Fatalf("a sibling under the same Cached was laid out %d times in 5 frames that changed nothing in it", got)
			}
		})
	}
}

// A field in a group shares the spare width as a text field does, whether
// it is a text field, a number input or an input group.
func TestButtonGroupGivesEveryFieldTheSpareWidth(t *testing.T) {
	t.Parallel()
	for name, field := range map[string]ggui.Widget{
		"NumberInput": ui.NumberInput(ggui.State(1.0)).Name("Count"),
		"InputGroup":  ui.InputGroup(ui.TextField(ggui.State(""))).Leading(ggui.Text("$")).Name("Amount"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			g := ui.ButtonGroup(field, ui.Button("Open", nil).Outline())
			p := ggui.NewProbe(ggui.Row(ggui.Box(g).Width(400), ggui.Spacer()), ggui.Sz(500, 100))
			defer p.Close()
			open := find(t, p, ggui.RoleButton, "Open").Rect
			if end := open.Origin.X + open.Size.W; end != 400 {
				t.Fatalf("the strip ends at %v, want the 400 its parent gave it", end)
			}
		})
	}
}

// The sides are the field's: a button the field holds is not joined, so
// an input group in a group keeps its own button round.
func TestJoinedSidesStopAtTheControl(t *testing.T) {
	t.Parallel()
	inner := &sidesRecorder{size: ggui.Sz(20, 20)}
	g := ui.ButtonGroup(ui.Button("a", nil), ui.InputGroup(ggui.TextInput(ggui.State(""))).Trailing(inner), ui.Button("b", nil))
	p := ggui.NewProbe(g, ggui.Sz(400, 100))
	defer p.Close()
	p.Frame()
	if inner.sides.Any() {
		t.Fatalf("a part inside a joined field was laid out as joined on %+v", inner.sides)
	}
}
