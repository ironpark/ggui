package main

import (
	"fmt"
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// newSpecimens builds once: theme changes and responsive layout reuse every
// control, preserving drafts, selected tabs, keyboard focus and scroll position.
func newSpecimens() ggui.Widget {
	status := ggui.State("Choose a button to inspect its active and focus states.")
	name, email := ggui.State("Alex Morgan"), ggui.State("alex@example.com")
	tab, volume := ggui.State(0), ggui.State(60.0)
	updates, agreed := ggui.State(true), ggui.State(false)
	formStatus := ggui.State("Your changes stay in this preview.")
	card := func(title, description string, body ggui.Widget) ggui.Widget {
		return ui.Card(widget.Column(
			widget.Column(ui.Heading(title), ui.Caption(description)).Gap(5).Stretch(),
			body,
		).Gap(20).Stretch()).Pad(20)
	}
	typography := card("Typography", "A type scale, with your selected fonts.", widget.Column(
		ui.Caption("TITLE"), ui.Title("Make something great."),
		ui.Divider(), ui.Caption("HEADING"), ui.Heading("A fresh perspective"),
		widget.Text("Good design starts with clear words. This body text inherits the preview's font and foreground color."),
		ui.Caption("Caption · small details that make a difference."),
		ui.Mono("const theme = \"yours\""),
	).Gap(12).Stretch())
	var swatches []ggui.Widget
	for _, name := range []string{"Background", "Foreground", "Primary", "Secondary", "Muted", "Accent", "Border", "Destructive"} {
		swatches = append(swatches, specimenSwatch(name))
	}
	colors := card("Colors", "Eight semantic roles, live from the current theme.",
		widget.Wrap(swatches...).Gap(12).RunGap(12))
	button := func(label string) *ui.ButtonWidget {
		return ui.Button(label, func() { status.Set(label + " button pressed.") }).Name("Specimen " + label)
	}
	buttons := card("Buttons", "Compare contrast, borders, corners and feedback.", widget.Column(
		widget.Wrap(button("Primary"), button("Secondary").Secondary(), button("Outline").Outline(),
			button("Ghost").Ghost(), button("Destructive").Destructive(), button("Link").Link()).Gap(8).RunGap(8),
		ui.Divider(), widget.Wrap(button("Small").Size(ui.ButtonSmall), button("Large").Size(ui.ButtonLarge),
			ui.Button("Disabled", nil).Disabled(true).Name("Specimen disabled")).Gap(8).RunGap(8),
		ui.CaptionOf(status).Role(ggui.RoleStatus),
	).Gap(16).Stretch())
	profile := widget.Column(
		ui.Field("Full name", ui.TextField(name).Name("Specimen full name").MinWidth(0)),
		ui.Field("Email", ui.TextField(email).Name("Specimen email").MinWidth(0).Placeholder("you@example.com")),
		ui.Checkbox(agreed, "I agree to the terms").Name("Specimen terms"),
		ui.Button("Save profile", func() { formStatus.Set("Profile saved for " + ggui.Peek(name) + ".") }).Name("Save specimen profile"),
	).Gap(16).Stretch()
	preferences := widget.Column(
		ui.Switch(updates, "Email updates").Name("Specimen email updates"),
		widget.Column(ui.CaptionOf(ggui.Map(volume, func(v float64) string { return fmt.Sprintf("Volume · %.0f%%", v) })),
			ui.Slider(volume, 0, 100).Step(5).Name("Specimen volume")).Gap(12).Stretch(),
		ui.Progress(ggui.Map(volume, func(v float64) float64 { return v / 100 })),
		ui.Button("Reset preferences", func() { updates.Set(true); volume.Set(60); formStatus.Set("Preferences reset.") }).Outline().Name("Reset specimen preferences"),
	).Gap(20).Stretch()
	forms := card("Forms", "Try editing, selecting and moving between tabs.", widget.Column(
		ui.Tabs(tab, ui.Tab("Profile", profile), ui.Tab("Preferences", preferences)).Name("Specimen form tabs"),
		ui.CaptionOf(formStatus).Role(ggui.RoleStatus),
	).Gap(16).Stretch())
	wide := widget.Row(
		widget.Expanded(widget.Column(typography, colors).Gap(16).Stretch()),
		widget.Expanded(widget.Column(buttons, forms).Gap(16).Stretch()),
	).Gap(16).Align(widget.AlignStart)
	narrow := widget.Column(typography, colors, buttons, forms).Gap(16).Stretch()
	return widget.Scroll(widget.Padding(widget.Responsive(760, wide, narrow), 24))
}

// Swatches resolve inherited colors during layout; no global theme subscription
// or reconstruction of the preview is needed when the editor changes a token.
func specimenSwatch(name string) ggui.Widget {
	fill := widget.Box().Height(48)
	hex := ui.Mono("").Size(11)
	body := widget.Column(fill, ui.Caption(name), hex).Gap(5).Stretch()
	box := widget.Box(body).Width(112)
	return ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
		t := theme.Of(env)
		var value color.Color
		switch name {
		case "Background":
			value = t.Bg
		case "Foreground":
			value = t.Fg
		case "Primary":
			value = t.Primary
		case "Secondary":
			value = t.Secondary
		case "Muted":
			value = t.Muted
		case "Accent":
			value = t.Accent
		case "Border":
			value = t.Border
		case "Destructive":
			value = t.Destructive
		}
		fill.Fill(value).Border(1, t.Border).Radius(t.Radius)
		rgb := color.NRGBAModel.Convert(value).(color.NRGBA)
		hex.Content(fmt.Sprintf("#%02X%02X%02X", rgb.R, rgb.G, rgb.B))
		return box.Layout(c, env)
	}, func(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(box, r) })
}
