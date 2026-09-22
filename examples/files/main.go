// Command files shows the two ways a file gets into a ggui app: dropped
// onto the window from the desktop, or chosen in the platform's own file
// dialog. Drop anything onto the card to list it; the buttons open the
// native Open, Save and folder dialogs through the host's Dialogs.
//
// The UI lives in build so main_test.go can drive it headlessly with a
// Probe, dropping in-memory files and answering the dialogs with the stub
// a Probe carries.
package main

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/runtime"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// model is the list of files the app has been handed and a line about the
// last thing that happened.
type model struct {
	dialogs runtime.FilePicker // the host's, set once it exists; see main
	Files   *ggui.StateValue[[]string]
	Status  *ggui.StateValue[string]
}

func newModel() *model {
	return &model{
		Files:  ggui.State([]string(nil)),
		Status: ggui.State("Drop files onto the card, or open a dialog."),
	}
}

// accept takes a drop: every file it carries joins the list. A file dropped
// in a browser has no path, so the name stands in for it.
func (m *model) accept(ev ggui.DropEvent) {
	for _, f := range ev.Files {
		name := f.Path
		if name == "" {
			name = f.Name
		}
		if f.Dir {
			name += string(filepath.Separator)
		}
		ggui.Append(m.Files, name)
	}
	m.Status.Set(fmt.Sprintf("Dropped %d item(s).", len(ev.Files)))
}

var images = []runtime.FileFilter{
	{Name: "Images", Extensions: []string{"png", "jpg", "jpeg", "gif"}},
	{Name: "All files", Extensions: nil},
}

func (m *model) open() {
	path, err := m.dialogs.OpenFile(runtime.FileDialog{Title: "Open a file", Filters: images})
	m.report(err, path)
}

func (m *model) openMany() {
	paths, err := m.dialogs.OpenFiles(runtime.FileDialog{Title: "Open files"})
	m.report(err, paths...)
}

func (m *model) folder() {
	path, err := m.dialogs.PickFolder(runtime.FileDialog{Title: "Choose a folder"})
	m.report(err, path)
}

func (m *model) save() {
	path, err := m.dialogs.SaveFile(runtime.FileDialog{Title: "Save as", FileName: "untitled.png", Filters: images})
	m.report(err, path)
}

// report folds a dialog's answer into the list and the status line. A
// cancel is not an error to the user, only nothing to do.
func (m *model) report(err error, paths ...string) {
	switch {
	case errors.Is(err, runtime.ErrCanceled):
		m.Status.Set("Canceled.")
	case errors.Is(err, runtime.ErrUnsupported):
		m.Status.Set("No file dialog on this platform.")
	case err != nil:
		m.Status.Set("Dialog failed: " + err.Error())
	default:
		ggui.Append(m.Files, paths...)
		m.Status.Set(fmt.Sprintf("Chose %d path(s).", len(paths)))
	}
}

func (m *model) clear() {
	m.Files.Set(nil)
	m.Status.Set("Cleared.")
}

// build is the root Builder. The card is the drop zone: Pointer with OnDrop
// takes any drop inside it, and a drop anywhere else on the window reaches
// nothing, since main registers no App.OnDrop.
func (m *model) build() ggui.Widget {
	list := ggui.View(m.Files, func(files []string) ggui.Widget {
		if len(files) == 0 {
			return ui.Caption("Nothing yet.")
		}
		return ggui.List(files, func(f string) ggui.Widget {
			return ggui.Text(f).NoWrap()
		}).Space(0.25)
	})
	zone := ggui.Pointer(ui.Card(ggui.Column(
		ggui.Text("Drop files here").StyleKey(uitheme.TitleKey, uitheme.Default().Title).Role(ggui.RoleHeading),
		list,
	).Space(1).Align(ggui.AlignStretch)).Pad(24)).OnDrop(m.accept)
	buttons := ggui.Wrap(
		ui.Button("Open…", m.open),
		ui.Button("Open several…", m.openMany),
		ui.Button("Choose folder…", m.folder),
		ui.Button("Save…", m.save),
		ui.Button("Clear", m.clear),
	).Gap(8)
	return ggui.Box(ggui.Column(
		ggui.Expanded(zone),
		buttons,
		ggui.TextOf(m.Status).StyleKey(uitheme.CaptionKey, uitheme.Default().Caption),
	).Space(1).Align(ggui.AlignStretch)).Pad(24)
}

func main() {
	m := newModel()
	app := ggui.New(ggui.Config{
		Title:     "ggui · files",
		Width:     560,
		Height:    420,
		Resizable: true,
	}, m.build)
	m.dialogs = app.Dialogs()
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
