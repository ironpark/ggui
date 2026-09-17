package ui

import "github.com/ironpark/ggui"

// AlertDialogWidget is a confirmation that has to be answered: the scrim
// takes the clicks but does not close it, so the only ways out are the two
// buttons and Escape, which cancels. Build one with AlertDialog.
//
// It is Dialog with a policy, and the policy is the point: a dialog asking
// whether to delete something should not be dismissed by a stray click
// beside it.
type AlertDialogWidget struct {
	open               ggui.Binding[bool]
	title, description string
	confirmLabel       string
	cancelLabel        string
	onConfirm          func()
	onCancel           func()
	destructive        bool

	dialog   *DialogWidget
	text     *ggui.TextWidget
	body     *ggui.ColumnWidget
	actions  *ggui.RowWidget
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
func (a *AlertDialogWidget) Cancel(label string) *AlertDialogWidget { a.cancelLabel = label; return a }

// OnCancel fires when the dialog is cancelled, by the button or by Escape.
func (a *AlertDialogWidget) OnCancel(fn func()) *AlertDialogWidget { a.onCancel = fn; return a }

// Destructive draws the confirming button in the theme's Destructive color,
// for an answer that cannot be taken back.
func (a *AlertDialogWidget) Destructive() *AlertDialogWidget { a.destructive = true; return a }

// Dialog returns the panel underneath, for Width, Named and Rect.
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

func (a *AlertDialogWidget) build() {
	a.text = ggui.Text(a.description)
	confirm := Button(a.confirmLabel, func() { a.finish(a.onConfirm) })
	if a.destructive {
		confirm.Destructive()
	}
	a.actions = ggui.Row(
		ggui.Spacer(),
		Button(a.cancelLabel, func() { a.finish(a.onCancel) }).Outline(),
		confirm,
	)
	a.body = ggui.Column(a.text, a.actions).Align(ggui.AlignStretch)
	a.dialog = Dialog(a.open, a.body).
		Title(a.title).
		Dismissible(false).
		OnClose(func() {
			if !a.answered && a.onCancel != nil {
				a.onCancel()
			}
		})
}

// Layout implements ggui.Widget: the dialog takes no room in the tree.
func (a *AlertDialogWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	if a.dialog == nil {
		a.build()
	}
	t := env.Theme()
	a.text.Style(t.Text).Color(t.MutedFg)
	a.actions.Gap(t.Space)
	a.body.Gap(t.Space * 2)
	return a.dialog.Layout(c, env)
}

// Paint implements ggui.Widget.
func (a *AlertDialogWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(a.dialog, r) }
