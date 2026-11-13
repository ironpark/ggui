package ui

import (
	"math"
	"slices"
	"testing"

	"github.com/ironpark/ggui"
)

// A choice card is its control's label: a click anywhere in it acts on the
// control, it names a control that has no name from its title, and the
// pointer crossing from the card onto the control leaves it under the
// pointer.
func TestFieldCardActsAsItsControlsLabel(t *testing.T) {
	t.Parallel()
	plan := ggui.State("free")
	free := Field("Free", Radio(plan, "free", "")).Help("For trying things out.").Card()
	pro := FieldCard(Radio(plan, "pro", ""), FieldContent(FieldTitle("Pro"), FieldDescription("For growing businesses.")))
	p := ggui.NewProbe(ggui.Box(ggui.Column(free, pro).Gap(12).Align(ggui.AlignStretch)).Padding(ggui.Insets(20)), ggui.Sz(360, 240))
	defer p.Close()
	radio, ok := p.FindRole(ggui.RoleRadio, "Pro")
	if !ok {
		t.Fatal("the card's radio did not take its title for a name")
	}
	// shadcn's p-2.5 inside a one-pixel border.
	if got := free.card.pad; got != ggui.Insets(11) {
		t.Fatalf("the card pads its field by %v, want 11 all round", got)
	}
	if got := radio.Rect.Origin.X + radio.Rect.Size.W; got != 360-20-11 {
		t.Fatalf("the radio ends at %v, want it at the card's end, 11px inside", got)
	}
	text := ggui.Pt(60, radio.Rect.Center().Y+20)
	p.Move(text)
	if !pro.Hovered {
		t.Fatal("the card is not hovered over its description")
	}
	p.Click(text)
	if plan.Get() != "pro" {
		t.Fatalf("a click on the card's description left the choice at %q", plan.Get())
	}
	p.Move(radio.Rect.Center())
	r := pro.control.(*RadioWidget[string])
	if !r.hovered() {
		t.Fatal("the radio in the card is not hovered under the pointer")
	}
	if _, ok := p.FindRole(ggui.RoleRadio, "Free"); !ok {
		t.Fatal("Field(...).Card() lost its control's name")
	}
	p.Tap("Free")
	if plan.Get() != "free" {
		t.Fatal("the radio in a card no longer takes a click itself")
	}
	if !r.inCard {
		t.Fatal("the control was not told it sits in a card, and draws a focus ring of its own")
	}
}

// A card around a disabled control takes no clicks.
func TestFieldCardFollowsADisabledControl(t *testing.T) {
	t.Parallel()
	on := ggui.State(false)
	card := Field("Airplane mode", Switch(on, "").Disabled(true)).Card()
	p := ggui.NewProbe(ggui.Box(card).Padding(ggui.Insets(20)), ggui.Sz(360, 120))
	defer p.Close()
	p.Click(ggui.Pt(60, 40))
	if on.Get() || card.card.Hovered {
		t.Fatal("a card around a disabled switch reacted to the pointer")
	}
}

// Errors drops empty and repeated messages: one left reads as Error's
// does, several as a list, and any makes the field invalid.
func TestFieldErrorsListsUniqueMessages(t *testing.T) {
	t.Parallel()
	errs := ggui.State([]string{})
	box := Checkbox(ggui.State(false), "")
	f := Field("Password", box).BindErrors(errs)
	p := ggui.NewProbe(ggui.Box(f).Padding(ggui.Insets(20)), ggui.Sz(360, 240))
	defer p.Close()
	p.Advance(0)
	if f.shape.alerted || box.fieldInvalid {
		t.Fatal("a field with no errors is invalid")
	}
	errs.Set([]string{"Too short.", "", "Too short."})
	p.Advance(0)
	if !f.shape.alerted || f.shape.listed || !box.fieldInvalid {
		t.Fatalf("one message left: alerted=%v listed=%v invalid=%v, want a single error", f.shape.alerted, f.shape.listed, box.fieldInvalid)
	}
	errs.Set([]string{"Too short.", "Add a number.", "Too short."})
	p.Advance(0)
	if want := []string{"Too short.", "Add a number."}; !f.shape.listed || !slices.Equal(f.alerts.messages, want) {
		t.Fatalf("listed=%v messages=%q, want the list %q", f.shape.listed, f.alerts.messages, want)
	}
	// shadcn's ml-4 list with gap-1: the lines 4px apart, 16px in.
	size := f.alerts.Layout(ggui.Loose(ggui.Sz(300, 300)), ggui.Env{})
	if got, want := size.H, f.alerts.sizes[0].H+f.alerts.sizes[1].H+4; math.Abs(got-want) > .01 {
		t.Fatalf("the list is %v tall, want its two lines and 4px, %v", got, want)
	}
	if got := size.W - max(f.alerts.sizes[0].W, f.alerts.sizes[1].W); got != 16 {
		t.Fatalf("the list indents its messages by %v, want 16", got)
	}
}
