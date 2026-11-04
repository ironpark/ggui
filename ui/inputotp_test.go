package ui

import (
	"regexp"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/ironpark/ggui"
)

func TestInputOTPPasteSelectionUndo(t *testing.T) {
	t.Parallel()
	value := ggui.State("")
	completed, changed := 0, 0
	o := InputOTP(value, 6).Groups(3, 3).OnChange(func(string) { changed++ }).OnComplete(func(string) { completed++ })
	p := ggui.NewProbe(o, ggui.Sz(320, 80))
	defer p.Close()
	clip := p.Clipboard()
	p.Click(ggui.Pt(15, 16))
	clip.Write("12-34 56extra7")
	p.Key("cmd+v")
	if value.Get() != "123456" || completed != 1 || changed != 1 {
		t.Fatalf("paste %q completed %d changed %d", value.Get(), completed, changed)
	}
	p.Click(ggui.Pt(48, 16))
	clip.Write("9")
	p.Key("cmd+v")
	if value.Get() != "193456" {
		t.Fatalf("slot replacement %q", value.Get())
	}
	p.Key("cmd+z")
	if value.Get() != "123456" {
		t.Fatal("undo", value.Get())
	}
	p.Type(ggui.Mods{}, ggui.KeyBackspace)
	if value.Get() != "13456" {
		t.Fatal("backspace", value.Get())
	}
	p.Key("cmd+a", "cmd+c")
	if clip.Read() != value.Get() {
		t.Fatal("copy")
	}
	p.Key("cmd+x")
	if value.Get() != "" {
		t.Fatal("cut")
	}
}
func TestInputOTPAccessibilityAndExternalValue(t *testing.T) {
	t.Parallel()
	value := ggui.State("")
	o := InputOTP(value, 4).Alphanumeric().Name("Code")
	p := ggui.NewProbe(o, ggui.Sz(300, 80))
	defer p.Close()
	p.Frame()
	if !o.Act(ggui.Action{Kind: ggui.ActionSetValue, Text: "A한1 b2!3"}) || value.Get() != "A1b2" {
		t.Fatal("filtered accessibility", value.Get())
	}
	value.Set("Z9")
	p.Frame()
	if o.Describe().Value != "Z9" {
		t.Fatal("external binding")
	}
	o.Disabled(true)
	p.Frame()
	if o.Act(ggui.Action{Kind: ggui.ActionSetValue, Text: "0000"}) {
		t.Fatal("disabled edit")
	}
	if !o.Describe().Disabled {
		t.Fatal("disabled semantics")
	}
}
func TestInputOTPCompositionSizesRTL(t *testing.T) {
	t.Parallel()
	for _, rtl := range []bool{false, true} {
		for _, width := range []float64{0, 20, 320} {
			o := InputOTP(ggui.State("한글12"), 4, InputOTPGroup(InputOTPSlot(0), InputOTPSlot(1)), InputOTPSeparator(), InputOTPGroup(InputOTPSlot(2), InputOTPSlot(3))).Accept(func(r rune) bool { return true }).RTL(rtl)
			p := ggui.NewProbe(o, ggui.Sz(width, 32))
			p.Frame()
			if len(o.cells) != 4 {
				t.Fatal("slots")
			}
			if width > 0 && rtl && o.cells[0].rect.Origin.X <= o.cells[3].rect.Origin.X {
				t.Fatal("RTL layout")
			}
			o.input.Select(3, 6)
			if o.input.EditingState().Caret != 6 {
				t.Fatal("UTF-8 selection")
			}
			p.Close()
		}
	}
	o := InputOTP(ggui.State(""), 4).Pattern(regexp.MustCompile(`[A-F0-9]`))
	if s := o.normalize("AbC-17F"); s != "AC17" {
		t.Fatal(s)
	}
}
func TestInputOTPInvalidAndDisabledBindings(t *testing.T) {
	t.Parallel()
	locked, bad := ggui.State(false), ggui.State(false)
	o := InputOTP(ggui.State("123456"), 6).BindDisabled(locked).BindInvalid(bad)
	p := ggui.NewProbe(o, ggui.Sz(300, 40))
	defer p.Close()
	p.Frame()
	locked.Set(true)
	bad.Set(true)
	p.Frame()
	if !o.Describe().Disabled || !o.invalid {
		t.Fatal("reactive state")
	}
	o.Disabled(false).Invalid(false)
	p.Frame()
	if o.Describe().Disabled || o.invalid {
		t.Fatal("explicit setters")
	}
}

