# Getting started

[Documentation](README.md) · [Project README](../README.md)

Start with the [complete counter example](../README.md#quick-start). This page
explains the app configuration and lifecycle used by that example.

## On this page

- [Create an app](#create-an-app)
- [Configure the window](#configure-the-window)
- [Set up state and cleanup](#set-up-state-and-cleanup)

## Create an app

In your Go module, install the library:

```sh
go get github.com/ironpark/ggui
```

The repository requires Go 1.27+ for generic methods. The core package provides
layout, state, rendering, and input; `github.com/ironpark/ggui/ui` provides themed
controls. Platform build dependencies still apply even though ggui itself does
not use CGO. See the [README prerequisites](../README.md#install).

`ggui.New(config, build)` creates an app. Call `app.Run()` to open its window
and run the event loop; check the returned error. `ggui.Run(config, build)` is a
shorthand when you do not need to register setup or shortcuts first.

## Configure the window

| `Config` field | Default | Use |
| --- | --- | --- |
| `Title` | `"ggui"` | Window title. |
| `Width`, `Height` | `800`, `600` when zero | Initial window dimensions. |
| `MinWidth`, `MinHeight`, `MaxWidth`, `MaxHeight` | `0`, no limit | Bound the size the user can resize to. |
| `Position` | `nil`, centered | Open at a point on the screen, in logical pixels. |
| `Resizable` | `false` | Allow the user to resize the window. |
| `Frameless`, `AlwaysOnTop`, `Transparent` | `false` | Drop the title bar, float above other windows, or let the desktop show through where nothing is painted. |
| `Hidden`, `Maximized`, `Unfocused` | `false` | Open hidden (call `Show` later), maximized, or without taking the focus. |
| `Icon` | `nil` | The window icon, in several sizes. |
| `Background` | `nil` | Follow `ggui.BackgroundKey` (the UI theme maps `Bg` to it); set a color to override it. |
| `Inspector` | zero, disabled | Bind a chord such as `"f1"` to the [inspector](inspector.md). |
| `Accessibility` | `AccessibilityAuto` | Choose when the macOS [accessibility bridge](accessibility.md) is active. |

The same `Config` opens more windows with `app.OpenWindow`, and the window it
describes can be moved, resized and retitled while the app runs. See
[Windows and the desktop](windows-and-platform.md).

## Set up state and cleanup

The root `build` callback runs once. `Component` setup also runs once per mount.
Use bindings, `View`, `Reactive`, or control-flow blocks for subsequent updates;
reading `state.Get()` in setup alone does not make that setup reactive.

Register app-wide effects and theme bindings before calling `Run`:

```go
dark := ggui.State(false)
app := ggui.New(ggui.Config{Title: "Settings", Resizable: true}, func() ggui.Widget {
    return ui.ThemeSwitch(dark).Name("Dark mode")
})
app.Setup(func() {
    theme.Bind(dark, theme.Dark(), theme.Default())
    ggui.OnCleanup(func() { /* release app-owned resources */ })
})
```

This snippet uses `ggui`, `ui`, and `theme`
(`github.com/ironpark/ggui/ui/theme`).
`Setup` runs under the app's root owner before the first build. `Run` disposes
owned work when it returns, including after an error. `Run` returns when the
last window closes. Call `app.Close()` on the UI thread to close every window
and end `Run`; from any other goroutine, call `app.Quit()`.

State and widget operations belong to the UI thread. Post background results
with `app.Post(func() { result.Set(value) })`, or use a
[`Resource`](reactivity.md#resources-and-await-blocks) for cancellation and
lifetime handling, or [`ggui.Async`](windows-and-platform.md#slow-calls-from-a-handler)
for a one-off call. `Post` queues work for the next frame, runs it even while
the window is hidden or covered, and drops queued work after shutdown. Work
posted before `app.Run` is kept, and runs once after `Setup` and the first
build, so a goroutine started in `main` may Post from its first line.

`app.OnFrame(fn)` registers per-frame work before input and effect processing.
Prefer owned effects or resources when the work belongs to a component, and
[`app.Shortcut`](input.md#shortcuts) for keyboard commands.

Continue with [State and components](reactivity.md), or run the
[example applications](../README.md#examples).
