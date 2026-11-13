package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/examples/internal/offscreen"
	"github.com/ironpark/ggui/fonts/gofont"
	"github.com/ironpark/ggui/ui/theme"
)

// Render the actual app with deterministic time, fonts and state, not a mockup.
func render(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	index := 0
	return offscreen.Run(func() (bool, error) {
		sizes := []ggui.Size{ggui.Sz(1440, 1000), ggui.Sz(1024, 768), ggui.Sz(390, 844)}
		size := sizes[index%3]
		dark := index/3%2 == 0
		variant := index / 6
		now := time.Date(2026, time.November, 13, 12, 0, 0, 0, time.UTC)
		restore := ggui.SetClock(func() time.Time { return now })
		defer restore()
		var m *model
		p := ggui.ProbeBuilder(func() ggui.Widget { m = newModel(); return ggui.Reactive(m.build) }, size).Setup(func() { gofont.Enable() })
		defer p.Close()
		p.Frame()
		s := defaults()
		s.Dark = dark
		if variant == 1 {
			s.Theme.Preset.Accent = theme.AccentViolet
			s.Theme.Chart = theme.AccentBlue
			s.Theme.Radius = theme.RadiusLarge
			s.Theme.Font = theme.FontSans
			s.Theme.Heading = theme.FontMono
			s.Theme.Preset.Style = theme.StyleRhea
			s.Icons = "Tabler"
			s.BoldMenu = true
		}
		m.settings.Set(s)
		if variant == 3 {
			m.example.Set("02")
		}
		if variant == 2 {
			if size.W < 760 {
				m.settingsOpen.Set(true)
			} else {
				m.codeOpen.Set(true)
			}
		}
		img := ggfx.NewImage(int(size.W), int(size.H))
		defer img.Deallocate()
		for i := 0; i < 4; i++ {
			img.Fill(theme.Preset{}.For(dark).Bg)
			p.Draw(img)
			now = now.Add(time.Second)
		}
		mode := "light"
		if dark {
			mode = "dark"
		}
		name := fmt.Sprintf("create-%s-%d-%d.png", mode, int(size.W), variant)
		if err := offscreen.SavePNG(filepath.Join(dir, name), img); err != nil {
			return true, err
		}
		index++
		return index == 24, nil
	})
}