func TestInputOTPCompleteAfterExternalReset(t *testing.T) {
	t.Parallel()
	value := ggui.State("")
	calls := 0
	o := InputOTP(value, 4).OnComplete(func(string) { calls++ })
	p := ggui.NewProbe(o, ggui.Sz(200, 40))
	defer p.Close()
	p.Frame()
	o.Act(ggui.Action{Kind: ggui.ActionSetValue, Text: "1234"})
	o.Act(ggui.Action{Kind: ggui.ActionSetValue, Text: "1234"})
	if calls != 1 {
		t.Fatal("unchanged edit repeated completion")
	}
	value.Set("")
	p.Frame()
	o.Act(ggui.Action{Kind: ggui.ActionSetValue, Text: "1234"})
	if calls != 2 {
		t.Fatal("reset prevented completion")
	}
}

func TestInputOTPSlotSizeSetsTheStripWidth(t *testing.T) {
	t.Parallel()
	o := InputOTP(ggui.State(""), 4).SlotSize(48)
	if size := o.Layout(ggui.Loose(ggui.Sz(400, 100)), ggui.Env{}); size.W != 4*48 || size.H != 48 {
		t.Fatalf("four 48px slots laid out %v, want 192x48", size)
	}
	o.SlotSize(-3)
	if size := o.Layout(ggui.Loose(ggui.Sz(400, 100)), ggui.Env{}); size.W != 4 || size.H != 1 {
		t.Fatalf("a negative slot size laid out %v, want the 1px minimum", size)
	}
}

func TestInputOTPPlaceholderFillsOnlyEmptySlots(t *testing.T) {
	t.Parallel()
	// A slot shows a glyph when its label measures wider than nothing.
	shown := func(o *InputOTPWidget) []bool {
		p := ggui.NewProbe(o, ggui.Sz(300, 40))
		defer p.Close()
		p.Frame()
		out := make([]bool, len(o.sizes))
		for i, s := range o.sizes {
			out[i] = s.W > 0
		}
		return out
	}
	if got := shown(InputOTP(ggui.State("12"), 4)); !slices.Equal(got, []bool{true, true, false, false}) {
		t.Fatalf("without a placeholder slots show %v, want only the two typed", got)
	}
	if got := shown(InputOTP(ggui.State("12"), 4).Placeholder("abc")); !slices.Equal(got, []bool{true, true, true, false}) {
		t.Fatalf("with a three-rune placeholder slots show %v, want the third filled by it", got)
	}
}

func TestInputOTPSubmitsOnEnterThroughItsEditor(t *testing.T) {
	t.Parallel()
	value := ggui.State("")
	submitted := ""
	o := InputOTP(value, 4).OnSubmit(func(s string) { submitted = s })
	p := ggui.NewProbe(o, ggui.Sz(300, 40))
	defer p.Close()
	p.Click(ggui.Pt(10, 16))
	p.Clipboard().Write("4821")
	p.Key("cmd+v")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if submitted != "4821" {
		t.Fatalf("Enter submitted %q, want 4821", submitted)
	}
	if got := o.Input().EditingState().Text; got != "4821" {
		t.Fatalf("Input() editor holds %q, want the bound value", got)
	}
}

