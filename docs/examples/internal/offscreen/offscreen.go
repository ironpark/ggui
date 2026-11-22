// Package offscreen runs the examples' render harnesses, which draw into
// offscreen images and write them to disk without showing a window.
package offscreen

import (
	"image/png"
	"os"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggwd"
)

// Run calls step on every frame until step reports that it is done or fails.
// The graphics driver draws and reads back pixels only inside a frame, so Run
// opens a window for the frames, but keeps it hidden: nothing is shown.
func Run(step func() (done bool, err error)) error {
	var window *ggwd.Window
	return ggwd.Run(ggwd.HandlerFunc(func(ev ggwd.Event) error {
		switch ev.(type) {
		case ggwd.StartEvent:
			w, err := ggwd.NewWindow(&ggwd.WindowOptions{Title: "ggui render", Width: 64, Height: 64, Hidden: true})
			if err != nil {
				return err
			}
			window = w
		case ggwd.FrameEvent:
			done, err := step()
			if err != nil {
				return err
			}
			if done {
				return ggwd.Termination
			}
			window.RequestFrame()
		}
		return nil
	}), nil)
}

// SavePNG writes img to name, reporting either an encode or a close failure.
func SavePNG(name string, img *ggfx.Image) (err error) {
	f, err := os.Create(name)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	return png.Encode(f, img)
}
