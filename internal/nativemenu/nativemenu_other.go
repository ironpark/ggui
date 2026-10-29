//go:build !darwin || ios

package nativemenu

// Supported reports whether the platform shows Set's menus.
func Supported() bool { return false }

// TraySupported reports whether the platform shows SetTray's icons.
func TraySupported() bool { return false }

// Handle does nothing where there is nothing to choose from.
func Handle(func(id int)) {}

// Set does nothing where there is no menu bar to fill.
func Set([]Item) {}

// SetEnabled does nothing where there is no menu bar.
func SetEnabled(int, bool) {}

// SetChecked does nothing where there is no menu bar.
func SetChecked(int, bool) {}

// SetTray does nothing where there is no status area.
func SetTray(int, Tray) {}

// RemoveTray does nothing where there is no status area.
func RemoveTray(int) {}

// ContextSupported reports whether native popup menus are available.
func ContextSupported() bool { return false }

// Popup does nothing on unsupported platforms.
func Popup(uintptr, float64, float64, []Item) int { return 0 }
