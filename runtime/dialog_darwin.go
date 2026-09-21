//go:build darwin && !ios

package runtime

import (
	"github.com/ebitengine/purego/cstrings"
	"github.com/ebitengine/purego/objc"
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ironpark/ggui/internal/cocoa"
)

// The macOS dialogs are NSOpenPanel and NSSavePanel. A panel is shown as a
// sheet attached to the app's window, the way a document app asks, and run
// modally on the main thread, which is the one AppKit lets show a window;
// the frame goroutine waits, so the app's window stops repainting until
// the sheet closes, as it would under any modal panel. Without a window to
// attach to, the panel stands on its own.

var (
	selOpenPanel                  = objc.RegisterName("openPanel")
	selSavePanel                  = objc.RegisterName("savePanel")
	selSetCanChooseFiles          = objc.RegisterName("setCanChooseFiles:")
	selSetCanChooseDirectories    = objc.RegisterName("setCanChooseDirectories:")
	selSetCanCreateDirectories    = objc.RegisterName("setCanCreateDirectories:")
	selSetAllowsMultipleSelection = objc.RegisterName("setAllowsMultipleSelection:")
	selSetMessage                 = objc.RegisterName("setMessage:")
	selSetTitle                   = objc.RegisterName("setTitle:")
	selSetDirectoryURL            = objc.RegisterName("setDirectoryURL:")
	selSetNameFieldStringValue    = objc.RegisterName("setNameFieldStringValue:")
	selSetAllowedFileTypes        = objc.RegisterName("setAllowedFileTypes:")
	selRunModal                   = objc.RegisterName("runModal")
	selURL                        = objc.RegisterName("URL")
	selURLs                       = objc.RegisterName("URLs")
	selPath                       = objc.RegisterName("path")
	selCount                      = objc.RegisterName("count")
	selObjectAtIndex              = objc.RegisterName("objectAtIndex:")
	selFileURLWithPath            = objc.RegisterName("fileURLWithPath:isDirectory:")
	selBeginSheetModal            = objc.RegisterName("beginSheetModalForWindow:completionHandler:")
	selRunModalForWindow          = objc.RegisterName("runModalForWindow:")
	selStopModalWithCode          = objc.RegisterName("stopModalWithCode:")

	classNSOpenPanel = objc.ID(objc.GetClass("NSOpenPanel"))
	classNSSavePanel = objc.ID(objc.GetClass("NSSavePanel"))
	classNSURL       = objc.ID(objc.GetClass("NSURL"))
)

// nsModalResponseOK is what a panel returns when the user chose.
const nsModalResponseOK = 1

// sheetDone is the sheet's completion handler: the sheet has closed with a
// response, and the modal loop runSheet started is told so.
var sheetDone = cocoa.NewBlock(func(_ uintptr, response int) {
	cocoa.App().Send(selStopModalWithCode, response)
})

// showDialog runs the panel on the main thread and waits for it.
func showDialog(k dialogKind, d FileDialog) (paths []string, err error) {
	ebiten.RunOnMainThread(func() { paths, err = runPanel(k, d) })
	return paths, err
}

// runPanel builds and runs one panel. It must run on the main thread.
func runPanel(k dialogKind, d FileDialog) ([]string, error) {
	var panel objc.ID
	if k == kindSave {
		panel = classNSSavePanel.Send(selSavePanel)
	} else {
		panel = classNSOpenPanel.Send(selOpenPanel)
		panel.Send(selSetCanChooseFiles, k != kindFolder)
		panel.Send(selSetCanChooseDirectories, k == kindFolder)
		panel.Send(selSetAllowsMultipleSelection, k == kindOpenMultiple)
	}
	panel.Send(selSetCanCreateDirectories, true)
	if d.Title != "" {
		// A panel's title bar is empty on modern macOS; the message is the
		// line above the browser, which is where the caption is read.
		panel.Send(selSetTitle, cocoa.String(d.Title))
		panel.Send(selSetMessage, cocoa.String(d.Title))
	}
	if d.Directory != "" {
		panel.Send(selSetDirectoryURL, classNSURL.Send(selFileURLWithPath, cocoa.String(d.Directory), true))
	}
	if d.FileName != "" {
		panel.Send(selSetNameFieldStringValue, cocoa.String(d.FileName))
	}
	if exts, restricted := extensions(d.Filters); restricted && k != kindFolder {
		// Deprecated since macOS 12 in favour of UTType content types, and
		// still honoured; the replacement needs another framework loaded.
		panel.Send(selSetAllowedFileTypes, cocoa.StringArray(exts))
	}
	if runSheet(panel) != nsModalResponseOK {
		return nil, ErrCanceled
	}
	if k == kindSave {
		return []string{urlPath(panel.Send(selURL))}, nil
	}
	urls := panel.Send(selURLs)
	n := objc.Send[uint](urls, selCount)
	paths := make([]string, 0, n)
	for i := range n {
		paths = append(paths, urlPath(urls.Send(selObjectAtIndex, i)))
	}
	return paths, nil
}

// runSheet shows the panel attached to the app's window and waits for the
// user's answer, or runs it on its own when there is no window yet. The
// completion handler stops the modal loop with the response, which is the
// conventional way to make a sheet synchronous.
func runSheet(panel objc.ID) int {
	win := cocoa.AppWindow()
	if win == 0 {
		return objc.Send[int](panel, selRunModal)
	}
	panel.Send(selBeginSheetModal, win, sheetDone.Ptr())
	return objc.Send[int](cocoa.App(), selRunModalForWindow, panel)
}

// extensions flattens the filters into the one list of extensions a panel
// admits, since AppKit offers no named groups, and reports whether the
// panel is restricted at all: a filter with no extensions admits every
// file, which makes the whole list moot.
func extensions(filters []FileFilter) (exts []string, restricted bool) {
	if len(filters) == 0 {
		return nil, false
	}
	for _, f := range filters {
		if len(f.Extensions) == 0 {
			return nil, false
		}
		exts = append(exts, f.Extensions...)
	}
	return exts, true
}

// urlPath is the file system path of an NSURL.
func urlPath(url objc.ID) string {
	return cstrings.NSStringToString(url.Send(selPath))
}
