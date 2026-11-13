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
root environment. `Default` and `Dark` are shadcn/ui's stock base-nova themes:
they have the tokens of `Preset{}.Light()` and `Preset{}.Dark()`, and differ
from them only in that a color equal to the one it would follow keeps following
it, so changing `Primary` or `Fg` on a copy moves what derives from them. `theme.Set(t)` replaces the global theme while preserving
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

### Light, dark and system

`theme.Mode` is `ModeLight`, `ModeDark` or `ModeSystem`, the three choices of
shadcn/ui's mode toggle; the zero Mode means system too. `theme.BindMode(mode,
dark, light)` applies the theme the mode names and follows the desktop through
`ggui.SystemDark()` while it is system. `ui.ThemeMode(mode)` is the segmented
control for it, beside the two-state `ui.ThemeSwitch`.

```go
mode := ggui.State(theme.ModeSystem)
app.Setup(func() { theme.BindMode(mode, recipe.Dark(), recipe.Light()) })
ui.ThemeMode(mode)
```

### Radius scale

`Radius` is shadcn/ui's `--radius`, and the other radii are its scale of it.
ggui names each step one smaller than Tailwind does. A radius left at zero
follows `Radius`, and so does one still holding the value it was given that way,
so setting `Radius` on a copy, through `--radius` in `ApplyCSS` or in a
`theme.Override` restyles every corner, the chat corners among them.

| Token | Tailwind | Of `Radius` in Nova | Used by |
| --- | --- | --- | --- |
| `RadiusXs` | `rounded-sm` | 0.6 | chips, keys and the smallest insets |
| `RadiusSm` | `rounded-md` | 0.8 | menu rows, tab triggers |
| `Radius` | `rounded-lg` | 1 | buttons, fields, menus |
| `RadiusLg` | `rounded-xl` | 1.4 | cards, dialogs |
| `RadiusXl` | `rounded-2xl` | 1.8 | toasts |
| `Chat.BubbleRadius`, `Chat.AttachmentRadius` | `rounded-xl` | 1.4 | bubbles, attachments |
| `Chat.AttachmentXSRadius`, `Chat.QuestionRadius`, `Chat.QuestionInputRadius` | `rounded-lg` | 1 | small attachments, question choices and input |

### Input surface

`Input` is the surface a field paints, and it is translucent, as shadcn/ui's
`bg-transparent dark:bg-input/30` is: `InputBorder` at the opacity of the
theme's Style, so a field takes on the card, sidebar or dialog it sits on. Nova,
Vega and Lyra are clear in a light theme and 30% in a dark one; Maia is 30% in
both, Mira 20% and 30%, Luma and Rhea 50% in both. A control paints it over its
surface, never instead of it. Set `Input` to pin an opaque surface.

### Motion and elevation

| Token | Default | Used for |
| --- | --- | --- |
| `MotionFast` | 150ms | hover and control transitions |
| `MotionSlow` | 200ms | disclosures and sheets |
| `MotionOverlay` | 100ms | dialog and popup entrances and exits |
| `MotionDrawer` | 450ms | a drawer sliding in or out |
| `CardShadow` | `shadow-xs`: 0 1px 2px, 6% black | cards, the raised tab |
| `PanelShadow` / `PanelContact` | `shadow-md`: 0 4px 6px -1px and 0 2px 4px -2px, 10% black | menus, select and combobox lists, popovers, hover cards, date pickers |
| `OverlayShadow` / `OverlayContact` | `shadow-lg`: 0 10px 15px -3px and 0 4px 6px -4px, 10% black | sheets, toasts, submenus |

### Styles

A `Style` maps one of shadcn/ui's style sheets onto the geometry tokens: which
step of the radius scale each kind of corner takes, the padding of buttons,
fields, menu rows, cards, panels and tabs, the text size, and the chat geometry.

| Style | Corners (`RadiusSm` / `Radius` / `RadiusLg`) | Controls | Text |
| --- | --- | --- | --- |
| `StyleNova` | 8 / 10 / 14 | 32px high, 10px sides | 14px |
| `StyleVega` | 6 / 8 / 14 | 36px high, roomier rows, cards and dialogs | 14px |
| `StyleMaia` | 14 / 18 / 18 | 36px high pills, 12px sides, filled fields | 14px |
| `StyleLyra` | square, whatever `--radius` is | 32px high | 12px |
| `StyleMira` | 8 / 8 / 10 | 28px high, 8px sides | 12px, relaxed |
| `StyleLuma` | 18 / 22 / 26 | 36px high pills, 12px sides | 14px |
| `StyleRhea` | 14 / 18 / 24 (capped) | 32px high, 12px sides | 14px |

The steps are the ones most components take in each sheet; a component whose
sheet gives it a corner of its own, such as Luma's fully round tab list, keeps
the token's. Where a sheet gives cards and dialogs, or buttons and menus,
different steps that one token holds, the token takes the step of the more
common part: Maia's dialogs (`rounded-4xl`) and Mira's dialogs (`rounded-xl`)
take their cards' `RadiusLg`, and Mira's menus (`rounded-lg`) its buttons'
`Radius`. Sera is not offered: it sets labels in tracked capitals and draws
fields as underlines, which no token reaches.

Core-only applications need no UI package. They can use `ggui.SetEnv`,
`ggui.UseEnv`, `ggui.Provide` and `ggui.WithEnv` to configure the same primitives.

## Reusable configurations

`Config` is a small, comparable recipe for an application theme or an interactive
builder, with the choices of shadcn/ui's own builder. `Config{}` produces exactly
`Preset{}`: Neutral/Nova, the base palette's own accent and chart colors, default
corners, subtle menus and the application's default font and icons. It leaves
`Default`, `Dark`, and `Preset` unchanged.

```go
var AppTheme = theme.Config{
    Preset: theme.Preset{
        Base:   theme.BaseStone,
        Accent: theme.AccentBlue,
        Style:  theme.StyleNova,
    },
    Chart:      theme.AccentRose,
    Radius:     theme.RadiusLarge,
    Font:       theme.FontGeist,
    Heading:    theme.FontGeistMono,
    MenuAccent: theme.MenuAccentBold,
    MenuColor:  theme.MenuColorInverted,
    Icons:      theme.IconsLucide, // registered by linking ui/icons/lucide
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

`Chart` follows `Preset.Accent` while it is `AccentBase`, as shadcn/ui's chart
color follows its theme. `ChartBase` keeps the base palette's own chart colors
under a colored accent, and an accent selects that accent's; either changes only
the five chart tokens. `BaseColors`, `AccentColors`, `ChartColors`, `Styles`,
`RadiusOptions`, `Fonts`, `MenuAccents`, `MenuColors` and `IconLibraries` return
fresh option slices for controls. Invalid names panic, as they do for `Preset`.

| Radius choice | `--radius` |
| --- | --- |
| `RadiusDefault` | 10px (0.625rem), shadcn/ui's stock radius |
| `RadiusNone` | 0px |
| `RadiusSmall` | 7.2px (0.45rem) |
| `RadiusMedium` | 10px (0.625rem) |
| `RadiusLarge` | 14px (0.875rem) |

These are the values of shadcn/ui's builder. Every corner is the selected
style's step of the chosen `--radius`, the chat corners included, as the
[radius scale](#radius-scale) describes; under Nova a large radius is a `Radius`
of 14, a `RadiusSm` of 11.2 and a `RadiusLg` of 19.6. Lyra is square at every
choice. Padding and spacing retain the preset's values.

| Menu choice | Effect |
| --- | --- |
| `MenuAccentSubtle` | The palette's own quiet `Accent` |
| `MenuAccentBold` | `Accent` and `AccentFg` become `Primary` and `PrimaryFg`; the sidebar's accent is unchanged |
| `MenuColorDefault` | Menus draw in the theme they open under |
| `MenuColorInverted` | `Theme.Menu()` of a light theme is the recipe's dark theme |

A menu draws under `theme.Of(env).Menu()`, which is the theme itself unless the
recipe inverts its menus. shadcn/ui's translucent menus blur what is behind
them, which a ggui popup does not, so they are not offered.

`Icons` names an icon library for the recipe's themes to carry. `ui/theme`
cannot import the icon packages, which import it, so a library is a choice once
it is registered. The bundled sets register themselves when linked; another set
registers with `theme.RegisterIcons`:

```go
import _ "github.com/ironpark/ggui/ui/icons/tabler"

recipe := theme.Config{Icons: theme.IconsTabler}
```

A recipe naming a library that is not registered panics, like any unknown name.

`FontDefault` uses the application's font. `FontSans` selects bundled Go Sans
with medium and bold faces, and `FontMono` selects bundled Go Mono with its bold
face. `FontInter` selects the official Inter 4.1 family, and `FontGeist` and
`FontGeistMono` the official Geist 1.9.2 families, the fonts of shadcn/ui's Nova
preset, each including regular, medium, semibold and bold static faces. These
fonts work offline and have the same family across platforms. Each is an
explicit choice; none changes the zero configuration or global defaults. Their
sources, hashes and SIL Open Font Licenses are documented in
[fonts/inter](../../fonts/inter/README.md) and
[fonts/geist](../../fonts/geist/README.md).
`Heading` changes `Title` and `Heading`; its default inherits the body font.
Labels and captions inherit the body font, while code uses the independent
`Theme.Mono` style. Applications can load other fonts and customize the returned
`Theme` through its existing text styles.

`ApplyCSS` and `FromCSS` read `--font-sans` into `Text`, `--font-heading` into
`Title` and `Heading`, and `--font-mono` into `Mono`, taking the first family in
the list that is registered. Inter, Geist, Geist Mono, Go and Go Mono are;
`theme.RegisterFont("Figtree", figtree.Font)` adds another. A list naming only
system fonts leaves the theme's font as it is.

`GoCode(dark)` exports the complete recipe and appearance as a deterministic Go
expression. It needs the `github.com/ironpark/ggui/ui/theme` import named `theme`.
It reproduces the configuration's theme, including its chart, font, menu and
icon choices, the last needing its library registered where the code runs;
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
The style geometry mappings are in [presets.go](presets.go), read on 2026-11-13
from the utilities each style sheet applies, `style-nova.css`, `style-vega.css`,
`style-maia.css`, `style-lyra.css`, `style-mira.css`, `style-luma.css` and
`style-rhea.css`, in:
https://github.com/shadcn-ui/ui/tree/main/apps/v4/registry/styles
The radius choices are `RADII` in:
https://github.com/shadcn-ui/ui/blob/main/apps/v4/registry/config.ts

Default light/dark geometry now follows Base UI Nova: see the
[component defaults and migration notes](../../docs/shadcn-comparison.md).
