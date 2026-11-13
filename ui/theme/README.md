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
| `MotionOverlay` | `ggui.PopupDurationKey` |

`TitleKey`, `HeadingKey`, `LabelKey`, `CaptionKey` and `MonoKey` belong to
`ui/theme`. `ui.Title`, `ui.Heading`, `ui.Caption` and `ui.Mono` select these
keys through the generic `TextWidget.StyleKey` API; a heading's accessibility
role is set separately with `Role(ggui.RoleHeading)`.

`Apply`, `Set` and `With` resolve the theme first: tokens that follow others,
such as `PrimaryHover` following `Primary`, take their source's value unless
set, and `theme.Override(fn, child)` changes tokens for a subtree with the
followers following there too. `theme.FromCSS` reads a shadcn/ui stylesheet.
See [styling](../../docs/styling.md#deriving-a-theme).
A local theme preserves inherited text fields omitted by its `Text` style.
A global `Set` replaces the complete root text style so a previous theme's
font does not leak into the next theme. Accessibility preferences such as
text scale and reduced motion survive both operations.

Core-only applications need no UI package. They can use `ggui.SetEnv`,
`ggui.UseEnv`, `ggui.Provide` and `ggui.WithEnv` to configure the same primitives.

## Reusable configurations

`Config` is a small, comparable recipe for an application theme or an interactive
builder. `Config{}` produces exactly `Preset{}`: Neutral/Nova, the base palette's
own accent and chart colors, default corners and the application's default font.
It leaves `Default`, `Dark`, and `Preset` unchanged.

```go
var AppTheme = theme.Config{
    Preset: theme.Preset{
        Base:   theme.BaseStone,
        Accent: theme.AccentBlue,
        Style:  theme.StyleNova,
    },
    Chart:   theme.AccentRose,
    Radius:  theme.RadiusMedium,
    Font:    theme.FontSans,
    Heading: theme.FontMono,
}

// Apply globally, bind appearance, or scope a preview:
theme.Set(AppTheme.Light())
stop := theme.Bind(dark, AppTheme.Dark(), AppTheme.Light())
defer stop()
preview := theme.With(AppTheme.For(true), content)

// A formatted Go expression suitable for a code preview or clipboard:
source := AppTheme.GoCode(true)
// theme.Config{ ... }.For(true)
```

`Chart` selects chart colors independently of `Preset.Accent`. Leaving `Chart`
as `AccentBase` uses the selected base's chart palette even when the primary
accent is colored. `BaseColors`, `AccentColors`, `Styles`, `RadiusOptions` and
`Fonts` return fresh option slices for controls. Invalid names panic, as they do
for `Preset`.

| Radius choice | Main radius |
| --- | --- |
| `RadiusDefault` | Selected preset's geometry |
| `RadiusNone` | 0px |
| `RadiusSmall` | 4px |
| `RadiusMedium` | 8px |
| `RadiusLarge` | 16px |

Explicit radius choices set `RadiusSm` to 0.8 times the main radius and
`RadiusLg` to 1.4 times it, and scale the preset's chat corner tokens by the same
proportion. Padding and spacing retain the preset's values.

`FontDefault` uses the application's font. `FontSans` selects bundled Go Sans
with medium and bold faces, and `FontMono` selects bundled Go Mono with its bold
face. `FontInter` selects the official Inter 4.1 family, including regular,
medium, semibold and bold static faces. These fonts work offline and have the
same family across platforms. Inter is an explicit choice; it does not change
the zero configuration or global defaults. Its source, hashes and SIL Open Font
License are documented in [fonts/inter](../../fonts/inter/README.md).
`Heading` changes `Title` and `Heading`; its default inherits the body font.
Labels and captions inherit the body font, while code uses the independent
`Theme.Mono` style. Applications can load other fonts and customize the returned
`Theme` through its existing text styles.

`GoCode(dark)` exports the complete recipe and appearance as a deterministic Go
expression. It needs the `github.com/ironpark/ggui/ui/theme` import named `theme`.
It reproduces the configuration's theme, including its chart and font choices;
additional changes made to a returned `Theme` are outside the recipe. Building
or exporting a configuration has no global theme side effect. Applying it uses
the existing theme APIs, preserving accessibility preferences and layout caches.

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
[palette sources](#palette-sources) for upstream data and licenses.

## Palette sources

`shadcn.json` is the theme registry from:
https://github.com/shadcn-ui/ui/blob/main/apps/v4/registry/themes.ts
Retrieved 2026-09-18. It contains seven neutral base palettes and seventeen
accent palettes, with light/dark semantic tokens. Only TypeScript type syntax
was removed. Source license: MIT, included in [LICENSE-shadcn.txt](LICENSE-shadcn.txt).

The palette loader retains the original OKLCH values until conversion to sRGB.
Nova/Rhea geometry mappings are in [presets.go](presets.go), informed by:
https://github.com/shadcn-ui/ui/blob/main/apps/v4/registry/styles/style-nova.css
https://github.com/shadcn-ui/ui/blob/main/apps/v4/registry/styles/style-rhea.css

Default light/dark geometry now follows Base UI Nova: see the
[component defaults and migration notes](../../docs/shadcn-comparison.md).
