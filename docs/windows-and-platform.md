# Windows and the desktop

[Documentation](README.md) · [Getting started](getting-started.md) · [Windows example](../examples/windows)

An `App` is the process's GUI: one event loop, one UI thread, one reactive
graph, and any number of windows. This page covers the windows and what an
app asks of the desktop around them: menus, a tray icon, message boxes, the
system appearance and a single running instance.

## On this page

- [Several windows](#several-windows)
- [Controlling a window](#controlling-a-window)
- [Reopening where it was](#reopening-where-it-was)
- [Reaching the host from a widget](#reaching-the-host-from-a-widget)
- [Slow calls from a handler](#slow-calls-from-a-handler)
- [App menus](#app-menus)
- [Context menus](#context-menus)
- [Tray icons](#tray-icons)
- [Message dialogs](#message-dialogs)
- [Light and dark](#light-and-dark)
- [One instance](#one-instance)
- [System languages](#system-languages)
- [Open at login](#open-at-login)
- [Platform support](#platform-support)

## Several windows

`ggui.New` makes the app with its main window, which `App` embeds: `app.Post`,
`app.Shortcut` and `app.SetTitle` all reach that window. `app.OpenWindow` adds
another, with a `Config` and a builder of its own:

```go
inspector, err := app.OpenWindow(ggui.Config{
    Title: "Inspector", Width: 320, Height: 240, AlwaysOnTop: true,
}, func() ggui.Widget {
    return widget.Textf("shared count: %d", count)
})
```

Called before `Run`, the window waits and opens with the main one; called while
the app runs, it opens at once and its builder runs before `OpenWindow`
returns. Each window has its own root owner, disposed when it closes, and its
own shortcuts, close handlers, dialogs and inspector.

State is shared by being ordinary signals. A write in one window repaints every
window that reads it, including a window that is covered or minimized, whose
effects run straight away and whose paint waits for it to show.

`Window.Close` closes one window without asking its `OnCloseRequest` handlers.
`Run` returns when the last window closes. `app.Close` closes them all from the
UI thread, and `app.Quit` does the same from any goroutine.

## Controlling a window

Every `Window`, the main one included, can be changed while it is open. Before
it opens, the same calls change the `Config` it will open with.

| Call | Does |
| --- | --- |
| `SetTitle`, `Title` | Change or read the title. |
| `SetSize`, `Size` | Resize the content, in logical pixels. |
| `SetPosition`, `Position` | Move the window on its screen, from the screen's top-left corner. |
| `SetSizeLimits(min, max)` | Bound the size the user can resize to. |
| `SetResizable`, `SetFrameless`, `SetAlwaysOnTop`, `SetIcon` | Change what `Config` set. |
| `Minimize`, `Maximize`, `Restore`, `SetFullscreen` | Change the window's state. |
| `Show`, `Hide`, `Visible` | Show or hide the window without closing it. |
| `Focus`, `RequestAttention` | Bring it forward, or bounce the dock icon. |
| `Screen`; `ggui.Screens()` | The display the window is on; every display, the primary first. |

A `Screen` has a `Name` for people and an `ID` to save, which tells two displays
of one model apart and is the same after a restart. `Position` is the display's
corner in the desktop, `Size` its size and `WorkArea` the part the menu bar,
dock and taskbar leave to windows. `Config.Screen` opens a window on the display
of that `ID`, with `Config.Position` on it; a display that is not connected
leaves the window to the default one.

What the window is doing is readable as signals, so a widget follows it like any
other state:

```go
status := ggui.Combine(w.Focused(), w.Viewport(), func(focused bool, size ggui.Size) string {
    return fmt.Sprintf("focused: %t · %.0f×%.0f", focused, size.W, size.H)
})
```

`Focused` follows the keyboard focus, `Viewport` the content size, and `State`
whether the window is normal, minimized, maximized or fullscreen, whether the
app, the user or the OS changed it.

## Reopening where it was

`Config.Remember` names a window's placement, and the window reopens where it
was last closed: on the same display, at the same position and size, and
maximized if it was.

```go
app := ggui.New(ggui.Config{
    Title: "Notes", Width: 900, Height: 640, Resizable: true,
    Remember: "main",
}, build)
prefs, err := app.OpenWindow(ggui.Config{Title: "Preferences", Remember: "preferences"}, settings)
```

The window keeps its placement when it closes, by its close button, `Close`,
`app.Close`, `app.Quit` or the platform's Quit. What it keeps is the frame it
had while it was last shown normally: closed maximized, it reopens maximized,
and `Restore` takes it back to that frame. A frame it had while minimized or
fullscreen is never kept.

The placement is kept as the display's `ID` and the position on that display,
so a window comes back to its display even after the displays were rearranged,
and two displays of one model are not confused. When the window opens, a saved
placement takes the place of `Screen`, `Position`, `Width`, `Height` and
`Maximized`, and the window is placed on its display before it first shows.
The size stays within `MinWidth`, `MinHeight`, `MaxWidth` and `MaxHeight`, and
within the display's work area. A position that would leave the window's top
edge out of reach, as after the display's resolution was lowered, is moved just
far enough to put the window in the work area. When the display is no longer
connected, the window keeps its saved size and opens where `Screen` and
`Position` put it, centered on the default display when they are empty.

The placements of an app's windows live in `windows.json`, in a directory of
`os.UserConfigDir()` named for the app: the id given to `app.SingleInstance`
when there is one, such as `~/Library/Application Support/com.example.notes`,
and otherwise the main window's title as `New` was given it, or the
executable's name when it was given none. Each window of an
app needs a `Remember` name of its own. A file that cannot be read or written
never stops the app: the window opens as its `Config` says, and the trouble is
said once on stderr.

Nothing is kept under a `Probe` or a `WindowProbe`, in a browser, or in the
viewer of `ggui dev`, whose one window is the viewer's.

Run by `ggui dev` in a window of its own (`--display window`), an app whose
`Config` was edited to place the window otherwise, in its `Screen`,
`Position`, `Width`, `Height` or `Maximized`, opens as the code now says
rather than where it was last closed, and says so once: the edit is what was
just asked for. The placement is kept again from there. An app run by itself
never does this, so its users' placements outlive a release that changes the
defaults.

## Reaching the host from a widget

`ggui.UseHost()` returns the `Host` a component was built in or a handler was
called from: the `Window` under an app, the `Probe` under a test. It reaches
dialogs, the clipboard and `Post` without the model carrying them.
`ggui.UseWindow()` returns the `Window`, or nil under a probe. Outside any
window or probe `UseHost` panics; `ggui.LookupHost()` reports false there
instead, for a widget that may also be laid out on its own and can do
without the host.

```go
ui.Button("Close", func() { ggui.UseWindow().Close() })
```

A component captures the host while it is built to use it later from a
goroutine.

## Slow calls from a handler

`ggui.Async(work, done)` runs `work` on a goroutine and hands its result to
`done` on the UI thread. It is the shape of every slow call a handler makes: a
dialog, a request, a query. `done` may write signals; `work` must not.

```go
ui.Button("Open…", func() {
    host := ggui.UseHost()
    ggui.Async(func() (string, error) {
        return host.Dialogs().OpenFile(runtime.FileDialog{Title: "Open"})
    }, func(path string, err error) {
        if err == nil {
            m.Path.Set(path)
        }
    })
})
```

`Resource` is the form for a value that follows a signal and is cancelled when
it changes; `Async` is for a one-off action.

## App menus

`app.SetMenu` installs the app's menus from `ggui.Menu`, `ggui.MenuAction` and
`ggui.MenuSeparator`. An action takes a chord, and can follow a signal for its
enabled state and for a check mark:

```go
app.SetMenu(
    ggui.Menu("File",
        ggui.MenuAction("New", "cmd+n", m.create),
        ggui.MenuAction("Save", "cmd+s", m.save).BindEnabled(m.Dirty),
        ggui.MenuSeparator(),
        ggui.Menu("Recent", recent...),
    ),
    ggui.Menu("View",
        ggui.MenuAction("Dark Mode", "", func() { ggui.Toggle(m.Dark) }).BindChecked(m.Dark),
    ),
)
```

On macOS the menus go in the menu bar, between the application menu (About,
Hide, Quit) and the Window menu, and their chords are the menu's key
equivalents. Elsewhere every window runs the chords as shortcuts, and
`ui.AppMenubar` draws the menus inside a window. It draws nothing on macOS, so
one tree serves every platform:

```go
widget.Column(ui.AppMenubar(app.Menu()), widget.Expanded(content))
```

`ggui.EditMenu` is the usual Edit menu: Undo, Redo, Cut, Copy, Paste and
Select All. A focused text field takes its own editing chords ahead of any
shortcut or menu action on them, so with a field focused Edit ▸ Undo, or ⌘Z,
undoes the typing, and with none it runs the app's action:

```go
app.SetMenu(fileMenu, ggui.EditMenu(ggui.EditActions{Undo: m.undo, Redo: m.redo}))
```

A widget of your own keeps chords the same way by implementing
`ggui.ChordClaimer`.

An action runs on the UI thread, where `UseWindow` is the window that last had
the focus. `ggui.NativeMenu()` reports whether the menu bar shows the menus.
`Chord.Label` writes a chord the way the platform shows it: `⇧⌘S` on macOS,
`Ctrl+Shift+S` elsewhere.

## Context menus

`ui.NativeContextMenu` wraps content with a native context menu on macOS.
It accepts the same `ggui.MenuItem` model as the menu bar and tray, including
submenus, separators, enabled state, and check marks:

```go
ui.NativeContextMenu(widget.Text("Right-click me"),
    ggui.MenuAction("Open", "", open),
    ggui.MenuAction("Save", "", save).BindEnabled(dirty),
    ggui.MenuSeparator(),
    ggui.Menu("More", ggui.MenuAction("Duplicate", "", duplicate)),
)
```

`ui.NativeContextMenuFunc(content,
func() []ggui.MenuItem { ... })` builds entries at invocation time. Secondary
click, Shift+F10, Enter, Space, and accessibility actions can open the menu.
Windows, Linux, browsers, and probes use the regular widget context menu;
its submenus are displayed inline, as with `ui.AppMenubar`.

For custom controls, `window.ShowContextMenu(position, items...)` opens the
native menu directly. Positions are logical content coordinates from the
window's top-left corner. Call it on the UI thread; it waits for dismissal,
runs the selected action, and releases the temporary entries. It returns
false when unavailable; `ggui.NativeContextMenu()` reports platform support.

## Tray icons

`app.AddTray` puts an icon in the system tray, with a menu of the same items or
a click handler:

```go
tray := app.AddTray(ggui.TrayConfig{
    Icon: icon, Template: true, Tooltip: "Sync",
    Menu: []ggui.MenuItem{
        ggui.MenuAction("Show", "", func() { app.Show(); app.Focus() }),
        ggui.MenuSeparator(),
        ggui.MenuAction("Quit", "", app.Quit),
    },
})
ggui.Watch(pending, func(n int) { tray.SetTitle(fmt.Sprint(n)) })
```

A `Template` icon is drawn from its alpha alone in the menu bar's own color,
which suits a monochrome glyph. `SetTitle`, `SetTooltip`, `SetIcon` and
`SetMenu` change a tray, and `Remove` takes it away.

On Windows the icon goes in the taskbar's notification area. A `Template` icon
is drawn white on a dark taskbar and black on a light one, following the
setting as it changes. The tray shows no text, so `Title` serves as the tooltip
when `Tooltip` is empty, and a tray without an `Icon` shows the executable's.
The menu opens on a click of either button; `OnClick` runs on the primary
button or Enter. The icons come back if Explorer restarts.

The app runs as long as a window is open, so an app that lives in the tray
keeps its window and hides it rather than closing it:

```go
app.OnCloseRequest(func() bool { app.Hide(); return false })
```

Work posted to a hidden window, like a tray action, still runs.

## Message dialogs

`Dialogs().Message` asks a question in the platform's own message box and
returns the index of the button pressed, the default first:

```go
host := ggui.UseHost()
ggui.Async(func() (int, error) {
    return host.Dialogs().Message(runtime.Message{
        Kind:    runtime.MessageWarning,
        Title:   "Discard changes?",
        Detail:  "Your edits will be lost.",
        Buttons: []string{"Discard", "Cancel"},
    })
}, func(answer int, err error) {
    if err == nil && answer == 0 {
        m.discard()
    }
})
```

On macOS it is an alert attached to the window as a sheet; on Linux `zenity` or
`kdialog`; in a browser `alert` or `confirm`. The Windows message box has fixed
buttons whatever the labels say: one is OK, two are OK and Cancel (Yes and No
for a `MessageQuestion`), three are Yes, No and Cancel. A `Probe` answers with
its stub's `Button`.

## Light and dark

`ggui.SystemDark()` follows whether the desktop is set to a dark appearance, so
an app follows the system with one line:

```go
app.Setup(func() { theme.Bind(ggui.SystemDark(), theme.Dark(), theme.Default()) })
```

It is read about once a second while the app runs. `theme.Set` matches the
windows' title bars to the theme it applies; `ggui.SetAppearance` sets them
directly.

## One instance

`app.SingleInstance` keeps one process of the app running for the user. A later
launch hands its arguments and working directory to the first and gets
`runtime.ErrRunning`, which is its cue to exit:

```go
err := app.SingleInstance("com.example.editor", func(l runtime.Launch) {
    app.Show()
    app.Focus()
    m.open(l.Dir, l.Args...)
})
if errors.Is(err, runtime.ErrRunning) {
    return
}
```

The callback runs on the UI thread. The instances meet at a local socket that
only the same user can reach.

## System languages

`ggui.SystemLanguages()` returns the languages the user prefers, most preferred
first, as BCP 47 tags: `["ko-KR", "en-US"]`. An app picks its translation from
them:

```go
lang := i18n.Match([]string{"en", "ko", "ja"}, ggui.SystemLanguages())
```

It asks the platform on every call, from any goroutine, so a change in the
system's settings shows in the next call; it returns an empty list when the
platform says nothing. macOS gives the list in Language & Region settings
(`CFLocaleCopyPreferredLanguages`), Windows the user's preferred display
languages (`GetUserPreferredUILanguages`, else the user's locale), and a
browser `navigator.languages`. Linux and the BSDs read `LANGUAGE`, a list
separated by colons, then the first of `LC_ALL`, `LC_MESSAGES` and `LANG`; as
in gettext, a `C` or `POSIX` locale asks for no translation and leaves
`LANGUAGE` unread. A POSIX name such as `ko_KR.UTF-8@mod` becomes `ko-KR`, and
each tag appears once.

## Open at login

`ggui.SetOpenAtLogin(true)` has the system start the app when the user logs in,
and `SetOpenAtLogin(false)` stops it; `ggui.OpenAtLogin()` reports whether it
will. Call them from a setting the user changes, not on every launch, and show
`OpenAtLogin` as that setting's state:

```go
on, err := ggui.OpenAtLogin()
if err == nil {
    openAtLogin.Set(on)
}
...
ui.Switch(ggui.Bind(openAtLogin.Get, func(v bool) {
    if err := ggui.SetOpenAtLogin(v); err == nil {
        openAtLogin.Set(v)
    }
}), "Open at login")
```

What is registered is the executable running now, links resolved, so call it
from the installed app; an app inside a bundle registers its
`Contents/MacOS/<name>`. The registration is named for the app: the bundle
identifier on macOS when the app runs from a bundle, else the id given to
`app.SingleInstance`, else the executable's name. Keep it the same between
releases, so a new version finds what an old one registered.

| Platform | Registration |
| --- | --- |
| macOS | A LaunchAgent, `~/Library/LaunchAgents/<id>.plist`, with `RunAtLoad`. It takes effect from the next login, and System Settings lists it under Login Items. |
| Windows | A value named `<id>` in `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, holding the quoted path. An entry the user turned off in Task Manager counts as off, and `SetOpenAtLogin` turns it back on. |
| Linux and BSD | An XDG autostart entry, `$XDG_CONFIG_HOME/autostart/<id>.desktop` (`~/.config/autostart` by default). |
| Browser | Unsupported: both return an error wrapping `errors.ErrUnsupported`. |

The files are written whole through a temporary file renamed over the old one.

## Platform support

| Feature | macOS | Windows | Linux | Browser |
| --- | --- | --- | --- | --- |
| Several windows, window control | Yes | Yes | Yes | One window |
| `Config.Remember` | Yes | Yes | Yes | No |
| App menus in the menu bar | Yes | Drawn with `ui.AppMenubar` | Drawn with `ui.AppMenubar` | Drawn with `ui.AppMenubar` |
| Native context menus | Yes | Widget fallback | Widget fallback | Widget fallback |
| Tray icon | Yes | No | No | No |
| Message dialogs | Alert sheet | Message box | `zenity` / `kdialog` | `alert` / `confirm` |
| Clipboard | Pasteboard API | Clipboard API | X11 `CLIPBOARD` selection | Paste events and `navigator.clipboard` |
| `SystemDark` | Yes | Yes | Yes | Yes |
| `SingleInstance` | Yes | Yes | Yes | Every tab is one |
| `SystemLanguages` | Yes | Yes | Environment | `navigator.languages` |
| `SetOpenAtLogin` | LaunchAgent | `Run` key | XDG autostart | No |
| `runtime.OpenURL` | `open` | Default handler | `xdg-open` | New tab |
| `runtime.Reveal` | Selected in Finder | Selected in Explorer | FileManager1 over D-Bus, else the folder | No |
