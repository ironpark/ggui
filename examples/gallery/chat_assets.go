package main

import (
	"embed"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

//go:embed assets/chat/*
var chatAssets embed.FS

var chatFonts = sync.OnceValue(func() [2]*ggui.Font {
	var fonts [2]*ggui.Font
	for i, name := range []string{"Geist-Regular.ttf", "Geist-Medium.ttf"} {
		data, err := chatAssets.ReadFile("assets/chat/" + name)
		if err != nil {
			panic(err)
		}
		fonts[i] = ggui.MustFont(data)
	}
	return fonts
})

func chatImage(name string) *ebiten.Image {
	data, err := chatAssets.ReadFile("assets/chat/" + name)
	if err != nil {
		panic(err)
	}
	img, err := ggui.DecodeImage(data)
	if err != nil {
		panic(err)
	}
	ggui.OnCleanup(img.Deallocate)
	return img
}

// Reference scenes inherit the selected preset, whose fonts main.go installs.
func chatSurface(content ggui.Widget, maxWidth ...float64) ggui.Widget {
	t := ggui.UseTheme()
	t.Card = t.Bg // reaction rings cut out against the actual conversation surface
	width := 384.0
	if len(maxWidth) > 0 {
		width = maxWidth[0]
	}
	scene := ggui.Row(ggui.Spacer(), ggui.Box(ggui.Themed(t, content)).Width(width), ggui.Spacer())
	return ggui.Box(ggui.Padding(scene, 24, 20, 32, 20)).Fill(t.Bg).Radius(12)
}

// Small outline icons use logical coordinates so they stay crisp at any scale.
type chatIcon struct {
	kind string
	col  color.Color
}

func (i *chatIcon) Layout(c ggui.Constraints, e ggui.Env) ggui.Size {
	i.col = e.Text().Color
	return c.Constrain(ggui.Sz(16, 16))
}
func (i *chatIcon) Paint(c *ggui.Canvas, r ggui.Rect) {
	line := func(x, y, u, v float64) {
		c.StrokeLine(r.Origin.Add(ggui.Pt(x, y)), r.Origin.Add(ggui.Pt(u, v)), 1.25, i.col)
	}
	circle := func(x, y, rad float64) {
		c.StrokeRoundRect(ggui.Rct(r.Origin.Add(ggui.Pt(x-rad, y-rad)), ggui.Sz(rad*2, rad*2)), rad, 1.25, i.col)
	}
	switch i.kind {
	case "branch":
		line(4, 2, 4, 11)
		circle(4, 13, 2)
		circle(12, 3, 2)
		line(12, 5, 12, 7)
		line(12, 7, 10, 10)
		line(10, 10, 6, 12)
	case "search":
		circle(7, 7, 5.5)
		line(11, 11, 15, 15)
	case "close":
		line(5, 5, 11, 11)
		line(11, 5, 5, 11)
	case "file":
		line(3, 1, 9, 1)
		line(9, 1, 13, 5)
		line(13, 5, 13, 15)
		line(13, 15, 3, 15)
		line(3, 15, 3, 1)
		line(9, 1, 9, 5)
		line(9, 5, 13, 5)
		line(6, 8, 4.5, 10)
		line(4.5, 10, 6, 12)
		line(10, 8, 11.5, 10)
		line(11.5, 10, 10, 12)
	}
}
