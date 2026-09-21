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
| `Resizable` | `false` | Allow the user to resize the window. |
| `Background` | `nil` | Follow the theme's `Bg`; set a color to override it. |
| `Inspector` | zero, disabled | Bind a chord such as `"f1"` to the [inspector](inspector.md). |
| `Accessibility` | `AccessibilityAuto` | Choose when the macOS [accessibility bridge](accessibility.md) is active. |

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
    ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme())
    ggui.OnCleanup(func() { /* release app-owned resources */ })
})
```

This snippet uses the `ggui` and `ui` imports from the counter example.
`Setup` runs under the app's root owner before the first build. `Run` disposes
owned work when it returns, including after an error. Call `app.Close()` on
the UI thread to request shutdown. From a worker, use `app.Post(app.Close)`.

State and widget operations belong to the UI thread. Post background results
with `app.Post(func() { result.Set(value) })`, or use a
[`Resource`](reactivity.md#resources-and-await-blocks) for cancellation and
lifetime handling. `Post` queues work for the next frame and drops queued work
after shutdown.

`app.OnFrame(fn)` registers per-frame work before input and effect processing.
Prefer owned effects or resources when the work belongs to a component, and
[`app.Shortcut`](input.md#shortcuts) for keyboard commands.

Continue with [State and components](reactivity.md), or run the
[example applications](../README.md#examples).
