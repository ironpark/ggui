package ui

import "github.com/ironpark/ggui"

// AppMenubar draws the app's menus, as App.SetMenu installed them, as a
// Menubar inside a window: what an app shows on Windows and Linux, which
// have no menu bar of the platform's for them. On macOS, where the menus
// are in the menu bar, it draws nothing, so one tree serves every
// platform. The chords work either way; the menubar only shows them.
//
//	ggui.Column(ui.AppMenubar(app.Menu()), ggui.Expanded(content))
//
// A submenu shows its entries inline, under a divider.
func AppMenubar(menus []ggui.MenuItem) ggui.Widget {
	if ggui.NativeMenu() || len(menus) == 0 {
		return ggui.Box()
	}
	var bar []*MenuWidget
	for _, m := range menus {
		bar = append(bar, Menu(m.Label, menuEntries(m.Items)...))
	}
	return Menubar(bar...)
}

// menuEntries turns entries of the app's menu model into menu widgets.
func menuEntries(items []ggui.MenuItem) []ggui.Widget {
	var out []ggui.Widget
	for _, m := range items {
		switch {
		case m.Separator:
			out = append(out, MenuDivider())
		case len(m.Items) > 0:
			out = append(out, MenuDivider(), Caption(m.Label))
			out = append(out, menuEntries(m.Items)...)
		default:
			it := MenuItem(m.Label, m.Run)
			if m.Chord != "" {
				it.Shortcut(ggui.MustChord(m.Chord).Label())
			}
			if m.Enabled != nil {
				it.BindDisabled(ggui.Map(m.Enabled, func(on bool) bool { return !on }))
			}
			if m.Checked != nil {
				it.BindChecked(m.Checked)
			}
			out = append(out, it)
		}
	}
	return out
}
