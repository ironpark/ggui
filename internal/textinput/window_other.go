//go:build (!darwin || ios) && !windows

package textinput

import (
	"github.com/ironpark/ggfx"
	"image"
)

func WithWindow(*ggfx.Window) func() { return func() {} }
func CloseWindow(*ggfx.Window)       {}
func HandleEvent(ggfx.Event) bool    { return false }
func UpdateCaret(image.Rectangle)    {}
