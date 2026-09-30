//go:build darwin && !ios

package runtime

import (
	"github.com/ebitengine/purego/objc"
	"github.com/ironpark/ggfx"

	"github.com/ironpark/ggui/internal/platform/cocoa"
)

// A message is an NSAlert, shown as a sheet on the app's window as the
// file panels are.

var (
	selSetMessageText      = objc.RegisterName("setMessageText:")
	selSetInformativeText  = objc.RegisterName("setInformativeText:")
	selSetAlertStyle       = objc.RegisterName("setAlertStyle:")
	selAddButtonWithTitle  = objc.RegisterName("addButtonWithTitle:")
	selWindow              = objc.RegisterName("window")
	selNewAlert            = objc.RegisterName("new")
	selRelease             = objc.RegisterName("release")
	classNSAlert           = objc.ID(objc.GetClass("NSAlert"))
	nsAlertFirstButtonCode = 1000 // NSAlertFirstButtonReturn
)

// NSAlertStyle values.
const (
	nsAlertStyleWarning       = 0
	nsAlertStyleInformational = 1
	nsAlertStyleCritical      = 2
)

// showMessage runs the alert on the main thread and waits for it.
func showMessage(m Message, owner uintptr) (i int, err error) {
	err = ErrNotRunning
	ggfx.RunOnMainThread(func() { i, err = runAlert(m, objc.ID(owner)) })
	return i, err
}

// runAlert builds and runs one alert. It must run on the main thread.
func runAlert(m Message, win objc.ID) (int, error) {
	alert := classNSAlert.Send(selNewAlert)
	defer alert.Send(selRelease)
	style := nsAlertStyleInformational
	switch m.Kind {
	case MessageWarning:
		style = nsAlertStyleWarning
	case MessageError:
		style = nsAlertStyleCritical
	}
	alert.Send(selSetAlertStyle, style)
	alert.Send(selSetMessageText, cocoa.String(m.Title))
	alert.Send(selSetInformativeText, cocoa.String(m.Detail))
	buttons := m.buttons()
	for _, b := range buttons {
		alert.Send(selAddButtonWithTitle, cocoa.String(b))
	}
	var resp int
	if win == 0 {
		resp = objc.Send[int](alert, selRunModal)
	} else {
		alert.Send(selBeginSheetModal, win, sheetDone().Ptr())
		resp = objc.Send[int](cocoa.App(), selRunModalForWindow, alert.Send(selWindow))
	}
	i := resp - nsAlertFirstButtonCode
	if i < 0 || i >= len(buttons) {
		return 0, ErrCanceled
	}
	return i, nil
}
