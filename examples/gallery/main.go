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
	notify := ggui.State(true)
	plan := ggui.State("free")
	tags := ggui.State([]string{"go", "gui", "ebiten", "signals", "flutter", "svelte", "layout", "hidpi"})

	ggui.Watch(dark, func(on bool) {
		if on {
			ggui.SetTheme(ggui.DarkTheme())
		} else {
			ggui.SetTheme(ggui.DefaultTheme())
		}
	})
	removeTag := func(tag string) {
		tags.Update(func(ts []string) []string {
			out := ts[:0:0]
			for _, t := range ts {
				if t != tag {
					out = append(out, t)
				}
			}
			return out
		})
	}

	app := ggui.New(ggui.Config{Title: "ggui · gallery", Width: 640, Height: 640, Resizable: true, Inspector: ebiten.KeyF1}, func() ggui.Widget {
		t := ggui.UseTheme()
		section := func(title string, body ggui.Widget) ggui.Widget {
			return ggui.Column(ggui.Text(title).Style(t.Title).Size(16), body).Gap(t.Space)
		}
		swatch := func(label string, c color.Color) ggui.Widget {
			return ggui.Column(
				ggui.Box().Height(28).Fill(c).Radius(t.Radius).Border(1, t.Border),
				ggui.Text(label).Size(11).Color(t.Muted),
			).Gap(2).Align(ggui.AlignStretch)
		}
		return ggui.Scroll(ggui.Padding(ggui.Column(
			ggui.Row(
				ggui.Text("gallery").Style(t.Title),
				ggui.Spacer(),
				ggui.Tooltip(ui.Switch(dark, "Dark"), "Swaps the theme; every widget re-reads it"),
			).Align(ggui.AlignCenter),

			section("Text", ggui.Column(
				ui.TextField(name).Placeholder("Type here, IME works"),
				ggui.Reactive(func() ggui.Widget {
					s := size.Get()
					return ggui.Text(fmt.Sprintf("Hello, %s", or(name.Get(), "stranger"))).Size(s)
				}),
				ggui.Row(
					ggui.Text("Size").Color(t.Muted),
					ggui.Expanded(ui.Slider(size, 10, 40).Step(1)),
					ggui.Reactive(func() ggui.Widget { return ggui.Text(fmt.Sprintf("%2.0f", size.Get())).NoWrap() }),
				).Gap(t.Space).Align(ggui.AlignCenter),
			).Gap(t.Space)),

			section("Choices", ggui.Column(
				ui.Checkbox(notify, "Send notifications"),
				ggui.Row(
					ui.Radio(plan, "free", "Free"),
					ui.Radio(plan, "pro", "Pro"),
					ui.Radio(plan, "team", "Team").Disabled(true),
				).Gap(t.Space*2),
			).Gap(t.Space)),

			section("Wrap", ggui.Reactive(func() ggui.Widget {
				return ggui.Wrap(ggui.Children(tags.Get(), func(tag string) ggui.Widget {
					return ggui.Tooltip(
						ui.Button(tag+"  ×", func() { removeTag(tag) }).Secondary().Pad(4, 10),
						"Click to remove",
					)
				})...).Gap(t.Space / 2)
			})),

			section("Grid", ggui.Grid(4,
				swatch("Bg", t.Bg), swatch("Surface", t.Surface), swatch("Field", t.Field), swatch("Border", t.Border),
				swatch("Accent", t.Accent), swatch("AccentHover", t.AccentHover), swatch("Muted", t.Muted), swatch("Fg", t.Fg),
			).Gap(t.Space)),

			ui.Divider(),
			ggui.Row(
				ui.Button("Save", func() {}),
				ui.Button("Reset", func() { name.Set(""); size.Set(16) }).Secondary(),
				ggui.Spacer(),
				ggui.Text("F1 toggles the inspector, Tab moves focus").Color(t.Muted).Size(12),
			).Gap(t.Space).Align(ggui.AlignCenter),
		).Gap(t.Space*2), t.Space*3))
	})

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
