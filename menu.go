package ggui

import (
	"bytes"
	"image"
	"image/png"
	"slices"
	"strings"

	"github.com/ironpark/ggui/internal/platform/nativemenu"
	"github.com/ironpark/ggwd/native"
)

// MenuItem is one entry of an app's menus: an action, a separator, or a
// submenu. Build them with Menu, MenuAction and MenuSeparator, and hand the
// top-level menus to App.SetMenu.
//
//	app.SetMenu(
//		ggui.Menu("File",
//			ggui.MenuAction("New", "cmd+n", m.create),
//			ggui.MenuAction("Save", "cmd+s", m.save).BindEnabled(m.Dirty),
//			ggui.MenuSeparator(),
//			ggui.MenuAction("Close Window", "cmd+w", func() { ggui.UseWindow().Close() }),
//		),
//		ggui.Menu("View",
//			ggui.MenuAction("Dark Mode", "", func() { ggui.Toggle(m.Dark) }).BindChecked(m.Dark),
//		),
//	)
type MenuItem struct {
	Label string
	// Chord is the key that runs the action, such as "cmd+s"; see
	// ParseChord. Empty for none.
	Chord string
	Run   func()
	// Enabled, when set, greys the action out while it is false.
	Enabled Readable[bool]
	// Checked, when set, puts a check mark by the action while it is true.
	Checked   Readable[bool]
	Items     []MenuItem // a submenu's entries
	Separator bool
}

// Menu returns a menu, or a submenu within one, labelled label.
func Menu(label string, items ...MenuItem) MenuItem {
	return MenuItem{Label: label, Items: items}
}

// MenuAction returns an entry that runs run, on chord when it is not
// empty. It panics on a chord ParseChord rejects.
func MenuAction(label, chord string, run func()) MenuItem {
	if chord != "" {
		MustChord(chord)
	}
	return MenuItem{Label: label, Chord: chord, Run: run}
}

// EditActions is what an app's Edit menu does when no focused field takes
// the command. A nil action does nothing then.
type EditActions struct {
	Undo, Redo, Cut, Copy, Paste, SelectAll func()
}

// EditMenu returns the usual Edit menu: Undo, Redo, Cut, Copy, Paste and
// Select All on their usual chords. Each goes to the focused text field
// while one has the focus, as the key would, and to app's action
// otherwise, so one Undo serves the field being edited and the document.
func EditMenu(app EditActions) MenuItem {
	return Menu("Edit",
		MenuAction("Undo", "cmd+z", app.Undo),
		MenuAction("Redo", "cmd+shift+z", app.Redo),
		MenuSeparator(),
		MenuAction("Cut", "cmd+x", app.Cut),
		MenuAction("Copy", "cmd+c", app.Copy),
		MenuAction("Paste", "cmd+v", app.Paste),
		MenuAction("Select All", "cmd+a", app.SelectAll),
	)
}

// MenuSeparator returns a line between groups of entries.
func MenuSeparator() MenuItem { return MenuItem{Separator: true} }

// BindEnabled greys the entry out while r is false.
func (m MenuItem) BindEnabled(r Readable[bool]) MenuItem { m.Enabled = r; return m }

// BindChecked puts a check mark by the entry while r is true.
func (m MenuItem) BindChecked(r Readable[bool]) MenuItem { m.Checked = r; return m }

// NativeMenu reports whether App.SetMenu's menus appear in the platform's
// menu bar, which is so on macOS. Elsewhere an app draws them itself, with
// ui.AppMenubar, and their chords work either way.
func NativeMenu() bool { return nativemenu.Supported() }

// menuSet is one group of menus the app installed: the menu bar's, or a
// tray's. It owns the actions it numbered and the watchers that follow
// them.
type menuSet struct {
	items   []MenuItem
	ids     []int             // the actions it numbered
	native  []nativemenu.Item // the items as the platform half takes them
	dispose func()            // stops the watchers
}

