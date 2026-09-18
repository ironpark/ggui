# Styling

How a ggui app decides what things look like. Three layers do the work, and
they are read in one order:

| Layer | What it is | Set with | Read by |
| --- | --- | --- | --- |
| Local style | one widget's own font, size, color | `Text(s).Size(18).Color(c)` | that widget |
| Inherited style | what a subtree starts from | `Styled(child)`, `Themed(t, child)`, `Provide(key, v, child)` | every widget below, at layout |
| Theme | the app's tokens: colors, spacing, named text styles | `SetTheme(t)`, `BindTheme(sig, on, off)` | the whole tree, and `UseTheme()` in a Builder |

A widget's own setters win over what it inherited, and what it inherited wins
over the built-in defaults. Nothing is resolved at construction: the chain is
walked during layout, so a theme swap or a `Styled` wrapper reaches widgets
that were built long before it.

- [Local styles](#local-styles)
- [Inherited styles](#inherited-styles)
- [Theme tokens](#theme-tokens)
- [Theme presets](#theme-presets)
- [Deriving a theme](#deriving-a-theme)
- [Porting a shadcn/ui palette](#porting-a-shadcnui-palette)
- [Tokens of your own](#tokens-of-your-own)
- [Switching themes](#switching-themes)
- [Styling a custom widget](#styling-a-custom-widget)
- [Accessibility preferences](#accessibility-preferences)

## Local styles

`TextStyle` is a plain value with four fields:

```go
type TextStyle struct {
	Font       *Font
	Size       float64     // pixels
	Color      color.Color
	LineHeight float64     // multiple of Size between baselines
}
```

**A zero field means "inherit", not "zero".** `a.Merge(b)` lays the set fields
of `b` over `a` and leaves the rest of `a` alone, which is what makes a style
composable:

```go
ggui.Text("Heading").Style(t.Title)              // the theme's heading
ggui.Text("Heading").Style(t.Title).Color(brand) // the same, in one other color
```

`Title(s)` and `Caption(s)` are shorthands for the same merge against the
theme's named styles, resolved from the `Env` at layout, so a heading needs no
`UseTheme`. Note that the named styles are *deltas*: `DefaultTheme().Title` is
`{Size: 24}` and nothing else, so a title inherits its font, color and line
height from `Theme.Text`. Change `Theme.Text.Color` and the headings follow.

The full order a `Text` resolves in, every layout:

1. the inherited style from the `Env` (`env.Text()`)
2. merged with `Theme.Title` or `Theme.Caption`, for `Title` and `Caption`
3. merged with the widget's own setters
4. anything still unset filled from the built-in defaults — Go Regular,
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

The root `Env` starts from `Theme.Text`, so a bare `Text(s)` already looks
right without anything wrapping it.

`Themed(t, child)` gives a subtree a theme of its own — a preview pane showing
the dark theme inside a light window, say. `.Space(n)` on `Column`, `Row`,
`Wrap`, `Grid` and `For` is n times the theme's `Space`, resolved at layout,
so gaps track the theme without a `UseTheme` either.

Your own inherited values travel the same road. `NewKey[T](name)` makes a key,
`Provide(key, v, child)` stores a value under it, and a widget reads it back
with `env.Get(key)` in `Layout`:

```go
var Density = ggui.NewKey[float64]("density")

ggui.Provide(Density, 0.75, page)      // above
d, ok := env.Get(Density)              // inside a widget's Layout
```

Two properties are worth knowing. An `Env` is a **value**: adding to it never
changes the parent's, so there is no stack to unwind and no cleanup to forget.
And inheritance happens at **layout time**, not construction time, which is
what lets Go's eager evaluation work here at all — no closures wrapped around
subtrees, no builder callbacks, just a tree that is walked once a frame.

## Theme tokens

### Colors

The palette follows shadcn/ui's semantic tokens, so each one answers to a CSS
variable and a palette written for shadcn ports across a line at a time. Every
surface comes with the foreground drawn on it; use the pair together and text
stays legible under any theme.

| Token | CSS variable | For | Light | Dark |
| --- | --- | --- | --- | --- |
| `Bg` / `Fg` | `--background` / `--foreground` | the window and the text on it | white / zinc-900 | zinc-950 / zinc-50 |
| `Card` | `--card` | cards and other raised inline surfaces | white | zinc-900 |
| `Popover` | `--popover` | menus, dialogs, sheets, toasts | white | zinc-900 |
| `Primary` / `PrimaryFg` | `--primary` / `--primary-foreground` | the main action | zinc-900 / zinc-50 | zinc-200 / zinc-900 |
| `PrimaryHover` | — | `Primary` under the pointer | zinc-700 | zinc-300 |
| `Secondary` / `SecondaryFg` | `--secondary` / `--secondary-foreground` | a supporting action | zinc-100 / zinc-900 | zinc-800 / zinc-50 |
| `Muted` / `MutedFg` | `--muted` / `--muted-foreground` | the quiet surface behind a hover; the grey of secondary text | zinc-100 / zinc-500 | zinc-800 / zinc-400 |
| `Destructive` / `DestructiveFg` | `--destructive` / `--destructive-foreground` | an irreversible action | `#d32f2f` / zinc-50 | same |
| `Border` | `--border` | outlines of inputs, dividers, hairlines | zinc-200 | `#323236` |
| `Input` | `--input` | the surface a text field or select paints | white | `#202023` |
| `Ring` | `--ring` | the focus halo, deliberately not `Primary` | zinc-400 | zinc-500 |
| `Selection` | — | selected text | zinc-300 | zinc-700 |
| `Scrim` | — | dims the window behind a modal | 38% black | same |

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
| `Radius` | boxes that ask for one | 8 |
| `RadiusSm` | rows and pills inside a rounded container | 6 |
| `RadiusLg` | cards, dialogs, toasts | 12 |
| `Space` | the unit every gap and padding is a multiple of | 8 |
| `BorderWidth` | the line `Box.Border` and the controls draw | 1 |
| `ControlSize` | a checkbox or radio glyph | 16 |
| `ControlGap` | between a glyph and its label | 8 |
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
| `HoverMix` | how far a color moves under the pointer | 0.06 |
| `PressMix` | how far it moves while pressed | 0.08 |
| `DisabledMix` | how far it fades when disabled | 0.55 |

The mixes are fractions toward another color, so a control of your own can
match the built-in ones rather than inventing its own greys.

### Text

`Text` is the base every piece of text inherits; `Title` and `Caption` are
merged onto it for headings and small secondary text. See
[Local styles](#local-styles) for how the merge resolves.

## Deriving a theme

**Always derive from `DefaultTheme()` or `DarkTheme()`. Never write a `Theme`
literal.** A `Theme` has no "inherit" state the way `TextStyle` does: a field
you leave out is a zero, and a zero `ControlSize` paints a checkbox with no
box at all.

```go
func brandTheme() ggui.Theme {
	t := ggui.DefaultTheme()
	t.Primary = color.RGBA{0x2f, 0x6f, 0xed, 0xff}
	t.PrimaryHover = color.RGBA{0x25, 0x5a, 0xc4, 0xff}
	t.PrimaryFg = color.White
	t.Ring = t.Primary
	t.Radius, t.RadiusSm, t.RadiusLg = 4, 3, 6
	t.Text.Font = inter
	return t
}
```

Changing `Text.Font` alone reaches every piece of text in the app, headings
and captions included, because those are deltas merged onto it.

## Porting a shadcn/ui palette

The color table above is the mapping. A shadcn `:root` block goes across
variable by variable:

```css
--background: #ffffff;   --foreground: #09090b;
--primary:    #18181b;   --primary-foreground: #fafafa;
--border:     #e4e4e7;   --ring: #a1a1aa;
```

```go
// hex is four lines of your own; ggui takes any color.Color.
func hex(s string) color.RGBA { ... }

t := ggui.DefaultTheme()
t.Bg, t.Fg = hex("#ffffff"), hex("#09090b")
t.Primary, t.PrimaryFg = hex("#18181b"), hex("#fafafa")
t.Border, t.Ring = hex("#e4e4e7"), hex("#a1a1aa")
```

Two differences to expect. shadcn derives hover states in CSS; ggui names
`PrimaryHover` as a token, so set it rather than expecting it to follow
`Primary`. And `--radius` is one variable that CSS derives the others from,
where ggui keeps `Radius`, `RadiusSm` and `RadiusLg` separately — set all
three if you change the scale.

## Tokens of your own

A theme carries tokens the framework never looks at, so a control set or a
brand can travel with it rather than beside it. `Set` returns a copy, so a
theme can be derived from another without disturbing it:

```go
var BrandGradient = ggui.NewKey[color.Color]("brand gradient")

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
| `UseTheme()` | returns the theme and subscribes the enclosing Builder, so it rebuilds on a change |
| `SetTheme(t)` | replaces the theme and rebuilds only what read it |
| `BindTheme(sig, on, off)` | follows a `Reader[bool]`, swapping between two themes |
| `Themed(t, child)` | gives one subtree a theme without touching the app's |
| `env.Theme()` | the theme at layout time, for a widget |

The usual dark-mode switch is three lines:

```go
dark := ggui.State(false)
app.Setup(func() { ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme()) })
// somewhere in the tree:
ui.Switch(dark, "Dark mode")
```

Register it in `App.Setup` so the watcher has an owner and is disposed with
the app. A window with no `Config.Background` paints the theme's `Bg`, so the
window follows along.

## Styling a custom widget

Read the theme from the `Env` in `Layout`, not with `UseTheme`:

```go
func (w *MyWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	w.theme = t                    // Paint needs it too
	w.pad = t.ItemPad
	return c.Constrain(...)
}
```

`UseTheme` subscribes a *Builder* and rebuilds it; a widget does not need
rebuilding, because `Layout` runs again anyway and reads the current theme
from the `Env`. Using it inside a widget would register an effect per layout.

Keep what `Paint` needs on the struct, since `Paint` gets no `Env`. That is
the pattern every control in the `ui` package follows, and it is why a theme
change costs a relayout rather than a rebuild.

Two things worth matching when you draw:

- Use the token pairs. A surface and its foreground go together, so the
  control stays legible when someone swaps the palette.
- Use `HoverMix`, `PressMix` and `DisabledMix` for state rather than picking
  greys, so your control ages with the theme.

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

## Where it lives

| Concern | File |
| --- | --- |
| `TextStyle`, `Env`, `Theme`, `DefaultTheme`, `DarkTheme` | [style.go](style.go) |
| `Text`, `Title`, `Caption`, `Styled`, `Provide` | [widgets.go](widgets.go) |
| Shadows | [shadow.go](shadow.go) |
| Motion and transitions | [anim.go](anim.go), [transition.go](transition.go) |
| The controls that consume the tokens | [ui/](ui/) |

See [GUIDE.md](GUIDE.md) for the rest of the framework, and
[GUIDE.md#animation](GUIDE.md#animation) for the values that move over time.

## Theme presets

`ThemePreset` combines the [shadcn/ui semantic color tokens](https://ui.shadcn.com/docs/theming)
with ggui's geometry tokens. Palette data is vendored locally; constructing a
preset never needs a network connection.

```go
preset := ggui.ThemePreset{
    Base: ggui.BaseNeutral,
    Accent: ggui.AccentBlue,
    Style: ggui.StyleRhea,
}
ggui.BindTheme(dark, preset.Dark(), preset.Light())

// Returned Themes are independent values; customize after selecting a preset.
theme := preset.Light()
theme.Chat.BubbleRadius = 20
theme.Chat.BubblePadding = ggui.Insets(10, 14)
ggui.SetTheme(theme)
```

The zero preset uses Neutral/Nova with no accent override. `DefaultTheme()` and
`DarkTheme()` preserve the existing ggui appearance.

| Axis | Presets |
| --- | --- |
| `BaseColor` | Neutral, Stone, Zinc, Mauve, Olive, Mist, Taupe |
| `AccentColor` | Base (no override), Amber, Blue, Cyan, Emerald, Fuchsia, Green, Indigo, Lime, Orange, Pink, Purple, Red, Rose, Sky, Teal, Violet, Yellow |
| `ThemeStyle` | Nova, Rhea |

`BaseColors()`, `AccentColors()` and `ThemeStyles()` return independent slices
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
