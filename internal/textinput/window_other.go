//go:build js

package textinput

import (
	"github.com/ironpark/ggwd"
	"image"
)

func WithWindow(*ggwd.Window) func() { return func() {} }
func CloseWindow(*ggwd.Window)       {}
func HandleEvent(ggwd.Event) bool    { return false }
func UpdateCaret(image.Rectangle)    {}
