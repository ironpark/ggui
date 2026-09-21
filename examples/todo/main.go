// Command todo is a small but complete ggui app: a text field with IME
// input under a labelled Field, a keyed list that keeps each row's widgets
// across edits and animates rows in and out, inline editing of a row's
// title, controls bound to signals, a tweened progress bar, an AlertDialog
// and a theme switch.
//
// model.go holds the state and what changes it; this file builds the
// widgets over it. The UI lives in build so main_test.go can drive it
// headlessly with a Probe.
package main

import (
	"log"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// build is the root Builder. It reads nothing reactive: the theme comes
// from the Env at layout, and the parts that change are islands.
func (m *model) build() ggui.Widget {
	counts := m.tally()
	progress := ggui.Tween(0.0, 300*time.Millisecond)
	ggui.Watch(counts, func(c tally) { progress.Set(c.fraction()) })

	return ggui.Center(ggui.Box(ui.Card(ggui.Column(
		ggui.Row(ggui.Title("todo"), ggui.Spacer(), ui.Switch(m.Dark, "Dark")),
		m.composer(),
		ui.Progress(progress).Height(4),
		ggui.Expanded(ggui.Scroll(m.list())),
		ui.Divider(),
		m.footer(counts),
		// The dialog takes no space here; it paints over the window while
		// Confirm is true, closes itself, then runs the confirmation.
		ui.AlertDialog(m.Confirm, "Clear done?", "Every finished item is removed.").
			Confirm("Remove", m.clearDone).Destructive(),
	).Space(1.5)).Pad(24)).Size(480, 540))
}

// composer is the field that adds an item; Enter and the button do the same.
func (m *model) composer() ggui.Widget {
	input := ui.TextField(m.Draft).Placeholder("What needs doing?").OnSubmit(func(string) { m.add() })
	return ui.Field("New item", ggui.Row(
		ggui.Expanded(input),
		ui.Button("Add", m.add),
	).Space(1)).Help("Enter adds it").Error(m.Draft.Map(titleError))
}

// list keys rows by ID so edits keep each row's widgets, slides rows in
// and out, and shows an empty state when nothing matches the filter.
func (m *model) list() ggui.Widget {
	return ggui.EachKeyed(m.visible(),
		func(td *Todo) int { return td.ID },
		func(item ggui.EachItem[*Todo]) ggui.Widget { return m.row(item.Value.Get()) },
	).Space(0.5).
		Transition(func(w ggui.Widget) *ggui.TransitionWidget { return ggui.Transition(w).Fade().Slide(-16, 0) }).
		Else(func() ggui.Widget { return ui.Empty("Nothing here", "Add an item above, or pick another filter.") })
}

// row builds one keyed row once; the checkbox and text follow the row's
// own signals afterwards without a rebuild. The row's editing signal and
// the Watch that keeps the accessible names current live as long as it does.
func (m *model) row(td *Todo) ggui.Widget {
	editing := ggui.State(false)
	check := ui.Checkbox(td.Done, "")
	remove := ui.Button("×", func() { m.remove(td.ID) }).Outline().Pad(2, 8)
	ggui.Watch(td.Title, func(title string) {
		check.SetName("Done: " + title)
		remove.SetName("Remove " + title)
	})
	return ggui.Row(check, ggui.Expanded(m.title(td, editing)), remove).Space(1)
}

// title shows the row's text, struck to a caption once done, and swaps in
// a field bound to the same signal while editing. A tap starts editing;
// Enter ends it.
func (m *model) title(td *Todo, editing *ggui.StateValue[bool]) ggui.Widget {
	return ggui.If(editing, func() ggui.Widget {
		// The branch runs fresh each time editing turns on, so the field
		// is named after the title as it was then.
		return ui.TextField(td.Title).
			Named("Edit: " + ggui.Untrack(td.Title.Get)).
			OnSubmit(func(string) { editing.Set(false) })
	}).Else(func() ggui.Widget {
		text := ggui.If(td.Done, func() ggui.Widget { return ggui.TextOf(td.Title).AsCaption() }).
			Else(func() ggui.Widget { return ggui.TextOf(td.Title) })
		return ggui.Tap(text, func() { editing.Set(true) })
	})
}

// footer shows what is left, the filter, and the clear action.
func (m *model) footer(counts *ggui.DerivedValue[tally]) ggui.Widget {
	clear := ui.Button("Clear done", m.askClearDone).Outline().DisabledWhen(counts.Map(tally.noneDone))
	return ggui.Row(
		ggui.Textf("%d left", counts.Map(tally.left)).AsCaption().NoWrap(),
		ggui.Spacer(),
		ui.Radios(m.Show, []filter{all, active, done}), // labelled through filter.String
		ggui.Tooltip(clear, "Removes every finished item (⌘/Ctrl+K)"),
	).Space(1)
}

// shortcuts binds the keys. Both App and Probe offer Shortcut, so the test
// installs the same bindings.
func (m *model) shortcuts(s ggui.Host) {
	s.Shortcut("cmd+k", m.askClearDone)
}

func main() {
	m := newModel()
	app := ggui.New(ggui.Config{Title: "ggui · todo", Width: 520, Height: 600, Resizable: true, Inspector: "f1"}, m.build)
	m.shortcuts(app)

	// Setup runs under the app's root owner, so the theme binding is
	// disposed with the app.
	app.Setup(func() { ggui.BindTheme(m.Dark, ggui.DarkTheme(), ggui.DefaultTheme()) })
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
