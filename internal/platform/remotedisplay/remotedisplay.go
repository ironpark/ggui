// Package remotedisplay says whether the app's window is shown by another
// process: ggui dev's viewer (see cmd/ggviewer and ggwd.Remote). Such an
// app opens no window of its own: its window, native menu bar (on macOS),
// and system tray icons are forwarded to the viewer, while native popup
// menus and the accessibility tree remain local-only. The packages that use
// those ask On and take their in-window or empty path instead.
//
// It is known before Run, from the build and the environment, by the rule
// ggwd decides by when Run starts: an app built with the ggwd_remote tag
// and started with ggwd/remote.EnvSocket set.
package remotedisplay

// On reports whether the app's window is shown by a viewer.
func On() bool { return on() }
