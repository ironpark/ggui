package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

const (
	// alertDialogSmallWidth is the width of a Small alert dialog: shadcn's
	// max-w-xs.
	alertDialogSmallWidth = 320
	// alertDialogMediaSize is the side of the tile Media sits in, and
	// alertDialogMediaIcon the size of an icon in it.
	alertDialogMediaSize = 40
	alertDialogMediaIcon = 24
)

// AlertDialogWidget is a confirmation that has to be answered: the scrim
// takes the clicks but does not close it, so the only ways out are the two
// buttons and Escape, which cancels. Build one with AlertDialog.
//
// It is Dialog with a policy, and the policy is the point: a dialog asking
// whether to delete something should not be dismissed by a stray click
// beside it.
type AlertDialogWidget struct {
	props              property.Owner
	open               ggui.Binding[bool]
	title, description string
	confirmLabel       string
	cancelLabel        string
	onConfirm          func()
	onCancel           func()
	destructive        bool
	small              bool
	media              ggui.Widget

	dialog   *DialogWidget
	heading  *ggui.TextWidget
	text     *ggui.TextWidget
	lines    *ggui.ColumnWidget // the title over the description
	tile     *ggui.BoxWidget    // the media, if any
	header   ggui.Widget
	beside   *ggui.RowWidget    // the media beside the lines
	stacked  *ggui.ColumnWidget // the media over the lines, in a Small dialog
	answered bool
}

// AlertDialog creates a confirmation shown while open is true. Confirm
// names the action it asks about; without it the button reads "Continue"
// and only closes.
//
//	del := ggui.State(false)
//	ui.AlertDialog(del, "Delete the file?", "This cannot be undone.").
//		Confirm("Delete", remove).Destructive()
func AlertDialog(open ggui.Binding[bool], title, description string) *AlertDialogWidget {
	return &AlertDialogWidget{
		open: open, title: title, description: description,
		confirmLabel: "Continue", cancelLabel: "Cancel",
	}
}

// Confirm names the confirming button and what it does. The dialog closes
// first, so fn may open another.
func (a *AlertDialogWidget) Confirm(label string, fn func()) *AlertDialogWidget {
	a.confirmLabel, a.onConfirm = label, fn
	return a
}

// Cancel renames the cancelling button.
func (a *AlertDialogWidget) Cancel(label string) *AlertDialogWidget {
	defer property.Watch(&a.props, &a.cancelLabel)()
	a.cancelLabel = label
	return a
}

// OnCancel fires when the dialog is cancelled, by the button or by Escape.
func (a *AlertDialogWidget) OnCancel(fn func()) *AlertDialogWidget { a.onCancel = fn; return a }

// Destructive draws the confirming button in the theme's Destructive color,
// for an answer that cannot be taken back.
func (a *AlertDialogWidget) Destructive() *AlertDialogWidget {
	defer property.Watch(&a.props, &a.destructive)()
	a.destructive = true
	return a
}

// Small makes the dialog narrow, 320px, with its text centered and its two
// buttons sharing the footer's width: the size for a short question.
func (a *AlertDialogWidget) Small() *AlertDialogWidget {
	a.small = true
	if a.dialog != nil {
		a.build()
	}
	a.props.Changed()
	return a
}

// Media shows w in a muted tile beside the text, or above it in a Small
// dialog: an icon that says what kind of question this is. An icon is drawn
// at 24px; wrap it in a Box to give it a size of its own.
//
//	ui.AlertDialog(open, "Allow Bluetooth?", "The accessory wants to connect.").
//		Media(ui.Icon(icons.Info))
func (a *AlertDialogWidget) Media(w ggui.Widget) *AlertDialogWidget {
	if icon, ok := w.(*icons.Widget); ok {
		icon.Size(alertDialogMediaIcon)
	}
	a.media = w
	if a.dialog != nil {
		a.build()
	}
	a.props.Changed()
	return a
}

// Dialog returns the panel underneath, for Width, Name and Rect.
func (a *AlertDialogWidget) Dialog() *DialogWidget {
	if a.dialog == nil {
		a.build()
	}
	return a.dialog
}

// Close cancels the dialog, as Escape does.
func (a *AlertDialogWidget) Close() { a.finish(a.onCancel) }

// finish closes the dialog and then runs fn. The flag keeps the dialog's
// OnClose, which is the Escape path, from cancelling a second time.
func (a *AlertDialogWidget) finish(fn func()) {
	a.answered = true
	a.dialog.Close()
	a.answered = false
	if fn != nil {
		fn()
	}
}

// build puts the dialog together: the header as its content, since where
// the media and the text go is the alert dialog's own, and the two buttons
// in the dialog's footer.
func (a *AlertDialogWidget) build() {
	// The title is left in text-base, whose lines are 1.5 times its size.
	a.heading, a.text = Heading(a.title).LineHeight(1.5), ggui.Text(a.description)
	a.lines = ggui.Column(a.heading, a.text)
	a.tile, a.beside, a.stacked = nil, nil, nil
	a.header = a.lines
	if a.small {
		a.heading.Align(.5)
		a.text.Align(.5)
		a.lines.Align(ggui.AlignCenter)
	} else {
		a.lines.Align(ggui.AlignStretch)
	}
	if a.media != nil {
		a.tile = ggui.Box(ggui.Center(a.media)).Size(alertDialogMediaSize, alertDialogMediaSize)
		if a.small {
			a.stacked = ggui.Column(a.tile, a.lines).Align(ggui.AlignCenter)
			a.header = a.stacked
		} else {
			a.beside = ggui.Row(a.tile, ggui.Expanded(a.lines)).Align(ggui.AlignStart)
			a.header = a.beside
		}
	}
	confirm := Button(a.confirmLabel, func() { a.finish(a.onConfirm) })
	if a.destructive {
		confirm.Destructive()
	}
	cancel := Button(a.cancelLabel, func() { a.finish(a.onCancel) }).Outline()
	if a.dialog == nil {
		a.dialog = Dialog(a.open, alertDialogHeader{a}).
			Name(a.title).
			Dismissible(false).
			CloseButton(false).
			OnClose(func() {
				if !a.answered && a.onCancel != nil {
					a.onCancel()
				}
			})
	}
	a.dialog.evenActions = a.small
	a.dialog.Footer(cancel, confirm)
	if a.small {
		a.dialog.Width(alertDialogSmallWidth)
	}
}

// alertDialogHeader is the dialog's content: whichever header build made
// last.
type alertDialogHeader struct{ a *AlertDialogWidget }

func (h alertDialogHeader) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return h.a.header.Layout(c, env)
}
func (h alertDialogHeader) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(h.a.header, r) }

// Layout implements ggui.Widget: the dialog takes no room in the tree.
func (a *AlertDialogWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer a.props.Layout().EndLayout()
	if a.dialog == nil {
		a.build()
	}
	t := uitheme.Of(env)
	a.text.Style(mutedText(t))
	a.lines.Gap(t.Space * .75)
	if a.tile != nil {
		a.tile.Fill(t.Muted).Radius(t.RadiusSm)
	}
	if a.beside != nil {
		a.beside.Gap(t.Space * 2)
	}
	if a.stacked != nil {
		// The tile stands a little further off the title than the title
		// does off the description.
		a.stacked.Gap(t.Space * 1.75)
	}
	return a.dialog.Layout(c, env)
}

// Paint implements ggui.Widget.
func (a *AlertDialogWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(a.dialog, r) }
