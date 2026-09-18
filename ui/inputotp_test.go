package ui

import (
	"github.com/ironpark/ggui"
	"regexp"
	"runtime"
	"testing"
)

func otpCmd() ggui.Mods {
	if runtime.GOOS == "darwin" {
		return ggui.Mods{Meta: true}
	}
	return ggui.Mods{Ctrl: true}
}
func TestInputOTPPasteSelectionUndo(t *testing.T) {
	clip := &ggui.MemoryClipboard{}
	ggui.SetClipboard(clip)
	defer ggui.SetClipboard(&ggui.MemoryClipboard{})
	value := ggui.State("")
	completed, changed := 0, 0
	o := InputOTP(value, 6).Groups(3, 3).OnChange(func(string) { changed++ }).OnComplete(func(string) { completed++ })
	p := ggui.NewProbe(o, ggui.Sz(320, 80))
	defer p.Close()
	p.Click(ggui.Pt(15, 16))
	clip.Write("12-34 56extra7")
	p.Type(otpCmd(), ggui.KeyV)
	if value.Get() != "123456" || completed != 1 || changed != 1 {
		t.Fatalf("paste %q completed %d changed %d", value.Get(), completed, changed)
	}
	p.Click(ggui.Pt(48, 16))
	clip.Write("9")
	p.Type(otpCmd(), ggui.KeyV)
	if value.Get() != "193456" {
		t.Fatalf("slot replacement %q", value.Get())
	}
	p.Type(otpCmd(), ggui.KeyZ)
	if value.Get() != "123456" {
		t.Fatal("undo", value.Get())
	}
	p.Type(ggui.Mods{}, ggui.KeyBackspace)
	if value.Get() != "13456" {
		t.Fatal("backspace", value.Get())
	}
	p.Type(otpCmd(), ggui.KeyA, ggui.KeyC)
	if clip.Read() != value.Get() {
		t.Fatal("copy")
	}
	p.Type(otpCmd(), ggui.KeyX)
	if value.Get() != "" {
		t.Fatal("cut")
	}
}
func TestInputOTPAccessibilityAndExternalValue(t *testing.T) {
	value := ggui.State("")
	o := InputOTP(value, 4).Alphanumeric().Named("Code")
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
	locked, bad := ggui.State(false), ggui.State(false)
	o := InputOTP(ggui.State("123456"), 6).DisabledWhen(locked).InvalidWhen(bad)
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
