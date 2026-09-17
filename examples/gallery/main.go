// Command gallery shows the layout widgets and the ui controls together:
// Wrap and Grid, every control bound to a signal, tooltips, and the
// inspector on F1.
package main

import (
	"fmt"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func main() {
	dark := ggui.State(false)
	name := ggui.State("")
	size := ggui.State(16.0)
	notes := ggui.State("")
	fruit := ggui.State("Banana")
	details := ggui.State(true)
	tab := ggui.State(0)
	more := ggui.State(false)
	progress := ggui.State(0.3)
	notify := ggui.State(true)
	plan := ggui.State("free")
	tags := ggui.State([]string{"go", "gui", "ebiten", "signals", "flutter", "svelte", "layout", "hidpi"})

	app := ggui.New(ggui.Config{Title: "ggui · gallery", Width: 640, Height: 640, Resizable: true, Inspector: ebiten.KeyF1}, func() ggui.Widget {
		t := ggui.UseTheme()
		section := func(title string, body ggui.Widget) ggui.Widget {
			return ggui.Column(ggui.Text(title).Style(t.Title).Size(16), body).Gap(t.Space)
		}
		swatch := func(label string, c color.Color) ggui.Widget {
			return ggui.Column(
				ggui.Box().Height(28).Fill(c).Radius(t.Radius).Border(1, t.Border),
				ggui.Text(label).Style(t.Caption),
			).Gap(2).Align(ggui.AlignStretch)
		}
		return ggui.Scroll(ggui.Padding(ggui.Column(
			ggui.Row(
				ggui.Text("gallery").Style(t.Title),
				ggui.Spacer(),
				ggui.Tooltip(ui.Switch(dark, "Dark"), "Swaps the theme; every widget re-reads it"),
			),

			section("Text", ggui.Column(
				ui.TextField(name).Placeholder("Type here, IME works"),
				ui.TextField(notes).Placeholder("Notes: Enter breaks the line, ⌘+Enter submits").Lines(3),
				ggui.Reactive(func() ggui.Widget {
					return ggui.Text(fmt.Sprintf("Hello, %s", or(name.Get(), "stranger"))).Size(size.Get())
				}),
				ggui.Row(
					ggui.Text("Size").Color(t.Muted),
					ggui.Expanded(ui.Slider(size, 10, 40).Step(1)),
					ggui.Textf("%2.0f", size).NoWrap(),
				).Gap(t.Space),
			).Gap(t.Space)),

			section("Choices", ggui.Column(
				ui.Checkbox(notify, "Send notifications"),
				ui.Radios(plan, []string{"free", "pro", "team"}),
				ggui.Row(
					ui.Select(fruit, []string{"Apple", "Banana", "Cherry", "Durian"}),
					ui.Menu("Actions",
						ui.MenuItem("Reset text size", func() { size.Set(16) }),
						ui.MenuItem("Clear name", func() { name.Set("") }),
						ui.MenuDivider(),
						ui.MenuItem("Toggle dark", func() { ggui.Toggle(dark) }),
					),
					ggui.Textf("picked %s", fruit).AsCaption(),
				).Gap(t.Space),
			).Gap(t.Space)),

			section("Tabs", ui.Card(ui.Tabs(tab,
				ui.Tab("Overview", ggui.Column(
					ggui.Row(ggui.Text("Status"), ui.Badge("stable"), ui.Badge("new").Accent()).Gap(t.Space),
					ui.Progress(progress), // reads the signal every frame: no Reactive needed
					ggui.Row(
						ui.Button("+10%", func() { progress.Set(min(progress.Peek()+0.1, 1)) }).Secondary(),
						ui.Button("Reset", func() { progress.Set(0) }).Secondary(),
					).Gap(t.Space),
				).Gap(t.Space).Align(ggui.AlignStretch)),
				ui.Tab("Details", ggui.Column(
					ui.Collapsible(more, "More options", ggui.Column(
						ui.Checkbox(notify, "Send notifications"),
						ggui.Text("Folded content fades and slides.").Color(t.Muted),
					).Gap(t.Space)),
				)),
				ui.Tab("About", ggui.Text("Tabs lay out only the page they show; Left and Right switch while focused.")),
			))),

			section("Transition", ggui.Column(
				ui.Switch(details, "Show details"),
				ggui.Presence(details, ggui.Transition(
					ggui.Box(ggui.Text("Slides and fades in, and back out when hidden. A leaving widget takes no input.")).
						Fill(t.Surface).Radius(t.Radius).Pad(t.Space),
				).Slide(0, -8).Fade()),
			).Gap(t.Space).Align(ggui.AlignStretch)),

			section("Wrap", ggui.View(tags, func(list []string) *ggui.WrapWidget {
				return ggui.Wrap(ggui.Children(list, func(tag string) ggui.Widget {
					return ggui.Tooltip(
						ui.Button(tag+"  ×", func() { ggui.Remove(tags, func(s string) bool { return s == tag }) }).Secondary().Pad(4, 10),
						"Click to remove",
					)
				})...).Space(0.5)
			})),

			section("Grid", ggui.Cached(ggui.Grid(4,
				swatch("Bg", t.Bg), swatch("Surface", t.Surface), swatch("Field", t.Field), swatch("Border", t.Border),
				swatch("Accent", t.Accent), swatch("AccentHover", t.AccentHover), swatch("Muted", t.Muted), swatch("Fg", t.Fg),
			).Gap(t.Space))),

			ui.Divider(),
			ggui.Row(
				ui.Button("Save", func() {}),
				ui.Button("Reset", func() { name.Set(""); size.Set(16) }).Secondary(),
				ggui.Spacer(),
				ggui.Text("F1 toggles the inspector, Tab moves focus").Style(t.Caption),
			).Gap(t.Space),
		).Gap(t.Space*2), t.Space*3))
	})

	app.Setup(func() { ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme()) })
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
