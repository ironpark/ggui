# Styling and themes

[Documentation](README.md) · [Project README](../README.md)

Customize local styles, inherited values, semantic colors, geometry, and motion.

Examples use `ggui`, `ui`, and `theme` (`github.com/ironpark/ggui/ui/theme`) imports and application-defined placeholders.
See the [migration guide](../ui/theme/README.md) for the package/API changes.
See [example conventions](README.md#start-here) before copying snippets.

Choose the scope of a change before setting a style:

| Layer | What it is | Set with | Read by |
| --- | --- | --- | --- |
| Local style | one widget's own font, size, color | `Text(s).Size(18).Color(c)` | that widget |
| Inherited style | what a subtree starts from | `Styled(child)`, `theme.With(t, child)`, `Provide(key, v, child)` | every widget below, at layout |
| Theme | the app's tokens: colors, spacing, named text styles | `theme.Set(t)`, `theme.Bind(sig, on, off)` | the tree at layout: controls, and colors given as tokens such as `theme.Primary` |

A widget's own setters win over what it inherited, and what it inherited wins
over the built-in defaults. Nothing is resolved at construction: the chain is
walked during layout, so a theme swap or a `Styled` wrapper reaches widgets
that were built long before it.

## On this page

- [Local styles](#local-styles)
- [Inherited styles](#inherited-styles)
- [Theme tokens](#theme-tokens)
- [Deriving a theme](#deriving-a-theme)
- [Porting a shadcn/ui theme](#porting-a-shadcnui-theme)
- [Tokens of your own](#tokens-of-your-own)
- [Switching themes](#switching-themes)
- [Styling a custom widget](#styling-a-custom-widget)
- [Accessibility preferences](#accessibility-preferences)
- [Theme presets](#theme-presets)
- [GPU shadows](#gpu-shadows)
- [Component appearance](#component-appearance)
- [Component variants](#component-variants)
- [Applying styles and a theme switch](#applying-styles-and-a-theme-switch)
- [Where it lives](#where-it-lives)

## Local styles

`TextStyle` is a plain value with five fields:

```go
type TextStyle struct {
	Font       *Font
	Size       float64     // pixels
	Color      color.Color
	LineHeight float64     // multiple of Size between baselines
	Weight     FontWeight  // drawn with the Font's face nearest it
}
```

**A zero field means "inherit", not "zero".** `a.Merge(b)` lays the set fields
of `b` over `a` and leaves the rest of `a` alone, which is what makes a style
composable:

```go
ggui.Text("Heading").Style(t.Title)              // the theme's heading
ggui.Text("Heading").Style(t.Title).Color(brand) // the same, in one other color
```

`ui.Title(s)`, `ui.Heading(s)`, `ui.Caption(s)` and `ui.Mono(s)` are
shorthands for the same merge against the theme's named styles, resolved from
the `Env` at layout, so a heading needs no `theme.Use`. Each has the forms
`ggui.Text` has: `ui.CaptionOf(reader)` follows a value and
`ui.Captionf("%d left", count)` formats one, as `TextOf` and `Textf` do. Note that the named
styles are *deltas*: `theme.Default().Title` is `{Size: 24, Weight:
WeightSemibold}` and nothing else, so a title inherits its font, color and
line height from `Theme.Text`. Change `Theme.Text.Font` and the headings
follow. See [Fonts](fonts.md#font-weights) for how a weight picks a face.

The full order a `Text` resolves in, every layout:

1. the inherited style from the `Env` (`env.Text()`)
2. merged with the theme's named style, for `Title`, `Heading`, `Caption` and `Mono`
3. merged with the widget's own setters
4. anything still unset filled from the built-in defaults — the default font,
   `DefaultTextSize` (14), black, line height 1.2. This step rarely does
   anything, because step 1 has already supplied `Theme.Text`; it is the
   floor under an `Env` built by hand, as a test does.
5. the size multiplied by `env.TextScale()`

## Inherited styles

Every widget lays out under an `Env` that flows down from the root, the way
CSS inherits font and color. `Styled(child)` sets what everything below starts
from, and a `Text`'s own setters still win:

```go
ggui.Styled(ggui.Column(
	ggui.Text("inherits 12px muted"),
	ggui.Text("but this one is 18").Size(18),
)).Color(t.MutedFg).Size(12)
```

Importing `ui/theme` (also imported by `ui`) installs the default UI environment.
`theme.Set` and `theme.Bind` supply `Theme.Text` to the root `Env`, so a bare
`ggui.Text(s)` inherits it. The core has no dependency on the theme package.

`theme.With(t, child)` gives a subtree a theme of its own — a preview pane showing
the dark theme inside a light window, say. `.Space(n)` on `Column`, `Row`,
`Wrap`, `Grid` and `EachKeyed` is n times `ggui.SpacingKey`, resolved at layout.
The theme maps its `Space` token to this key, so gaps track the theme without
a `theme.Use` call. Core-only applications can provide the spacing key directly.

Your own inherited values travel the same road. `NewEnvKey[T](name)` makes a key,
`Provide(key, v, child)` stores a value under it, and a widget reads it back
with `env.Get(key)` in `Layout`:

```go
var Density = ggui.NewEnvKey[float64]("density")

ggui.Provide(Density, 0.75, page)      // above
d, ok := env.Get(Density)              // inside a widget's Layout
```

Two properties are worth knowing. An `Env` is a **value**: adding to it never
changes the parent's, so there is no stack to unwind and no cleanup to forget.
And inheritance happens at **layout time**, not construction time, which is
what lets Go's eager evaluation work here at all — no closures wrapped around
subtrees, no builder callbacks, widgets resolve inherited values whenever layout is needed.

## Theme tokens

Defaults below describe `theme.Default()` and `theme.Dark()`. A `theme.Preset`
may supply different colors and geometry.

### Colors

The palette follows shadcn/ui's semantic tokens, so each one answers to a CSS
variable and a theme written for shadcn ports across as it is; see
[Porting a shadcn/ui theme](#porting-a-shadcnui-theme). Every surface comes
with the foreground drawn on it; use the pair together and text stays legible
under any theme.

Tokens marked *follows* take their value from another token unless set; see
[Deriving a theme](#deriving-a-theme).

### Coloring what you build

Give a widget a theme color by name, not by value. `theme.Primary`,
`theme.MutedFg` and the rest, one for every color field of `Theme`, are
`theme.Color` tokens: a widget resolves one against the `Env` it is laid out
in, so it follows a theme switch, or a subtree's own `theme.With`, without
anything being rebuilt.

```go
ggui.Box(ggui.Text("Z").Color(theme.PrimaryFg)).Fill(theme.Primary)
ggui.Text("Off").Color(theme.MutedFg.Alpha(.55))
ggui.Box().Fill(theme.Var("warning"))   // a variable of your own; see below
```

`Alpha` fades a token, and `theme.Fade` and `theme.Mix` keep a token a token
until it is drawn. Reading the theme's values with `theme.Use()` in a builder
takes a snapshot instead, which a theme switch leaves behind, and a
`ggui_debug` build reports it; `theme.Use()` is for a `Reactive` that builds
something other than colors from the theme. A custom widget that takes a
color resolves it with `ggui.ResolveColor(c, env)` in `Layout`; drawn without
an `Env`, a token is the color the application's theme has.

| Token | CSS variable | For | Light | Dark |
| --- | --- | --- | --- | --- |
| `Bg` / `Fg` | `--background` / `--foreground` | the window and the text on it | white / zinc-900 | zinc-950 / zinc-50 |
| `Card` / `CardFg` | `--card` / `--card-foreground` | raised inline surfaces and their text | follow `Bg` / `Fg` | zinc-900 / follows `Fg` |
| `Popover` / `PopoverFg` | `--popover` / `--popover-foreground` | floating surfaces: menus, dialogs, toasts | follow `Card` / `CardFg` | same |
| `Primary` / `PrimaryFg` | `--primary` / `--primary-foreground` | the main action | zinc-900 / zinc-50 | zinc-200 / zinc-900 |
| `PrimaryHover` | — | `Primary` under the pointer | follows `Primary`, at 90% over `Bg` | same |
| `Secondary` / `SecondaryFg` | `--secondary` / `--secondary-foreground` | a supporting action | zinc-100 / follows `Fg` | zinc-800 / follows `Fg` |
| `Muted` / `MutedFg` | `--muted` / `--muted-foreground` | the quiet surface of tracks and strips; the grey of secondary text | zinc-100 / zinc-500 | zinc-800 / zinc-400 |
| `Destructive` / `DestructiveFg` | `--destructive` / `--destructive-foreground` | an irreversible action | `#d32f2f` / zinc-50 | same |
| `Border` | `--border` | outlines of inputs, dividers, hairlines | zinc-200 | `#323236` |
| `Input` | — | the painted input surface | follows `Bg` | follows `InputBorder`, at 30% over `Bg` |
| `InputBorder` | `--input` | input outlines | follows `Border` | same |
| `Accent` / `AccentFg` | `--accent` / `--accent-foreground` | the highlighted row or button under the pointer, and its text | follow `Muted` / `Fg` | same |
| `Ring` | `--ring` | the focus halo, deliberately not `Primary` | zinc-400 | zinc-500 |
| `Selection` | — | selected text | follows `Primary`, at 25% over `Bg` | same |
| `Scrim` | — | dims the window behind a modal | 38% black | same |

The sidebar uses separate `Sidebar`/`SidebarFg`, `SidebarPrimary`/
`SidebarPrimaryFg`, `SidebarAccent`/`SidebarAccentFg`, `SidebarBorder`, and
`SidebarRing` tokens. `Sidebar` is near-white in light and zinc-900 in dark;
the rest follow the matching page tokens. Inside a `Sidebar`, its children
see these in place of the page's.

`Chart` contains five optional series colors. Default themes leave them unset,
so charts fall back to `Primary`; choose a `theme.Preset` or set `Chart` for a
multicolor palette. `Chat` controls bubble, attachment, and questionnaire
geometry; see [Theme presets](#theme-presets) and [Chat components](chat.md).

### Elevation

Three shadows, in the order a surface rises off the page. They are
`ShadowStyle{Offset, Blur, Spread, Color}` and cost no layout space.

| Token | For | Default |
| --- | --- | --- |
| `CardShadow` | cards, the raised tab, a resting button | `0 1px`, blur 2, 6% black |
| `PanelShadow` | menus, select lists, date pickers, toasts | `0 4px`, blur 10, 10% black |
| `OverlayShadow` | dialogs and sheets | `0 12px`, blur 28, 25% black |

### Size and space

| Token | For | Default |
| --- | --- | --- |
| `Radius` | buttons, inputs and boxes that ask for one | 8 |
| `RadiusSm` | rows and pills inside a rounded container | follows `Radius` × 0.75 |
| `RadiusLg` | cards, dialogs, toasts | follows `Radius` × 1.5 |
| `Space` | the unit every gap and padding is a multiple of | 8 |
| `BorderWidth` | the line `Box.Border` and the controls draw | 1 |
| `ControlSize` | a checkbox or radio glyph; a switch track is 4px taller, a slider knob this wide | 16 |
| `ControlGap` | between a glyph and its label | 8 |
| `IconSize` | chevrons, checks and other icons inside a control | 16 |
| `MenuWidth` | a dropdown panel, which does not stretch | 224 |

### Padding

Each is an `EdgeInsets`, built with the CSS shorthand `Insets` takes.

| Token | Inside | Default |
| --- | --- | --- |
| `ButtonPad` | a button | `8 16` |
| `FieldPad` | a text field, select or other input | `8 12` |
| `ItemPad` | one row of a list or menu | `6 8` |
| `CardPad` | a card, dialog or sheet | `24` |
| `PanelPad` | a popup panel, around its items | `4` |
| `TabPad` | one tab label | `5 10` |

### Motion and state

| Token | For | Default |
| --- | --- | --- |
| `MotionFast` | knobs, tab indicators, collapsing content | 150ms |
| `MotionSlow` | entrances and exits | 240ms |
| `HoverMix` | how far a color moves toward `Fg` under the pointer | 0.06 |
| `PressMix` | how far it moves toward `Fg` while pressed | 0.08 |
| `DisabledMix` | how much opacity a disabled control loses | 0.5 |
| `RingAlpha` | the opacity of a field's focus halo and of a warning ring | 0.5 |

Every control draws its states with these, through `t.Hovered(c)`,
`t.Pressed(c)`, `t.Disabled(c)` and `t.Halo(ring)`, so a control of your own
can match the built-in ones rather than inventing its own greys.

### Text

`Text` is the base every piece of text inherits; the others are merged onto
it. See [Local styles](#local-styles) for how the merge resolves.

| Token | For | Default |
| --- | --- | --- |
| `Text` | everything; its `Color` follows `Fg` | 14px, line height 1.4 |
| `Title` | page headings, `ui.Title` | 24px semibold |
| `Heading` | dialog, sheet and card titles, `ui.Heading` | 18px semibold |
| `Label` | the labels of buttons, tabs, badges and accordion headers | medium weight |
| `Caption` | small secondary text, `ui.Caption`; its `Color` follows `MutedFg` | 12px |
| `Mono` | code, `ui.Mono` | the platform's monospaced font, `ggui.DefaultMonoFont()` |

## Deriving a theme

Start from `theme.Default()`, `theme.Dark()`, or a `theme.Preset`, and set
only what your brand changes:

```go
func brandTheme() theme.Theme {
	t := theme.Default()
	t.Primary = color.RGBA{0x2f, 0x6f, 0xed, 0xff}
	t.PrimaryFg = color.White
	t.Ring = t.Primary
	t.Radius = 4
	t.Text.Font = inter
	return t
}
```

Some tokens **follow** others, as the tables mark: `PrimaryHover` and
`Selection` follow `Primary`, the foregrounds follow `Fg`, `RadiusSm` and
`RadiusLg` follow `Radius`, and so on. A token follows its source until you
set it, so above the hover color, the text selection and the smaller and
larger radii all move with the brand without being named. Setting a follower
pins it: `t.PrimaryHover = darker` keeps `darker` whatever `Primary` becomes.
A preset sets its whole palette and its radii, so on a preset only the tokens
shadcn has no variable for, such as `PrimaryHover`, `Selection` and `Input`,
follow.

`Apply`, `Set` and `With` resolve the followers as they apply a theme, and
`theme.From` and `theme.Use` return resolved themes. Call `t.Resolve()`
yourself only to read a follower of a theme you changed before applying it.

A `Theme` literal works too: colors it leaves unset with nothing to follow
are `Default`'s, as are a zero `ControlSize`, `IconSize`, `MenuWidth`, `Chat`
or `Mono` font. Other zero sizes, such as `BorderWidth`, mean zero.

Changing `Text.Font` alone reaches every piece of text in the app, headings
and captions included, because those are deltas merged onto it.

## Porting a shadcn/ui theme

`theme.FromCSS` reads the CSS that shadcn/ui's theme editor, tweakcn and
similar generators export, as it is:

```go
//go:embed theme.css
var themeCSS string

light, dark, err := theme.FromCSS(themeCSS)
if err != nil {
	log.Fatal(err)
}
app.Setup(func() { theme.Bind(isDark, dark, light) })
```

It lays the `:root` block over `Default` and the `.dark` block over `Dark`.
As in a browser, `.dark` inherits whatever it leaves out from `:root`. Blocks
may sit inside `@layer`, other rules such as `@theme inline` are skipped, and
a value may be `var()` of another variable. Each color variable sets the
token the color table names, with `--input` setting `InputBorder`. `--radius`
sets the radii the way shadcn derives its own: `Radius` 2px less for the
`rounded-md` controls, `RadiusSm` 4px less and `RadiusLg` 4px more.
A color variable with no token of its own, such as the `--warning` or
`--success` shadcn leaves to the app, is kept as a token of your own (below)
under `theme.CSSColor(name)`:

```go
ggui.Text("Low disk space").Color(theme.Var("warning"))
warning, ok := theme.From(env).Get(theme.CSSColor("warning")) // in a widget's Layout
```

Other variables, such as fonts, are ignored.

`t.ApplyCSS(decls)` lays one block's declarations over a theme you already
have, and `theme.ParseColor(s)` reads a single value. Both accept hex,
`rgb()`, `hsl()` and `oklch()` in either syntax, and shadcn v3's bare HSL
channels such as `222.2 84% 4.9%`.

```go
t, err := theme.Default().ApplyCSS(`--primary: oklch(0.546 0.245 262.881); --radius: 0.5rem;`)
```

shadcn derives hover states in CSS, and ggui has tokens for them that follow
`Primary` the same way, so a ported theme needs nothing more.

## Tokens of your own

A theme carries tokens the framework never looks at, so a control set or a
brand can travel with it rather than beside it. `Set` returns a copy, so a
theme can be derived from another without disturbing it:

```go
var BrandGradient = ggui.NewEnvKey[color.Color]("brand gradient")

t = t.Set(BrandGradient, gradient)
if c, ok := t.Get(BrandGradient); ok {
	// ...
}
```

Prefer a theme token over `Provide` when the value belongs to the *look* and
should change when the theme does; prefer `Provide` when it belongs to one
subtree, like a form's disabled state.

## Switching themes

| Call | Does |
| --- | --- |
| `theme.Use()` | Read the global theme's values; inside a tracked computation such as `Reactive`, subscribe it to changes. To color a widget, use a token such as `theme.Primary` instead. |
| `theme.Set(t)` | Replace the global theme, invalidate inherited layout, and notify tracked readers. |
| `theme.Bind(sig, on, off)` | follows a `Readable[bool]`, swapping between two themes |
| `theme.With(t, child)` | gives one subtree a theme without touching the app's |
| `theme.Override(fn, child)` | changes some tokens of the inherited theme for a subtree, as a CSS variable set on an element does |
| `theme.From(env)` | the theme at layout time, for a widget |

The usual dark-mode switch is three lines:

```go
dark := ggui.State(false)
app.Setup(func() { theme.Bind(dark, theme.Dark(), theme.Default()) })
// somewhere in the tree:
ui.Switch(dark, "Dark mode")
```

Register it in `App.Setup` so the watcher has an owner and is disposed with
the app. A window with no `Config.Background` paints the theme's `Bg`, so the
window follows along.

## Styling a custom widget

Read the theme from the `Env` in `Layout`, not with `theme.Use`:

```go
func (w *MyWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := theme.From(env)
	w.theme = t                    // Paint needs it too
	w.pad = t.ItemPad
	return c.Constrain(...)
}
```

`theme.Use` reads the theme from the reactive root environment; it does not create an effect.
Use `theme.From(env)` in a widget so local `theme.With` overrides are respected.
Use `theme.Use()` inside `Reactive` when constructing a subtree from the global
theme's values; for colors, a token does it with no rebuild. Reads in app root
setup or `Component` setup do not make setup rerun.

Keep what `Paint` needs on the struct, since `Paint` gets no `Env`. That is
the pattern every control in the `ui` package follows, and it is why a theme
change costs a relayout rather than a rebuild.

Two things worth matching when you draw:

- Use the token pairs. A surface and its foreground go together, so the
  control stays legible when someone swaps the palette.
- Draw states with `t.Hovered(c)`, `t.Pressed(c)`, `t.Disabled(c)` and
  `t.Halo(ring)` rather than picking greys, so your control ages with the
  theme. `theme.Mix(a, b, amount)` and `theme.Fade(c, alpha)` are the color
  arithmetic underneath.

`theme.Override` is the quick way to restyle part of a page:

```go
theme.Override(func(t *theme.Theme) {
	t.Primary = danger // PrimaryHover and Selection follow
	t.Radius = 0
}, dangerZone)
```

## Accessibility preferences

Two `Env` keys carry what the user asked for. Provide them above the tree — or
above a subtree, to preview them.

```go
ggui.Provide(ggui.TextScaleKey, 1.5, tree)     // every Text and TextInput grows
ggui.Provide(ggui.ReducedMotionKey, true, tree) // transitions land at once
```

`TextScaleKey` multiplies the resolved size of every `Text` and `TextInput`;
read it with `env.TextScale()`, which is 1 by default.

`ReducedMotionKey` is read through `env.Motion(d)`, which returns `d`
normally and zero when motion is reduced. A widget that animates should ask
for its duration that way rather than reading the theme directly:

```go
w.motion = env.Motion(t.MotionFast)   // zero under reduced motion
```

Transitions then land at once and eased motions jump, without every widget
having to know the preference exists.

## Theme presets

`theme.Preset` combines the [shadcn/ui semantic color tokens](https://ui.shadcn.com/docs/theming)
with ggui's geometry tokens. Palette data is vendored locally; constructing a
preset never needs a network connection.

```go
preset := theme.Preset{
    Base: theme.BaseNeutral,
    Accent: theme.AccentBlue,
    Style: theme.StyleRhea,
}
app.Setup(func() { theme.Bind(dark, preset.Dark(), preset.Light()) })

// Returned Themes are independent values; customize after selecting a preset.
// For(dark) picks Dark or Light.
t := preset.For(dark)
t.Chat.BubbleRadius = 20
t.Chat.BubblePadding = ggui.Insets(10, 14)
theme.Set(t)
```

The zero preset uses Neutral/Nova with no accent override. `theme.Default()` and
`theme.Dark()` preserve the existing ggui appearance.

| Axis | Presets |
| --- | --- |
| `BaseColor` | Neutral, Stone, Zinc, Mauve, Olive, Mist, Taupe |
| `AccentColor` | Base (no override), Amber, Blue, Cyan, Emerald, Fuchsia, Green, Indigo, Lime, Orange, Pink, Purple, Red, Rose, Sky, Teal, Violet, Yellow |
| `theme.Style` | Nova, Rhea |

`theme.BaseColors()`, `theme.AccentColors()` and `theme.Styles()` return independent slices
for pickers. Unknown enum names panic. An accent changes primary, secondary,
chart and sidebar-primary tokens while preserving the selected base surfaces.
Nova uses tighter corners and spacing; Rhea uses rounder controls, 24px bubble
corners and 16px attachment corners. These are native token mappings for ggui's
components, not CSS execution. `Theme.Chat` controls bubble padding/radius,
attachment radii and questionnaire geometry. Fonts remain caller-configurable
through `Theme.Text.Font` and `Theme.Title.Font`.

The semantic additions are `CardFg`, `PopoverFg`, `Accent`/`AccentFg`,
`InputBorder`, `Chart[5]` and `Sidebar`/`SidebarFg`, `SidebarPrimary`/
`SidebarPrimaryFg`, `SidebarAccent`/`SidebarAccentFg`, `SidebarBorder` and
`SidebarRing`. They preserve the corresponding shadcn roles, including alpha
on dark borders. CSS `--input` maps to `InputBorder`; ggui's existing `Input`
continues to mean the painted input surface. Color values are converted from
OKLCH to sRGB, clipping out-of-gamut channels at the rendering boundary.

The gallery's **Theme presets** preview switches base, accent and style for the
whole gallery, and the dark-mode switch preserves all three choices. **Emoji**
demonstrates color glyphs in labels and editable text. Gallery fonts and emoji
assets are embedded, so the same previews run offline in native and WASM builds.

## GPU shadows

`Box.Shadow(styles...)` and `ui.Card(...).Shadow(styles...)` paint outer shadows
before the surface. Multiple styles form layers; calling `.Shadow()` clears them.
For custom drawing, use `canvas.Shadow(rect, cornerRadius, style)`.

```go
ui.Card(content).Shadow(ggui.ShadowStyle{
    Offset: ggui.Pt(0, 6),
    Blur:   12,
    Spread: 0,
    Color:  color.NRGBA{A: 50},
})
```

All distances are logical pixels and scale with the display. Positive spread
expands the silhouette; negative spread contracts it. Nil or transparent colors
skip rendering. Blur is a smooth feather distance on both sides of the edge;
zero gives a sharp, antialiased shadow. This is a rounded-rectangle distance-field
approximation, not a Gaussian blur of the content or image alpha.

The renderer lazily shares one WGSL shader. Each visible shadow layer
is one quad, with no intermediate textures, blur passes or CPU
rasterization, and shadows drawn one after another share a draw call. Draw bounds are intersected with the target before rendering.
Cost still grows with visible pixel area and overlapping layers.

Shadows do not reserve layout space or create hit regions. Add padding/gaps when
needed; parent clipping and window bounds still clip them. The gallery's Shadows
preview compares subtle, floating and colored treatments. Toast uses this same
renderer, including its existing fade animation.

## Component appearance

The control set follows the visual hierarchy of [shadcn/ui's semantic theme
colors](https://ui.shadcn.com/docs/theming), [segmented tabs](https://ui.shadcn.com/docs/components/tabs)
and [cards](https://ui.shadcn.com/docs/components/card), adapted to native drawing
and ggui's existing APIs.

| Element | Appearance and configuration |
| --- | --- |
| Buttons | Primary by default; `Outline()` draws a border around the background. `Secondary()` adds a subdued fill, `Ghost()` removes the resting surface, `Destructive()` fills it with `Destructive` under a `DestructiveFg` label, and `Link()` underlines a `Primary` label under the pointer. Outline and ghost buttons turn `Accent` with an `AccentFg` label under the pointer. `Size(ui.ButtonSmall)`, `ui.ButtonLarge` and `ui.ButtonIcon` scale `ButtonPad`; `Radius(r)` and `Pad(...)` override the theme for one button. Labels use the `Label` text style. |
| Focus | Controls use a separate, softer focus color. Text fields add an outer halo while editing. |
| Tabs | A muted rounded strip with an animated raised selection; `.Line()` opts into the underline treatment. Reduced-motion settings still apply. |
| Cards and floating panels | Cards receive a subtle shadow; menus, select lists, comboboxes and date pickers use a stronger shared panel shadow. Dialogs use a larger radius and deeper elevation. |
| Labels and notices | Field labels use the body size; help text remains smaller. Alerts use body-size descriptions and tighter title spacing. |
| Badges and calendar | Badges are pills on the `Secondary` surface; `Primary()`, `Outline()` and `Destructive()` change the look and `Radius(r)` the corners. Calendar month navigation uses ghost buttons, with a centered month heading and contrasting selected-date text. |
| Menus, selects and dialogs | A highlighted item turns `Accent` with an `AccentFg` label. Floating surfaces draw their text in `PopoverFg`; dialog and sheet titles use the `Heading` style. |

These choices follow a brand by changing the theme rather than the widgets.
The controls read ordinary `Theme` fields, and a test checks that every one
of them is read by some control. Start with a default or preset and override
just the fields you need. `Card(...)` also takes `Fill`, `Border`, `Radius`,
`Pad` and `Shadow` for one card.

```go
t := theme.Default()
t.Ring = color.NRGBA{R: 140, G: 165, B: 230, A: 255}
t.PanelShadow = ggui.ShadowStyle{Offset: ggui.Pt(0, 4), Blur: 12, Color: color.NRGBA{A: 45}}
// Disable default card elevation globally, or call Card(...).Shadow() locally.
t.CardShadow = ggui.ShadowStyle{}
theme.Set(t)
```

## Component variants

A button or badge variant is a `ui.Variant`: a named look worked out from the
theme at layout. The built-in ones are package variables, such as
`ui.ButtonOutline`, `ui.ButtonSmall` and `ui.BadgeDestructive`. A variant can
be changed for everything drawn with it under a theme, the way editing the
`cva` call in a copied shadcn component does:

```go
t := theme.Default()
t = ui.ButtonPrimary.Restyle(t, func(t theme.Theme) ui.ButtonStyle {
	s := ui.ButtonPrimary.Base(t) // the built-in look, to change
	s.Shadow = false
	return s
})
theme.Set(t)
```

`Base` is the variant's own look under a theme, whatever `Restyle` did; a
restyle that starts from it changes the original rather than replacing it.
The restyle travels with the theme, so `theme.With` and `theme.Bind` carry it
too.

`ui.NewVariant` makes a variant of your own, used with `.Variant(v)`:

```go
var ButtonBrand = ui.NewVariant("brand", func(t theme.Theme) ui.ButtonStyle {
	return ui.ButtonStyle{Fill: brand, Hover: t.Hovered(brand), Label: color.White, Shadow: true}
})

ui.Button("Upgrade", upgrade).Variant(ButtonBrand)
```

| Type | Built-in variants | Fields |
| --- | --- | --- |
| `ui.ButtonVariant` | `ButtonPrimary`, `ButtonOutline`, `ButtonSecondary`, `ButtonGhost`, `ButtonDestructive`, `ButtonLink` | `Fill`, `Hover`, `Label`, `HoverLabel`, `Border`, `Ring`, `Shadow`, `Underline` |
| `ui.ButtonSizeVariant` | `ButtonDefault`, `ButtonSmall`, `ButtonLarge`, `ButtonIcon` | `Pad`, `Square` |
| `ui.BadgeVariant` | `BadgeSecondary`, `BadgePrimary`, `BadgeOutline`, `BadgeDestructive` | `Fill`, `Label`, `Border` |

## Applying styles and a theme switch

Styling has three layers: a widget's own setters, values inherited through the
`Env`, and the theme's tokens. A widget's setters win over what it inherited,
and inheritance is resolved at layout time, so a theme swap reaches widgets
built long before it.

```go
ggui.Text("Heading").Style(t.Title).Color(brand)        // local
ggui.Styled(page).Color(theme.MutedFg).Size(12)         // inherited
app.Setup(func() { theme.Bind(dark, theme.Dark(), theme.Default()) })
```

Use `ui.ThemeSwitch(dark)` for a compact day/night control: the large sun thumb turns into
a softly shaded full moon, with clouds fading into stars. `true` means dark
mode. Bind the same signal with `theme.Bind` as above to apply the theme. The
control supports `.Name("Appearance")`, `.OnChange(fn)`, `.Disabled(v)`, and
`.BindDisabled(reader)`, plus Space/Enter and reduced-motion preferences.

## Where it lives

| Concern | File |
| --- | --- |
| `TextStyle`, `Env`, typed environment values | [style.go](../style.go) |
| Root environment, editor/scrollbar styles, spacing and background | [environment.go](../environment.go) |
| Theme tokens, followers, defaults, presets and application | [ui/theme/](../ui/theme/) |
| Reading shadcn/ui CSS | [ui/theme/css.go](../ui/theme/css.go) |
| Color tokens, `EnvColor` | [ui/theme/tokens.go](../ui/theme/tokens.go), [envcolor.go](../envcolor.go) |
| `Variant`, `Restyle` | [ui/variant.go](../ui/variant.go) |
| `Text`, `Styled`, `Provide`, `WithEnv` | [widgets.go](../widgets.go) |
| `ui.Title`, `ui.Heading`, `ui.Caption`, `ui.Mono` | [ui/text.go](../ui/text.go) |
| Shadows | [shadow.go](../shadow.go) |
| Motion and transitions | [anim.go](../anim.go), [transition.go](../transition.go) |
| The controls that consume the tokens | [ui/](../ui/) |

See [Documentation](README.md) for the rest of the framework, and
[Animation](animation.md#animation) for the values that move over time.