// newMenuSet numbers the actions of items and follows their Enabled and
// Checked readers until release.
func (a *App) newMenuSet(items []MenuItem) *menuSet {
	s := &menuSet{items: items}
	s.dispose = Root(func() {
		for _, m := range items {
			s.native = append(s.native, a.nativeItem(s, m))
		}
	})
	return s
}

// release forgets the set's actions and stops its watchers.
func (a *App) release(s *menuSet) {
	if s == nil {
		return
	}
	s.dispose()
	for _, id := range s.ids {
		delete(a.actions, id)
	}
}

// SetMenu replaces the app's menus. On macOS they go in the menu bar,
// between the application menu and the Window menu, and their chords are
// the menu's key equivalents. Elsewhere every window runs the chords as
// shortcuts, and ui.AppMenubar draws the menus inside a window. An action
// runs on the UI thread, where UseWindow is the window that had the focus.
// Call SetMenu from the UI thread or before Run.
func (a *App) SetMenu(menus ...MenuItem) {
	a.release(a.menu)
	a.menu = a.newMenuSet(menus)
	if nativemenu.Supported() {
		if a.running {
			nativemenu.Set(a.menu.native)
		}
		return
	}
	for _, w := range a.Windows() {
		a.addMenuShortcuts(w)
	}
}

// Menu returns the menus SetMenu installed.
func (a *App) Menu() []MenuItem {
	if a.menu == nil {
		return nil
	}
	return a.menu.items
}

// nativeItem turns m into what the platform half takes, numbering its
// actions into s and following their Enabled and Checked readers.
func (a *App) nativeItem(s *menuSet, m MenuItem) nativemenu.Item {
	if m.Separator {
		return nativemenu.Item{Separator: true}
	}
	if len(m.Items) > 0 {
		it := nativemenu.Item{Title: m.Label}
		for _, e := range m.Items {
			it.Items = append(it.Items, a.nativeItem(s, e))
		}
		return it
	}
	if a.actions == nil {
		a.actions = map[int]MenuItem{}
	}
	a.nextAction++
	id := a.nextAction
	a.actions[id] = m
	s.ids = append(s.ids, id)
	it := nativemenu.Item{Title: m.Label, ID: id, Enabled: true}
	if m.Chord != "" {
		c := MustChord(m.Chord)
		it.Key = keyEquivalent(c.Key)
		it.Cmd, it.Shift, it.Alt, it.Ctrl = c.Cmd || c.Mods.Meta, c.Mods.Shift, c.Mods.Alt, c.Mods.Ctrl
	}
	if m.Enabled != nil {
		it.Enabled = Untrack(m.Enabled.Get)
		Watch(m.Enabled, func(on bool) { nativemenu.SetEnabled(id, on) })
	}
	if m.Checked != nil {
		it.Checked = Untrack(m.Checked.Get)
		Watch(m.Checked, func(on bool) { nativemenu.SetChecked(id, on) })
	}
	return it
}

// menuSelected runs on the main thread when a native item is chosen, and
// hands the action to the UI thread.
func (a *App) menuSelected(id int) {
	if w := a.activeWindow(); w != nil {
		w.post(func() { a.runAction(id) })
	}
}

// activeWindow is the window a platform callback's work goes to: the one
// that last had the focus, or the first open one, or nil when none is.
func (a *App) activeWindow() *Window {
	if w := a.focus.Load(); w != nil && !w.Closed() {
		return w
	}
	if open := a.Windows(); len(open) > 0 {
		return open[0]
	}
	return nil
}

// runAction runs the action with the id, unless it is disabled or gone.
// When the focused widget claims the action's chord, as a text field does
// ⌘Z, the chord goes to the widget instead, the way the key would have:
// Edit ▸ Undo undoes the field being edited.
func (a *App) runAction(id int) {
	a.runActionIn(a.activeWindow(), id)
}