func TestInputOTPDragSelectsARunOfSlots(t *testing.T) {
	t.Parallel()
	value := ggui.State("123456")
	o := InputOTP(value, 6)
	p := ggui.NewProbe(o, ggui.Sz(300, 40))
	defer p.Close()
	p.Frame()
	if !o.CaptureTouchDrag() {
		t.Fatal("an enabled code field must capture touch drags to select slots")
	}
	// Slots are 32px wide: press on the second and drag to the fourth.
	p.Press(ggui.Pt(48, 16))
	p.Move(ggui.Pt(112, 16))
	p.Release(ggui.Pt(112, 16))
	p.Clipboard().Write("0")
	p.Key("cmd+v")
	if got := ggui.Untrack(value.Get); got != "1056" {
		t.Fatalf("pasting over slots 2-4 gave %q, want 1056", got)
	}
	o.Disabled(true)
	p.Frame()
	if o.CaptureTouchDrag() {
		t.Fatal("a disabled code field captured a touch drag")
	}
}

func TestInputOTPGroupsMustCoverEverySlotOnce(t *testing.T) {
	t.Parallel()
	for name, o := range map[string]*InputOTPWidget{
		"missing":   InputOTP(ggui.State(""), 3, InputOTPGroup(InputOTPSlot(0), InputOTPSlot(1))),
		"duplicate": InputOTP(ggui.State(""), 2, InputOTPGroup(InputOTPSlot(0), InputOTPSlot(0))),
		"range":     InputOTP(ggui.State(""), 2, InputOTPGroup(InputOTPSlot(0), InputOTPSlot(2))),
		"sum":       InputOTP(ggui.State(""), 4).Groups(2, 1),
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: an invalid composition laid out without panicking", name)
				}
			}()
			o.Layout(ggui.Loose(ggui.Sz(300, 40)), ggui.Env{})
		}()
	}
}

// FuzzInputOTPNormalizeKeepsOnlyAcceptedRunes feeds random pasted text
// through the code field's filter: the result is valid UTF-8, holds only
// runes the mode accepts, is capped at the slot count, keeps the accepted
// runes in their order, and filtering it again changes nothing.
func FuzzInputOTPNormalizeKeepsOnlyAcceptedRunes(f *testing.F) {
	f.Add("12-34 56extra7", uint8(6), uint8(0))
	f.Add("A한1 b2!3", uint8(4), uint8(1))
	f.Add("AbC-17F", uint8(4), uint8(2))
	f.Add("\xff\xfe12", uint8(3), uint8(0))
	f.Add("", uint8(0), uint8(1))
	f.Add("０１２３", uint8(2), uint8(0)) // full-width digits are not ASCII
	hex := regexp.MustCompile(`[A-F0-9]`)
	f.Fuzz(func(t *testing.T, text string, n, mode uint8) {
		o := InputOTP(ggui.State(""), int(n%12))
		accept := func(r rune) bool { return r >= '0' && r <= '9' }
		switch mode % 3 {
		case 1:
			o.Alphanumeric()
			accept = func(r rune) bool {
				return r < utf8.RuneSelf && (r >= '0' && r <= '9' || r|0x20 >= 'a' && r|0x20 <= 'z')
			}
		case 2:
			o.Pattern(hex)
			accept = func(r rune) bool { return r >= '0' && r <= '9' || r >= 'A' && r <= 'F' }
		}
		got := o.normalize(text)
		var want []rune
		for _, r := range text {
			if accept(r) && len(want) < o.maxLength {
				want = append(want, r)
			}
		}
		if got != string(want) {
			t.Fatalf("normalize(%q) with %d slots, mode %d = %q, want %q", text, o.maxLength, mode%3, got, string(want))
		}
		if !utf8.ValidString(got) || utf8.RuneCountInString(got) > o.maxLength {
			t.Fatalf("normalize(%q) = %q: invalid UTF-8 or longer than %d", text, got, o.maxLength)
		}
		if again := o.normalize(got); again != got {
			t.Fatalf("normalize is not idempotent: %q then %q", got, again)
		}
	})
}
