package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/widget"
)

// AppMenubar draws the app's menus, as App.SetMenu installed them, as a
// Menubar inside a window: what an app shows on Windows and Linux, which
// have no menu bar of the platform's for them. On macOS, where the menus
// are in the menu bar, it draws nothing, so one tree serves every
// platform. The chords work either way; the menubar only shows them.
//
//	widget.Column(ui.AppMenubar(app.Menu()), widget.Expanded(content))
//
// A submenu opens beside its row, as MenuSub's does.
func AppMenubar(menus []ggui.MenuItem) ggui.Widget {
	if ggui.NativeMenu() || len(menus) == 0 {
		return widget.Box()
	}
	var bar []*MenuWidget
	for _, m := range menus {
		bar = append(bar, Menu(m.Label, menuEntries(m.Items)...))
	}
	return Menubar(bar...)
}

// notReader is a stateless Readable[bool] inverting r without creating a
// reactive memo or subscription.
type notReader struct{ r ggui.Readable[bool] }

func (n notReader) Get() bool { return !n.r.Get() }

// menuEntries turns entries of the app's menu model into menu widgets.
func menuEntries(items []ggui.MenuItem) []ggui.Widget {
	var out []ggui.Widget
	for _, m := range items {
		switch {
		case m.Separator:
			out = append(out, MenuDivider())
		case len(m.Items) > 0:
			sub := MenuSub(m.Label, menuEntries(m.Items)...)
			if m.Enabled != nil {
				sub.BindDisabled(notReader{m.Enabled})
			}
			out = append(out, sub)
		default:
			it := MenuItem(m.Label, m.Run)
			if m.Chord != "" {
				it.Shortcut(ggui.MustChord(m.Chord).Label())
			}
			if m.Enabled != nil {
				it.BindDisabled(notReader{m.Enabled})
			}
			if m.Checked != nil {
				it.BindChecked(m.Checked)
			}
			out = append(out, it)
		}
	}
	return out
}
