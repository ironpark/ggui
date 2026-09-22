package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// Render the same widgets and interactions as the app at desktop and narrow
// widths. Run with -render-dir to review layout changes without screen capture.
type workspaceRender struct {
	directory string
	done      bool
	err       error
}

func (r *workspaceRender) Layout(int, int) (int, int) { return 320, 240 }
func (r *workspaceRender) Update() error {
	if r.err != nil {
		return r.err
	}
	if r.done {
		return ebiten.Termination
	}
	return nil
}
func (r *workspaceRender) Draw(_ *ebiten.Image) {
	if r.done || r.err != nil {
		return
	}
	r.err = r.render()
	r.done = true
}
func (r *workspaceRender) render() error {
	now := time.Unix(100, 0)
	restore := ggui.SetClock(func() time.Time { return now })
	defer restore()
	for _, dark := range []bool{false, true} {
		mode := "light"
		if dark {
			mode = "dark"
		}
		for _, width := range []int{1040, 760, 480} {
			var m *model
			p := ggui.ProbeBuilder(func() ggui.Widget {
				return ggui.Provide(ggui.ReducedMotionKey, true, build(m))
			}, ggui.Sz(float64(width), 800))
			p.Setup(func() {
				m = newModel()
				m.Dark.Set(dark)
				bindTheme(m)
			})
			p.Frame()
			img := ebiten.NewImage(width, 800)
			save := func(name string) error {
				p.Frame()
				now = now.Add(time.Second)
				img.Fill(ggui.Untrack(uitheme.Use).Bg)
				p.Draw(img)
				file, err := os.Create(filepath.Join(r.directory, fmt.Sprintf("%s-%d-%s.png", mode, width, name)))
				if err != nil {
					return err
				}
				err = png.Encode(file, img)
				closeErr := file.Close()
				if err != nil {
					return err
				}
				return closeErr
			}
			var err error
			for index, name := range []string{"tasks", "insights", "settings"} {
				m.Tab.Set(index)
				if err = save(name); err != nil {
					break
				}
			}
			if err == nil {
				m.Tab.Set(0)
				m.create()
				err = save("editor")
			}
			img.Deallocate()
			p.Close()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
func renderWorkspace(directory string) error {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	ebiten.SetWindowSize(320, 240)
	ebiten.SetWindowTitle("Workspace layout previews")
	return ebiten.RunGame(&workspaceRender{directory: directory})
}
