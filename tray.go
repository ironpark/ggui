package ggui

import (
	"bytes"
	"image"
	"image/png"
	"slices"

	"github.com/ironpark/ggui/internal/nativemenu"
)

// TrayConfig describes an icon in the system tray: the status area at the
// right of the menu bar on macOS.
type TrayConfig struct {
	// Icon is the picture, drawn about 18 points high. On macOS a
	// Template icon is drawn from its alpha alone, in the menu bar's own
	// color, which is what suits a monochrome glyph.
	Icon     image.Image
	Template bool
	// Title is text beside the icon, or in its place.
	Title   string
	Tooltip string
	// Menu opens when the icon is clicked.
	Menu []MenuItem
	// OnClick runs when an icon with no Menu is clicked.
	OnClick func()
}

// Tray is an icon App.AddTray put in the system tray.
type Tray struct {
	app  *App
	key  int
	cfg  TrayConfig
	icon []byte   // cfg.Icon as PNG, made once per icon
	set  *menuSet // the actions of the menu and the click
}

// TraySupported reports whether AddTray's icons show, which is so on
// macOS. Elsewhere a Tray does nothing.
func TraySupported() bool { return nativemenu.TraySupported() }

// AddTray puts an icon in the system tray. Call it from the UI thread or
// before Run. Its menu actions and OnClick run on the UI thread, as the
// app menus' do.
//
// The tray lives as long as the app does, and the app as long as a
// window is open: an app that lives in the tray keeps a window, hidden
// rather than closed.
//
//	tray := app.AddTray(ggui.TrayConfig{
//		Icon: icon, Template: true,
//		Menu: []ggui.MenuItem{
//			ggui.MenuAction("Show", "", func() { app.Show(); app.Focus() }),
//			ggui.MenuSeparator(),
//			ggui.MenuAction("Quit", "", app.Quit),
//		},
//	})
//	app.OnCloseRequest(func() bool { app.Hide(); return false })
func (a *App) AddTray(cfg TrayConfig) *Tray {
	a.nextTray++
	t := &Tray{app: a, key: a.nextTray, cfg: cfg}
	t.encodeIcon()
	a.trays = append(a.trays, t)
	t.install()
	return t
}

// SetTitle changes the text beside the icon.
func (t *Tray) SetTitle(title string) { t.cfg.Title = title; t.install() }

// SetTooltip changes the text shown while the pointer rests on the icon.
func (t *Tray) SetTooltip(tip string) { t.cfg.Tooltip = tip; t.install() }

// SetIcon changes the picture; see TrayConfig.Icon.
func (t *Tray) SetIcon(icon image.Image, template bool) {
	t.cfg.Icon, t.cfg.Template = icon, template
	t.encodeIcon()
	t.install()
}

// SetMenu replaces the menu the icon opens.
func (t *Tray) SetMenu(items ...MenuItem) { t.cfg.Menu = items; t.install() }

// Remove takes the icon out of the tray.
func (t *Tray) Remove() {
	a := t.app
	if !slices.Contains(a.trays, t) {
		return
	}
	a.trays = slices.DeleteFunc(a.trays, func(o *Tray) bool { return o == t })
	a.release(t.set)
	t.set = nil
	if a.running {
		nativemenu.RemoveTray(t.key)
	}
}

func (t *Tray) encodeIcon() {
	t.icon = nil
	if t.cfg.Icon == nil {
		return
	}
	var b bytes.Buffer
	if png.Encode(&b, t.cfg.Icon) == nil {
		t.icon = b.Bytes()
	}
}

// install numbers the tray's actions and shows it, once the app runs.
func (t *Tray) install() {
	a := t.app
	if !slices.Contains(a.trays, t) {
		return
	}
	a.release(t.set)
	items := t.cfg.Menu
	var click int
	if len(items) == 0 && t.cfg.OnClick != nil {
		// The click is an action of its own, numbered with the menu's.
		items = []MenuItem{{Run: t.cfg.OnClick}}
	}
	t.set = a.newMenuSet(items)
	if len(t.cfg.Menu) == 0 && t.cfg.OnClick != nil {
		click = t.set.ids[0]
	}
	if !a.running || !nativemenu.TraySupported() {
		return
	}
	native := nativemenu.Tray{Icon: t.icon, Template: t.cfg.Template, Title: t.cfg.Title, Tooltip: t.cfg.Tooltip, ClickID: click}
	if len(t.cfg.Menu) > 0 {
		native.Menu = t.set.native
	}
	nativemenu.SetTray(t.key, native)
}
