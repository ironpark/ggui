//go:build (darwin && !ios) || windows

package textinput

import "image"

// Ebitengine reports the virtual keyboard's visible region to its internal
// ui package so the game can shift its rendering; macOS has no virtual
// keyboard, so this copy has nothing to report.

func reportVirtualKeyboardToUI(caretBounds image.Rectangle) {}

func clearVirtualKeyboardFromUI() {}
