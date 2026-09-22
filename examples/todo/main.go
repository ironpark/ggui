// Command todo is a small but complete ggui app: a text field with IME
// input under a labelled Field, a keyed list that keeps each row's widgets
// across edits and animates rows in and out, inline editing of a row's
// title, controls bound to signals, a tweened progress bar, an AlertDialog
// and a theme switch.
//
// model.go holds the state and what changes it; this file is the view:
// plain functions that take the model and return widgets, so the model
// never depends on the ui package. main_test.go drives build headlessly
// with a Probe.
package main

import (
	"log"

	"github.com/ironpark/ggui"
	_ "github.com/ironpark/ggui/inspect/panel"
	"github.com/ironpark/ggui/ui"
)

// build is the root Builder. It reads nothing reactive: the theme comes
// from the Env at layout, and every part that changes is bound to a signal
// or a derived value on the model.
func build(m *model) ggui.Widget {
	content := ggui.Column(
		ggui.Row(ggui.Title("todo"), ggui.Spacer(), ui.Switch(m.Dark, "Dark")),
		composer(m),
		ui.Progress(m.Progress).Height(4),
		ggui.Expanded(ggui.Scroll(list(m))),
		ui.Divider(),
		footer(m),
		// The dialog takes no space here; it paints over the window while
		// Confirm is true, closes itself, then runs the confirmation.
		ui.AlertDialog(m.Confirm, "Clear done?", "Every finished item is removed.").
			Confirm("Remove", m.clearDone).Destructive(),
	).Space(1.5)
	return ggui.Center(ggui.Box(ui.Card(content).Pad(24)).Size(480, 540))
}

// composer is the field that adds an item; Enter and the button do the same.
func composer(m *model) ggui.Widget {
	input := ui.TextField(m.Draft).Placeholder("What needs doing?").OnSubmit(func(string) { m.add() })
	return ui.Field("New item", ggui.Row(
		ggui.Expanded(input),
		ui.Button("Add", m.add),
	).Space(1)).Help("Enter adds it").BindError(m.DraftError)
}

// list keys rows by ID so edits keep each row's widgets, slides rows in
// and out, and shows an empty state when nothing matches the filter.
func list(m *model) ggui.Widget {
	return ggui.EachKeyed(m.Visible,
		func(td *Todo) int { return td.ID },
		func(item ggui.EachItem[*Todo]) ggui.Widget { return row(m, item.Value.Get()) },
	).Space(0.5).
		Transition(func(w ggui.Widget) *ggui.TransitionWidget { return ggui.Transition(w).Fade().Slide(-16, 0) }).
		Else(func() ggui.Widget { return ui.Empty("Nothing here", "Add an item above, or pick another filter.") })
}

// row builds one keyed row once; the checkbox and text follow the row's
// own signals afterwards without a rebuild, and so do the accessible names
// of its controls, bound to the title through BindName.
func row(m *model, td *Todo) ggui.Widget {
	editing := ggui.State(false)
	check := ui.Checkbox(td.Done, "")
	check.BindName(ggui.Sprintf("Done: %s", td.Title)) // Checkbox names itself after its label; this one has none
	remove := ui.Button("×", func() { m.remove(td.ID) }).BindName(ggui.Sprintf("Remove %s", td.Title)).Outline().Pad(2, 8)
	return ggui.Row(check, ggui.Expanded(title(td, editing)), remove).Space(1)
}

// title shows the row's text as a caption once done, and swaps in
// a field bound to the same signal while editing. A tap starts editing;
// Enter ends it.
func title(td *Todo, editing *ggui.StateValue[bool]) ggui.Widget {
	return ggui.If(editing, func() ggui.Widget {
		return ui.TextField(td.Title).
			BindName(ggui.Sprintf("Edit: %s", td.Title)).
			OnSubmit(func(string) { editing.Set(false) })
	}).Else(func() ggui.Widget {
		text := ggui.If(td.Done, func() ggui.Widget { return ggui.TextOf(td.Title).AsCaption() }).
			Else(func() ggui.Widget { return ggui.TextOf(td.Title) })
		return ggui.Tap(text, func() { editing.Set(true) })
	})
}

// footer shows what is left, the filter, and the clear action.
func footer(m *model) ggui.Widget {
	clear := ui.Button("Clear done", m.askClearDone).Outline().BindDisabled(m.NoneDone)
	return ggui.Row(
		ggui.Textf("%d left", m.Left).AsCaption().NoWrap(),
		ggui.Spacer(),
		ui.Radios(m.Show).Options([]filter{all, active, done}), // labelled through filter.String
		ggui.Tooltip(clear, "Removes every finished item (⌘/Ctrl+K)"),
	).Space(1)
}

// shortcuts binds the keys to the model's actions. Both App and Probe
// offer Shortcut, so the test installs the same bindings.
func shortcuts(h ggui.Host, m *model) {
	h.Shortcut("cmd+k", m.askClearDone)
}

// run builds the model under a Root so its derived values and tween have
// an owner, and disposes that owner after the app closes.
func run() error {
	var app *ggui.App
	dispose := ggui.Root(func() {
		m := newModel()
		app = ggui.New(ggui.Config{
			Title:     "ggui · todo",
			Width:     520,
			Height:    600,
			Resizable: true,
			Inspector: "f1",
		}, func() ggui.Widget { return build(m) })
		shortcuts(app, m)
		// Setup runs under the app's root owner, so the theme binding is
		// disposed with the app.
		app.Setup(func() { ggui.BindTheme(m.Dark, ggui.DarkTheme(), ggui.DefaultTheme()) })
	})
	defer dispose()
	defer app.Close()
	return app.Run()
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
