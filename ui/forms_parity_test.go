package ui

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// watchMotion paints w at its own size and reports where the Motion
// retained for it under slot stands in each frame, or -1 while there is
// none.
func watchMotion(w ggui.Widget, state *ggui.Interactive, slot ggui.Slot[*ggui.Motion], at *float64) ggui.Widget {
	return ggui.Column(watched(w, state, slot, at))
}

func watched(w ggui.Widget, state *ggui.Interactive, slot ggui.Slot[*ggui.Motion], at *float64) ggui.Widget {
	return ggui.FromFuncs(w.Layout, func(dst *ggui.Canvas, r ggui.Rect) {
		dst.Paint(w, r)
		*at = -1
		if m, ok := dst.Retained(state.Anchor(r), slot); ok {
			*at = m.Value(ggui.FrameTime())
		}
	})
}

// A focus ring fades in over the theme's MotionFast rather than switching,
// arrives, and under reduced motion is there at once.
func TestFocusRingEasesIn(t *testing.T) {
	t.Parallel()
	for _, reduced := range []bool{false, true} {
		box := Checkbox(ggui.State(false), "").Name("Agree")
		var ring float64
		root := ggui.Provide(ggui.ReducedMotionKey, reduced, watchMotion(box, &box.Interactive, focusRingSlot, &ring))
		p := ggui.NewProbe(root, ggui.Sz(200, 60))
		p.Advance(0)
		p.Advance(0)
		if ring != 0 {
			t.Fatalf("reduced=%v: an unfocused checkbox has its ring at %v", reduced, ring)
		}
		p.Key("tab")
		p.Advance(0) // the frame the ring starts in
		p.Advance(uitheme.Default().MotionFast / 2)
		if reduced {
			if ring != 1 {
				t.Fatalf("under reduced motion the ring is at %v half way, want 1", ring)
			}
		} else if ring <= 0 || ring >= 1 {
			t.Fatalf("the ring is at %v half way through MotionFast, want between 0 and 1", ring)
		}
		p.Advance(time.Second)
		if ring != 1 {
			t.Fatalf("reduced=%v: the ring settled at %v, want 1", reduced, ring)
		}
		p.Close()
	}
}

// A button's focus ring eases as a checkbox's does: shadcn's button is
// transition-all. A toggle is a button too.
func TestButtonFocusRingEasesIn(t *testing.T) {
	t.Parallel()
	for _, reduced := range []bool{false, true} {
		b := Button("Save", nil)
		var ring float64
		root := ggui.Provide(ggui.ReducedMotionKey, reduced, watchMotion(b, &b.Interactive, focusRingSlot, &ring))
		p := ggui.NewProbe(root, ggui.Sz(200, 60))
		p.Advance(0)
		p.Advance(0)
		if ring > 0 {
			t.Fatalf("reduced=%v: an unfocused button has its ring at %v", reduced, ring)
		}
		p.Key("tab")
		p.Advance(0)
		p.Advance(uitheme.Default().MotionFast / 2)
		if reduced {
			if ring != 1 {
				t.Fatalf("under reduced motion the ring is at %v half way, want 1", ring)
			}
		} else if ring <= 0 || ring >= 1 {
			t.Fatalf("the ring is at %v half way through MotionFast, want between 0 and 1", ring)
		}
		p.Advance(time.Second)
		if ring != 1 {
			t.Fatalf("reduced=%v: the ring settled at %v, want 1", reduced, ring)
		}
		p.Close()
	}
}

// A checkbox's fill eases to Primary when it is ticked.
func TestCheckboxFillEases(t *testing.T) {
	t.Parallel()
	on := ggui.State(false)
	box := Checkbox(on, "").Name("Agree")
	var fill float64
	p := ggui.NewProbe(watchMotion(box, &box.Interactive, checkSlot, &fill), ggui.Sz(200, 60))
	defer p.Close()
	p.Advance(0)
	on.Set(true)
	p.Advance(0)
	p.Advance(uitheme.Default().MotionFast / 2)
	if fill <= 0 || fill >= 1 {
		t.Fatalf("the fill is at %v half way through MotionFast, want between 0 and 1", fill)
	}
	p.Advance(time.Second)
	if fill != 1 {
		t.Fatalf("the fill settled at %v, want 1", fill)
	}
}

