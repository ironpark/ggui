package ui

import (
	"regexp"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/ironpark/ggui"
)

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
