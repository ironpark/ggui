# UI themes

`ui/theme` owns the UI design tokens, light/dark defaults, palettes, presets,
chat geometry, and theme application. `ggui` owns rendering, layout, input,
reactivity and typed environment values; it does not import `ui` or `ui/theme`.

```go
import (
    "github.com/ironpark/ggui"
    "github.com/ironpark/ggui/ui"
    "github.com/ironpark/ggui/ui/theme"
)

preset := theme.Preset{Base: theme.BaseNeutral, Accent: theme.AccentBlue}
dark := ggui.State(false)
app.Setup(func() { theme.Bind(dark, preset.Dark(), preset.Light()) })
// In the widget tree:
ui.ThemeSwitch(dark)
ui.Title("Settings")
theme.With(preset.Dark(), preview)
```

Importing `ui/theme`, including through `ui`, installs `theme.Default()` in the
root environment. `theme.Set(t)` replaces the global theme while preserving
unrelated environment values. `theme.Use()` reads it reactively;
`theme.From(env)` resolves the local theme during layout. `t.Apply(env)` returns
an environment with the theme and its core styles, and `theme.With(t, child)`
applies it to a subtree. Changes apply to already-built widgets on layout.

The core consumes only the settings it needs:

| Theme value | Core setting |
| --- | --- |
| `Text` | `Env.Text()` |
| `Bg` | `ggui.BackgroundKey` (unless `Config.Background` is explicit) |
| `Space` | `ggui.SpacingKey`, used by `.Space(n)` |
| `MutedFg`, `Selection` | `ggui.EditorStyleKey` |
| `MutedFg`, `Fg`, `MotionFast` | `ggui.ScrollStyleKey` |
| `MotionFast` | `ggui.PopupDurationKey` |

`TitleKey` and `CaptionKey` belong to `ui/theme`. `ui.Title` and `ui.Caption`
select these keys through the generic `TextWidget.StyleKey` API; a heading's
accessibility role is set separately with `Role(ggui.RoleHeading)`.
A local theme preserves inherited text fields omitted by its `Text` style.
A global `Set` replaces the complete root text style so a previous theme's
font does not leak into the next theme. Accessibility preferences such as
text scale and reduced motion survive both operations.

Core-only applications need no UI package. They can use `ggui.SetEnv`,
`ggui.UseEnv`, `ggui.Provide` and `ggui.WithEnv` to configure the same primitives.

## Migration

This move changes public import paths and APIs; the old paths are not aliases.

| Before | Now |
| --- | --- |
| `ggui/icons`, `ggui/icons/lucide` (and other sets) | `ggui/ui/icons`, `ggui/ui/icons/lucide` |
| `ggui.Theme`, `ggui.ChatTokens` | `theme.Theme`, `theme.ChatTokens` |
| `ggui.DefaultTheme()`, `ggui.DarkTheme()` | `theme.Default()`, `theme.Dark()` |
| `ggui.ThemePreset`, `ggui.ThemeStyle` | `theme.Preset`, `theme.Style` |
| `ggui.BaseColor`, `ggui.AccentColor`, palette/style constants | Same names in `theme` |
| `ggui.BaseColors()`, `ggui.AccentColors()`, `ggui.ThemeStyles()` | `theme.BaseColors()`, `theme.AccentColors()`, `theme.Styles()` |
| `ggui.UseTheme()`, `ggui.SetTheme(t)`, `ggui.BindTheme(...)` | `theme.Use()`, `theme.Set(t)`, `theme.Bind(...)` |
| `env.Theme()`, `env.WithTheme(t)` | `theme.From(env)`, `t.Apply(env)` |
| `ggui.Themed(t, child)` | `theme.With(t, child)` |
| `ggui.Title(s)`, `ggui.Caption(s)` | `ui.Title(s)`, `ui.Caption(s)` |
| `text.AsTitle()` | `text.StyleKey(theme.TitleKey, theme.Default().Title).Role(ggui.RoleHeading)` |
| `text.AsCaption()` | `text.StyleKey(theme.CaptionKey, theme.Default().Caption)` |

Unlike the old `WithTheme`, `Apply` also maps the theme's text, spacing, editor,
scrollbar and popup settings. See [styling](../../docs/styling.md) for usage and
[palette sources](../../internal/themedata/README.md) for upstream data and licenses.