// Checkbox, Radio and Switch are invalid inside a Field holding an error,
// and by their own Invalid.
func TestTogglesReadTheFieldsError(t *testing.T) {
	t.Parallel()
	err := ggui.State("")
	box := Checkbox(ggui.State(false), "")
	radio := Radio(ggui.State("a"), "b", "")
	sw := Switch(ggui.State(false), "")
	own := Checkbox(ggui.State(false), "Own").Invalid(true)
	bound := ggui.State(false)
	follows := Switch(ggui.State(false), "Follows").BindInvalid(bound)
	p := ggui.NewProbe(ggui.Column(
		Field("Terms", box).BindError(err), Field("Plan", radio).BindError(err), Field("Alerts", sw).BindError(err), own, follows,
	), ggui.Sz(320, 400))
	defer p.Close()
	p.Frame()
	if box.invalid() || radio.invalid() || sw.invalid() || follows.invalid() {
		t.Fatal("valid controls are marked invalid")
	}
	if !own.invalid() {
		t.Fatal("Invalid(true) did not mark the checkbox invalid")
	}
	err.Set("Required")
	bound.Set(true)
	p.Frame()
	if !box.invalid() || !radio.invalid() || !sw.invalid() {
		t.Fatal("the field's error did not reach the control inside it")
	}
	if !follows.invalid() {
		t.Fatal("BindInvalid did not follow its reader")
	}
	err.Set("")
	p.Frame()
	if box.invalid() || radio.invalid() || sw.invalid() {
		t.Fatal("the invalid state stayed after the error was cleared")
	}
}

func TestSwitchSmall(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	if size := Switch(ggui.State(false), "").Small().Layout(ggui.Loose(ggui.Sz(200, 200)), env); size != ggui.Sz(24, 14) {
		t.Fatalf("a small switch is %v, want 24 by 14", size)
	}
}

// Radios stack by default, as shadcn's RadioGroup does, and line up when
// asked to.
func TestRadiosStackUnlessHorizontal(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	c := ggui.Loose(ggui.Sz(600, 600))
	options := []string{"One", "Two", "Three"}
	one := Radio(ggui.State(""), "One", "One").Layout(c, env)
	column := Radios(ggui.State("")).Options(options).Layout(c, env)
	if want := 3*one.H + 2*uitheme.Default().Space; math.Abs(column.H-want) > 1e-9 {
		t.Fatalf("three options are %v tall, want a column of %v", column.H, want)
	}
	if again := Radios(ggui.State("")).Options(options).Horizontal().Vertical().Layout(c, env); again != column {
		t.Fatalf("Vertical after Horizontal is %v, want the column's %v", again, column)
	}
	if row := Radios(ggui.State("")).Options(options).Horizontal().Layout(c, env); row.H != one.H || row.W <= column.W {
		t.Fatalf("horizontal options are %v, want one row wider than the column's %v", row, column)
	}
}

// An input group is as tall as a text field whatever its addons, and a
// button given as one becomes the ghost, extra small button of shadcn's.
func TestInputGroupKeepsAFieldsHeight(t *testing.T) {
	t.Parallel()
	for _, theme := range []uitheme.Theme{uitheme.Default(), uitheme.Dark()} {
		env := theme.Apply(ggui.Env{})
		c := ggui.Loose(ggui.Sz(400, 400))
		field := TextField(ggui.State("")).Layout(c, env)
		plain := Button("Go", nil)
		kept := Button("Go", nil).Outline().Size(ButtonSmall)
		for name, g := range map[string]*InputGroupWidget{
			"bare":   InputGroup(ggui.TextInput(ggui.State(""))),
			"text":   InputGroup(ggui.TextInput(ggui.State(""))).Leading(ggui.Text("https://")).Trailing(ggui.Text("kg")),
			"button": InputGroup(ggui.TextInput(ggui.State(""))).Trailing(plain),
		} {
			if size := g.Layout(c, env); size.H != field.H || size.W != 400 {
				t.Errorf("%s: the group is %v, want a field's height %v across 400", name, size, field.H)
			}
		}
		if plain.variant != ButtonGhost || plain.size != ButtonExtraSmall || plain.radius != theme.Radius-3 {
			t.Errorf("a default button addon is %s %s with radius %v, want ghost xs with radius %v", plain.variant, plain.size, plain.radius, theme.Radius-3)
		}
		InputGroup(ggui.TextInput(ggui.State(""))).Trailing(kept).Layout(c, env)
		if kept.variant != ButtonOutline || kept.size != ButtonSmall {
			t.Errorf("a button given its own look became %s %s", kept.variant, kept.size)
		}
	}
}

