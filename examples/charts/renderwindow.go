package main

import "github.com/ironpark/ggfx"

// runRenderWindow opens one window and calls step on every frame until step
// reports that it is done or fails. A render harness needs a window because
// the graphics driver draws and reads back pixels inside a frame; the window
// only previews what is being written to disk.
func runRenderWindow(title string, width, height int, step func(screen *ggfx.Image) (done bool, err error)) error {
	var window *ggfx.Window
	return ggfx.Run(ggfx.HandlerFunc(func(ev ggfx.Event) error {
		switch ev := ev.(type) {
		case ggfx.StartEvent:
			w, err := ggfx.NewWindow(&ggfx.WindowOptions{Title: title, Width: width, Height: height})
			if err != nil {
				return err
			}
			window = w
		case ggfx.FrameEvent:
			done, err := step(ev.Screen)
			if err != nil {
				return err
			}
			if done {
				return ggfx.Termination
			}
			window.RequestFrame()
		}
		return nil
	}), nil)
}
