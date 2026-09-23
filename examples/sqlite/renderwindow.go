package main

import "github.com/ironpark/ggfx"

// runRenderWindow is examples/internal/offscreen.Run. This module resolves
// ggui at its pinned version outside the workspace, which predates that
// package, so it keeps a copy until the pin moves past it.
func runRenderWindow(step func() (done bool, err error)) error {
	var window *ggfx.Window
	return ggfx.Run(ggfx.HandlerFunc(func(ev ggfx.Event) error {
		switch ev.(type) {
		case ggfx.StartEvent:
			w, err := ggfx.NewWindow(&ggfx.WindowOptions{Title: "ggui render", Width: 64, Height: 64, Hidden: true})
			if err != nil {
				return err
			}
			window = w
		case ggfx.FrameEvent:
			done, err := step()
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