// Top and Bottom addons take rows of their own, and a Multiline group
// grows with its editor.
func TestInputGroupBlockAddons(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	c := ggui.Loose(ggui.Sz(400, 600))
	area := InputGroup(ggui.TextInput(ggui.State(""))).Multiline()
	plain := area.Layout(c, env)
	if single := InputGroup(ggui.TextInput(ggui.State(""))).Layout(c, env); plain.H <= single.H {
		t.Fatalf("a multiline group is %v tall, a single line %v", plain.H, single.H)
	}
	send := Button("Send", nil)
	g := InputGroup(ggui.TextInput(ggui.State(""))).Multiline().Top(ggui.Text("Context")).Bottom(send)
	if size := g.Layout(c, env); size.H <= plain.H+send.Layout(c, env).H {
		t.Fatalf("with addons above and below the group is %v tall, without %v", size.H, plain.H)
	}
	if g.topRect.Origin.Y >= g.rowRect.Origin.Y || g.rowRect.Origin.Y >= g.botRect.Origin.Y {
		t.Fatalf("rows out of order: top %v, editor %v, bottom %v", g.topRect, g.rowRect, g.botRect)
	}
	// A text area keeps its own padding towards a row of addons, and the
	// row its own towards the text area: shadcn's py-2 and py-1.5.
	if above, below := g.rowRect.Origin.Y-g.topRect.Origin.Y-g.topRect.Size.H, g.botRect.Origin.Y-g.rowRect.Origin.Y-g.rowRect.Size.H; math.Abs(above-14) > 1e-9 || math.Abs(below-14) > 1e-9 {
		t.Fatalf("the editor is %v below the top row and %v above the bottom one, want 14 each", above, below)
	}
	line := InputGroup(ggui.TextInput(ggui.State(""))).Bottom(Button("Send", nil))
	line.Layout(c, env)
	if below := line.botRect.Origin.Y - line.rowRect.Origin.Y - line.rowRect.Size.H; math.Abs(below-6) > 1e-9 {
		t.Fatalf("a single line is %v above its bottom row, want 6", below)
	}
	// The button below still takes its own clicks; the rest of the group
	// focuses the editor.
	taps := 0
	value := ggui.State("")
	input := ggui.TextInput(value).Name("Message")
	p := ggui.NewProbe(InputGroup(input).Multiline().Top(ggui.Text("Context")).Bottom(Button("Send", func() { taps++ })), ggui.Sz(400, 300))
	defer p.Close()
	p.Tap("Send")
	if taps != 1 {
		t.Fatalf("the addon button was tapped %d times, want 1", taps)
	}
	p.Click(ggui.Pt(200, 12)) // on the row of the top addon
	if !input.Focused() {
		t.Fatal("a click on an addon's row did not focus the editor")
	}
}

