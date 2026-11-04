// Package nativemenu puts an app's menus where the platform shows them:
// macOS's menu bar, and icons in its status area. It knows nothing of
// widgets or signals; ggui hands it trees of items and gets back the id of
// the action chosen.
package nativemenu

// Item is one entry of a menu: an action, a separator, or a submenu when
// Items is not empty.
type Item struct {
	Title     string
	Separator bool
	Items     []Item

	// ID names the action to the Handle callback. It is ignored for
	// separators and submenus.
	ID int
	// Key is the key equivalent: a character, or one of the function-key
	// characters AppKit defines for keys that type none. Empty for none.
	Key string
	// Mods are the key equivalent's modifiers.
	Cmd, Shift, Alt, Ctrl bool

	Enabled bool
	Checked bool
}

// Tray is an icon in the status area.
type Tray struct {
	// Icon is a PNG image; nil for none.
	Icon []byte
	// Template draws Icon from its alpha in the menu bar's own color.
	Template bool
	Title    string
	Tooltip  string
	// Menu opens when the icon is clicked. Without one, a click is the
	// action ClickID.
	Menu    []Item
	ClickID int
}
