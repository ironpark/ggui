// Package controldemo contains native adaptations of the shadcn Carousel and
// Input OTP documentation examples. No browser or runtime dependency is used.
package controldemo

import (
	"fmt"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

var Names = []string{"carousel-demo", "carousel-size", "carousel-spacing", "carousel-orientation", "carousel-api", "carousel-plugin", "carousel-multiple", "carousel-rtl", "carousel-loop", "input-otp-demo", "input-otp-pattern", "input-otp-separator", "input-otp-disabled", "input-otp-controlled", "input-otp-invalid", "input-otp-four-digits", "input-otp-alphanumeric", "input-otp-form", "input-otp-rtl"}

type Demo struct {
	Widget   ggui.Widget
	Carousel *ui.CarouselWidget
	OTP      *ui.InputOTPWidget
	Value    *ggui.StateValue[string]
}

func Title(name string) string {
	words := strings.Split(name, "-")
	for i, s := range words {
		if s == "otp" {
			words[i] = "OTP"
		} else if s == "api" {
			words[i] = "API"
		} else if s == "rtl" {
			words[i] = "RTL"
		} else {
			words[i] = strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return strings.Join(words, " ")
}
func Build(name string) Demo {
	d := Demo{}
	if strings.HasPrefix(name, "carousel-") {
		selected := ggui.State(0)
		items := make([]ggui.Widget, 5)
		for i := range items {
			slide := ui.Card(ggui.Center(ggui.Title(fmt.Sprint(i + 1)).Size(30))).Pad(24)
			it := ui.CarouselItem(ggui.Box(slide).Pad(4))
			switch name {
			case "carousel-size", "carousel-multiple":
				it.BasisWhen(func(s ggui.Size) float64 {
					if s.W >= 360 {
						return 1. / 3
					}
					return .5
				})
			case "carousel-spacing", "carousel-orientation":
				it.Basis(.5)
			}
			items[i] = it
		}
		c := ui.Carousel(selected, ui.CarouselContent(items...)).Height(288).Name("Slides")
		switch name {
		case "carousel-size", "carousel-multiple":
			c.Align(0).Height(180)
		case "carousel-spacing":
			c.Gap(4).Height(180)
		case "carousel-orientation":
			c.Vertical().Align(0).Gap(4).Height(366)
		case "carousel-plugin":
			c.Autoplay(2 * time.Second).StopOnInteraction(false)
		case "carousel-rtl":
			c.RTL(true)
		case "carousel-loop":
			c.Loop(true)
		}
		d.Carousel = c
		d.Widget = c
		if name == "carousel-api" {
			label := selected.Map(func(i int) string { return fmt.Sprintf("Slide %d of 5", i+1) })
			d.Widget = ggui.Column(c, ggui.Box(ggui.Center(ggui.Textf("%s", label).AsCaption())).Height(20)).Gap(16).Align(ggui.AlignStretch)
		}
		if name == "carousel-plugin" {
			d.Widget = ggui.Column(c, ggui.Box(ggui.Center(ggui.Row(ui.Button("Pause", c.Pause).Outline(), ui.Button("Play", c.Play).Outline()).Gap(8))).Height(36)).Gap(16).Align(ggui.AlignStretch)
		}
	} else {
		initial := ""
		switch name {
		case "input-otp-demo", "input-otp-disabled", "input-otp-rtl":
			initial = "123456"
		case "input-otp-invalid":
			initial = "000000"
		}
		value := ggui.State(initial)
		n := 6
		if name == "input-otp-four-digits" {
			n = 4
		}
		o := ui.InputOTP(value, n).Name("Verification code")
		d.OTP, d.Value = o, value
		body := ggui.Widget(o)
		switch name {
		case "input-otp-separator":
			o.Groups(2, 2, 2)
		case "input-otp-disabled":
			o.Disabled(true)
		case "input-otp-invalid":
			o.Invalid(true)
		case "input-otp-alphanumeric":
			o.Alphanumeric()
		case "input-otp-rtl":
			o.RTL(true)
		case "input-otp-controlled":
			message := value.Map(func(s string) string {
				if s == "" {
					return "Enter your one-time password."
				}
				return "You entered: " + s
			})
			body = ggui.Column(o, ggui.Textf("%s", message).AsCaption()).Gap(12)
		case "input-otp-pattern":
			body = ui.Field("Digits Only", o)
		case "input-otp-form":
			errorText := ggui.State("")
			status := ggui.State("")
			o.Groups(3, 3).BindInvalid(errorText.Map(func(s string) bool { return s != "" })).OnChange(func(string) { errorText.Set(""); status.Set("") })
			verify := func() {
				if len(value.Get()) != 6 {
					errorText.Set("Enter all six digits.")
					return
				}
				status.Set("Code complete — ready for your verification handler.")
			}
			o.OnSubmit(func(string) { verify() })
			body = ggui.Column(ggui.Title("Verify your login"), ggui.Text("Enter the verification code we sent to m@example.com."), ui.Field("Verification code", o).BindError(errorText), ggui.Row(ui.Button("Verify", verify), ui.Button("Resend code", func() { value.Set(""); status.Set("Demo code reset.") }).Outline()).Gap(12), ggui.Textf("%s", status).AsCaption()).Gap(16)
		}
		d.Widget = ggui.Box(ggui.Center(body)).Height(288)
	}
	d.Widget = ui.Card(ggui.Column(ggui.Title(Title(name)).Size(20), ggui.Caption("Native ggui · shadcn reference"), d.Widget).Gap(20).Align(ggui.AlignStretch)).Pad(24)
	return d
}