// A field shows its error under its description, turns its label
// destructive, and lays out as a row when Horizontal.
func TestFieldPartsAndOrientation(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	env := theme.Apply(ggui.Env{})
	c := ggui.Loose(ggui.Sz(300, 600))
	f := Field("Email", TextField(ggui.State(""))).Help("Work address").Error("Required")
	f.Layout(c, env)
	if f.note.Layout(c, env).H != f.alert.Layout(c, env).H || !f.shape.noted || !f.shape.alerted {
		t.Fatal("the description and the error are not both shown, in the same text")
	}
	valid := Field("Email", TextField(ggui.State(""))).Help("Work address")
	if with, without := f.Layout(c, env).H, valid.Layout(c, env).H; with != without+f.alert.Layout(c, env).H+theme.Space {
		t.Fatalf("with an error the field is %v tall, without %v: want the error's line and a Space more", with, without)
	}

	sw := Switch(ggui.State(false), "")
	box := Checkbox(ggui.State(false), "")
	p := ggui.NewProbe(ggui.Column(Field("Alerts", sw).Help("By email").Horizontal(), Field("Terms", box).ControlFirst()).Align(ggui.AlignStretch), ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	alerts, ok := p.FindRole(ggui.RoleSwitch, "Alerts")
	terms, ok2 := p.FindRole(ggui.RoleCheckbox, "Terms")
	if !ok || !ok2 {
		t.Fatal("the fields did not name their controls")
	}
	if right := alerts.Rect.Origin.X + alerts.Rect.Size.W; right != 300 {
		t.Fatalf("a horizontal field's control ends at %v, want the field's end at 300", right)
	}
	if terms.Rect.Origin.X != 0 {
		t.Fatalf("a control-first field's control starts at %v, want 0", terms.Rect.Origin.X)
	}
}

// FieldGroup, FieldSet and FieldSeparator space a form as shadcn's do.
func TestFieldGroupAndSet(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	env := theme.Apply(ggui.Env{})
	c := ggui.Loose(ggui.Sz(300, 900))
	one := Field("Name", TextField(ggui.State(""))).Layout(c, env)
	group := FieldGroup(Field("Name", TextField(ggui.State(""))), FieldSeparator().Label("Or"), Field("Email", TextField(ggui.State("")))).Layout(c, env)
	if want := 2*one.H + theme.Space/2 + 2*theme.Space*2.5; group.H != want || group.W != 300 {
		t.Fatalf("the group is %v, want 300 by %v", group, want)
	}
	bare := FieldSet(Field("Name", TextField(ggui.State(""))), Field("Email", TextField(ggui.State("")))).Layout(c, env)
	if want := 2*one.H + theme.Space*2; bare.H != want {
		t.Fatalf("a set of two fields is %v tall, want %v", bare.H, want)
	}
	set := FieldSet(Field("Name", TextField(ggui.State("")))).Legend("Profile").Description("How others see you.")
	p := ggui.NewProbe(set, ggui.Sz(300, 300))
	defer p.Close()
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleGroup, "Profile"); !ok {
		t.Fatal("the set is not a group named by its legend")
	}
}

// The slider's track runs the whole width with the thumb inside it, and the
// halo belongs to the thumb: the pointer elsewhere on the track leaves it.
func TestSliderTrackAndThumbHalo(t *testing.T) {
	t.Parallel()
	r := ggui.Rct(ggui.Pt(10.0, 0.0), ggui.Sz(200, 20))
	g := newSliderGeometry(r, 6, false, false)
	if tr := g.track(0, 1); tr.Origin.X != 10 || tr.Size.W != 200 || tr.Size.H != 4 {
		t.Fatalf("the track is %v, want the full width, 4 thick", tr)
	}
	if lo, hi := g.point(0).X, g.point(1).X; lo != 16 || hi != 204 {
		t.Fatalf("the thumb runs from %v to %v, want its edge on the track's ends", lo, hi)
	}
	if tr := g.track(0, .5); tr.Origin.X != 10 || tr.Origin.X+tr.Size.W != g.point(.5).X {
		t.Fatalf("the range is %v, want from the track's start to the thumb", tr)
	}
	rtl := newSliderGeometry(r, 6, false, true)
	if tr := rtl.track(0, .5); tr.Origin.X != rtl.point(.5).X || tr.Origin.X+tr.Size.W != 210 {
		t.Fatalf("the mirrored range is %v, want from the thumb to the track's far end", tr)
	}

	s := Slider(ggui.State(0.0), 0, 100)
	var halo float64
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, watched(s, &s.Interactive, sliderHaloSlot, &halo)), ggui.Sz(200, 20))
	defer p.Close()
	p.Move(ggui.Pt(150, 10))
	p.Frame()
	if halo != 0 {
		t.Fatalf("the pointer away from the thumb raised its halo to %v", halo)
	}
	p.Move(ggui.Pt(8, 10))
	p.Frame()
	if halo != 1 {
		t.Fatalf("the pointer over the thumb left its halo at %v", halo)
	}
}

