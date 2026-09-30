# Windows and the desktop

[Documentation](README.md) · [Getting started](getting-started.md) · [Windows example](../examples/windows)

An `App` is the process's GUI: one event loop, one UI thread, one reactive
graph, and any number of windows. This page covers the windows and what an
app asks of the desktop around them: menus, a tray icon, message boxes, the
system appearance and a single running instance.

## On this page

- [Several windows](#several-windows)
- [Controlling a window](#controlling-a-window)
- [Reaching the host from a widget](#reaching-the-host-from-a-widget)
- [Slow calls from a handler](#slow-calls-from-a-handler)
- [App menus](#app-menus)
- [Tray icons](#tray-icons)
- [Message dialogs](#message-dialogs)
- [Light and dark](#light-and-dark)
- [One instance](#one-instance)
- [Platform support](#platform-support)

## Several windows

`ggui.New` makes the app with its main window, which `App` embeds: `app.Post`,
`app.Shortcut` and `app.SetTitle` all reach that window. `app.OpenWindow` adds
another, with a `Config` and a builder of its own:

```go
inspector, err := app.OpenWindow(ggui.Config{
    Title: "Inspector", Width: 320, Height: 240, AlwaysOnTop: true,
}, func() ggui.Widget {
    return ggui.Textf("shared count: %d", count)
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
| `SetPosition`, `Position` | Move the window on the screen. |
| `SetSizeLimits(min, max)` | Bound the size the user can resize to. |
| `SetResizable`, `SetFrameless`, `SetAlwaysOnTop`, `SetIcon` | Change what `Config` set. |
| `Minimize`, `Maximize`, `Restore`, `SetFullscreen` | Change the window's state. |
| `Show`, `Hide`, `Visible` | Show or hide the window without closing it. |
| `Focus`, `RequestAttention` | Bring it forward, or bounce the dock icon. |
| `Screen`; `ggui.Screens()` | The display the window is on; every display. |

What the window is doing is readable as signals, so a widget follows it like any
other state:

```go
status := ggui.Combine(w.Focused(), w.Viewport(), func(focused bool, size ggui.Size) string {
    return fmt.Sprintf("focused: %t · %.0f×%.0f", focused, size.W, size.H)
})
```

`Focused` follows the keyboard focus, `Viewport` the content size, and `State`
whether the window is normal, minimized, maximized or fullscreen.

## Reaching the host from a widget

`ggui.UseHost()` returns the `Host` a component was built in or a handler was
called from: the `Window` under an app, the `Probe` under a test. It reaches
dialogs, the clipboard and `Post` without the model carrying them.
`ggui.UseWindow()` returns the `Window`, or nil under a probe.

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
ggui.Column(ui.AppMenubar(app.Menu()), ggui.Expanded(content))
```

An action runs on the UI thread, where `UseWindow` is the window that last had
the focus. `ggui.NativeMenu()` reports whether the menu bar shows the menus.
`Chord.Label` writes a chord the way the platform shows it: `⇧⌘S` on macOS,
`Ctrl+Shift+S` elsewhere.

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

## Platform support

| Feature | macOS | Windows | Linux | Browser |
| --- | --- | --- | --- | --- |
| Several windows, window control | Yes | Yes | Yes | One window |
| App menus in the menu bar | Yes | Drawn with `ui.AppMenubar` | Drawn with `ui.AppMenubar` | Drawn with `ui.AppMenubar` |
| Tray icon | Yes | No | No | No |
| Message dialogs | Alert sheet | Message box | `zenity` / `kdialog` | `alert` / `confirm` |
| Clipboard | Pasteboard API | Clipboard API | `wl-clipboard`, `xclip`, `xsel` | In-process |
| `SystemDark` | Yes | Yes | Yes | Yes |
| `SingleInstance` | Yes | Yes | Yes | Every tab is one |
| `runtime.OpenURL` | `open` | Default handler | `xdg-open` | New tab |
