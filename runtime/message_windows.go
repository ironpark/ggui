//go:build windows

package runtime

import (
	"unsafe"

	"github.com/ironpark/ggfx"
)

// A message is user32's MessageBoxW, whose buttons are fixed sets; see
// Message.Buttons for how a message's buttons map onto them.

var procMessageBoxW = dllUser32.NewProc("MessageBoxW")

// MessageBoxW styles and results, from winuser.h.
const (
	mbOK            = 0x0
	mbOKCancel      = 0x1
	mbYesNoCancel   = 0x3
	mbYesNo         = 0x4
	mbIconError     = 0x10
	mbIconQuestion  = 0x20
	mbIconWarning   = 0x30
	mbIconInfo      = 0x40
	mbTaskModal     = 0x2000
	mbSetForeground = 0x10000

	idOK     = 1
	idCancel = 2
	idYes    = 6
	idNo     = 7
)

// showMessage runs the message box on the main thread and waits for it.
func showMessage(m Message, owner uintptr) (i int, err error) {
	style, answers := messageBoxStyle(m)
	if owner == 0 {
		style |= mbTaskModal | mbSetForeground
	}
	text := m.Title
	if m.Detail != "" {
		text += "\n\n" + m.Detail
	}
	body, caption := utf16z(text), utf16z(m.Title)
	var r uintptr
	ran := false
	ggfx.RunOnMainThread(func() {
		ran = true
		r, _, _ = procMessageBoxW.Call(owner, uintptr(unsafe.Pointer(&body[0])), uintptr(unsafe.Pointer(&caption[0])), style)
	})
	if !ran {
		return 0, ErrNotRunning
	}
	for i, id := range answers {
		if uintptr(id) == r {
			return i, nil
		}
	}
	return 0, ErrCanceled
}

// messageBoxStyle is the style for m and the result each of its buttons
// comes back as, in order.
func messageBoxStyle(m Message) (uintptr, []int) {
	var style uintptr
	switch m.Kind {
	case MessageWarning:
		style = mbIconWarning
	case MessageError:
		style = mbIconError
	case MessageQuestion:
		style = mbIconQuestion
	default:
		style = mbIconInfo
	}
	switch n := len(m.buttons()); {
	case n == 1:
		return style | mbOK, []int{idOK}
	case n == 2 && m.Kind == MessageQuestion:
		return style | mbYesNo, []int{idYes, idNo}
	case n == 2:
		return style | mbOKCancel, []int{idOK, idCancel}
	default:
		return style | mbYesNoCancel, []int{idYes, idNo, idCancel}
	}
}
