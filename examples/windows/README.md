# Windows

Several windows over one piece of state. The main window counts; each
inspector window it opens shows the same count and can change it, so a click
in one repaints them all. The title bar follows the count, every window
follows the desktop's light or dark setting, and each one reports its focus
and size through `Window.Focused` and `Window.Viewport`.

The app menus (Counter, View) go in the menu bar on macOS and are drawn by
`ui.AppMenubar` elsewhere; their chords work in every window. On macOS a tray
icon shows the count and a menu of its own.

```sh
go run ./examples/windows
go run ./examples/windows -smoke   # open, resize, maximize and close windows, report, quit
```