// An OTP field's slots follow the theme's field height unless SlotSize says
// otherwise.
func TestInputOTPSlotFollowsTheTheme(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	c := ggui.Loose(ggui.Sz(600, 200))
	field := TextField(ggui.State("")).Layout(c, theme.Apply(ggui.Env{}))
	if size := InputOTP(ggui.State(""), 4).Layout(c, theme.Apply(ggui.Env{})); size != ggui.Sz(4*field.H, field.H) {
		t.Fatalf("four slots are %v, want %v square each", size, field.H)
	}
	theme.FieldPad = ggui.Insets(10, 12)
	if size := InputOTP(ggui.State(""), 4).Layout(c, theme.Apply(ggui.Env{})); size != ggui.Sz(160, 40) {
		t.Fatalf("with a taller field padding four slots are %v, want 160 by 40", size)
	}
	if size := InputOTP(ggui.State(""), 4).SlotSize(24).Layout(c, theme.Apply(ggui.Env{})); size != ggui.Sz(96, 24) {
		t.Fatalf("SlotSize(24) gave %v", size)
	}
}

// The form controls write no property back and forth in Layout, so a Cached
// parent holds through frames that change nothing in them: see
// TestControlsLeaveACachedParentHeld.
func TestFormControlsLeaveACachedParentHeld(t *testing.T) {
	t.Parallel()
	options := []string{"a", "b", "c"}
	for name, w := range map[string]ggui.Widget{
		"Checkbox":             Checkbox(ggui.State(true), "c"),
		"Checkbox invalid":     Checkbox(ggui.State(false), "c").Invalid(true),
		"Switch small":         Switch(ggui.State(true), "s").Small(),
		"Radios":               Radios(ggui.State("a")).Options(options),
		"Radios horizontal":    Radios(ggui.State("a")).Options(options).Horizontal(),
		"NumberInput":          NumberInput(ggui.State(1.0)),
		"InputOTP":             InputOTP(ggui.State("12"), 6).Groups(3, 3),
		"InputGroup":           InputGroup(ggui.TextInput(ggui.State("a"))).Leading(ggui.Text("$")).Trailing(Button("Go", nil)),
		"InputGroup multiline": InputGroup(ggui.TextInput(ggui.State("a"))).Multiline().Top(ggui.Text("t")).Bottom(Button("Send", nil)),
		"Field":                Field("Name", TextField(ggui.State("a"))).Help("help").Error("error"),
		"Field horizontal":     Field("Name", Switch(ggui.State(true), "")).Help("help").Horizontal(),
		"FieldSet":             FieldSet(Field("Name", TextField(ggui.State("a"))), FieldSeparator().Label("or")).Legend("Set").Description("d"),
		"FieldGroup":           FieldGroup(Field("Name", Checkbox(ggui.State(true), "")).ControlFirst(), FieldSeparator()),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			layouts := 0
			sibling := ggui.FromFuncs(func(c ggui.Constraints, _ ggui.Env) ggui.Size { layouts++; return c.Constrain(ggui.Sz(10, 10)) }, func(*ggui.Canvas, ggui.Rect) {})
			tick := ggui.State(0)
			root := ggui.Column(ggui.Cached(ggui.Column(w, sibling)), ggui.TextOf(ggui.Map[int, string](tick, strconv.Itoa)))
			p := ggui.NewProbe(root, ggui.Sz(600, 600))
			defer p.Close()
			for range 3 {
				p.Frame()
			}
			before := layouts
			for i := range 5 {
				tick.Set(i + 1)
				p.Frame()
			}
			if got := layouts - before; got != 0 {
				t.Fatalf("a sibling under the same Cached was laid out %d times in 5 frames that changed nothing in it", got)
			}
		})
	}
}

// A Radios group bound to a validity state marks every option as it changes,
// without a rebuild.
func TestRadiosBindInvalidMarksEveryOption(t *testing.T) {
	t.Parallel()
	bad := ggui.State(false)
	g := Radios(ggui.State("a")).Options([]string{"a", "b"}).BindInvalid(bad)
	p := ggui.NewProbe(g, ggui.Sz(320, 200))
	defer p.Close()
	p.Frame()
	for _, r := range g.radios {
		if r.invalid() {
			t.Fatal("an option is invalid before the binding says so")
		}
	}
	bad.Set(true)
	p.Frame()
	for _, r := range g.radios {
		if !r.invalid() {
			t.Fatal("BindInvalid did not reach an option")
		}
	}
}