func (a *App) runActionIn(w *Window, id int) {
	m, ok := a.actions[id]
	if !ok {
		return
	}
	if m.Chord != "" {
		c := MustChord(m.Chord)
		if w != nil && w.input.sendClaimed(KeyEvent{Kind: KeyPress, Key: c.Key, Mods: c.held()}) {
			return
		}
	}
	if m.Run == nil || (m.Enabled != nil && !Untrack(m.Enabled.Get)) {
		return
	}
	m.Run()
}

// addMenuShortcuts makes w run the menus' chords, where the menu bar does
// not. The shortcuts are removed when SetMenu replaces the menus.
func (a *App) addMenuShortcuts(w *Window) {
	for _, h := range w.menuShortcuts {
		h.Remove()
	}
	w.menuShortcuts = nil
	if a.menu == nil {
		return
	}
	for _, id := range a.menu.ids {
		if chord := a.actions[id].Chord; chord != "" {
			w.menuShortcuts = append(w.menuShortcuts, w.Shortcut(chord, func() { a.runAction(id) }))
		}
	}
}

// keyEquivalent is the character AppKit shows and matches for k: the
// character the key types on a US layout, or one of the function-key
// characters for a key that types none.
func keyEquivalent(k KeyboardKey) string {
	switch {
	case k >= KeyA && k <= KeyZ:
		return string(rune('a' + (k - KeyA)))
	case k >= KeyDigit0 && k <= KeyDigit9:
		return string(rune('0' + (k - KeyDigit0)))
	case k >= KeyF1 && k <= KeyF24:
		return string(rune(0xF704 + (k - KeyF1)))
	}
	switch k {
	case KeyEnter, KeyNumpadEnter:
		return "\r"
	case KeyEscape:
		return "\x1b"
	case KeyTab:
		return "\t"
	case KeySpace:
		return " "
	case KeyBackspace:
		return "\x08"
	case KeyDelete:
		return ""
	case KeyArrowUp:
		return ""
	case KeyArrowDown:
		return ""
	case KeyArrowLeft:
		return ""
	case KeyArrowRight:
		return ""
	case KeyHome:
		return ""
	case KeyEnd:
		return ""
	case KeyPageUp:
		return ""
	case KeyPageDown:
		return ""
	case KeyComma:
		return ","
	case KeyPeriod:
		return "."
	case KeySlash:
		return "/"
	case KeySemicolon:
		return ";"
	case KeyQuote:
		return "'"
	case KeyBracketLeft:
		return "["
	case KeyBracketRight:
		return "]"
	case KeyBackslash:
		return "\\"
	case KeyMinus:
		return "-"
	case KeyEqual:
		return "="
	case KeyBackquote:
		return "`"
	}
	return strings.ToLower(k.String())
}

// NativeContextMenu reports whether the platform supports native context menus.
func NativeContextMenu() bool { return nativemenu.ContextSupported() }

// ShowContextMenu opens a native menu at a position in window content coordinates
// (logical points, measured from the top left). It waits until dismissal and runs
// the chosen action on the UI thread before returning. Call it on the UI thread.
// It returns false if the platform is unsupported, the window is not open, or
// there are no entries; dismissal without selecting an action still returns true.
func (w *Window) ShowContextMenu(at Point, items ...MenuItem) bool {
	if w == nil || w.Closed() || !NativeContextMenu() || len(items) == 0 {
		return false
	}
	nw := w.window.Load()
	if nw == nil {
		return false
	}
	owner := native.Handle(nw)
	if owner == 0 {
		return false
	}
	s := w.app.newMenuSet(items)
	defer w.app.release(s)
	id := nativemenu.Popup(owner, float64(at.X), float64(at.Y), s.native)
	if id != 0 {
		w.app.runActionIn(w, id)
	}
	return true
}

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
//	quitting := false
//	app.OnQuitRequest(func() { quitting = true })
//	app.OnCloseRequest(func() bool {
//		if quitting {
//			return true
//		}
//		app.Hide()
//		return false
//	})
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
