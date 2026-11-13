# Create

A native theme playground inspired by [shadcn/ui Create](https://ui.shadcn.com/create).
The neutral editor surrounds a live, interactive card preview. Settings change
the preview without resetting its drafts, selections or scroll position.

```sh
go run ./examples/create
# macOS app bundle: task run-create
```

For the repository's browser runner, use `task serve EXAMPLE=create`.

- Choose Nova, Vega, Lyra, Luma or Rhea geometry, seven base palettes, an accent, a
  chart palette that follows it or goes its own way, body/heading fonts, corner
  radius, menu accent and Lucide/Tabler icons.
- Switch **01 / 02** between the card collection and component specimens.
- Switch light/dark from the header, pick **Light**, **Dark** or **System** in the
  menu, shuffle a palette, or use **Menu → Reset** for Neutral/Nova.
- Hover or focus a setting tile to reveal its lock. Locked choices survive Shuffle.
- **Copy preset** exports JSON; **Open preset** restores it after validation.
- **Get Code** exports a complete Go `applyTheme` function: the recipe, with its
  icon library registered and its menu accent, applied in the chosen mode. `Cmd+Shift+C` copies it directly.
- On narrow windows, **Customize** opens the settings sheet. The preview cards
  reflow to two columns and then one. Wide previews keep 376px cards with horizontal
  scrolling so the reference collage retains its proportions.
- The preview's forms, savings controls, charts and notifications update local
  sample state. They require no network service or account.

## Reuse the theme

`theme.Config{}` is the reusable baseline: Neutral/Nova, chart colors that follow
the accent, preset radii, subtle menus and the application's font and icons. It does not change global
defaults. Apply it to the whole app with `theme.Set`, or just a preview with
`theme.With`:

```go
recipe := theme.Config{
    Preset: theme.Preset{Base: theme.BaseNeutral, Accent: theme.AccentViolet},
    Chart: theme.AccentBlue,
    Radius: theme.RadiusLarge,
    Font: theme.FontInter,
    Heading: theme.FontInter,
    MenuAccent: theme.MenuAccentBold,
}
theme.Set(recipe.Dark())
```

The app starts in dark mode with bundled Inter for body and headings. The editor always keeps its own neutral palette,
so changing the preview's accent or fonts does not recolor the settings panel.

## Scope and reference

The live reference was inspected on 2026-11-13 and refined against the supplied
Create screenshot. The two-line setting tiles, neutral chart bars, contribution
summary tiles, payout form, savings targets, merchant icons and shaded card footers
follow that reference. This example reproduces its
theme-editor/card-collage workflow with GGUI controls. It opens directly in the
editor. Web framework installation, v0 publishing, browser navigation, preset
URLs and CSS backdrop blur are not part of this native example. The font menu
offers bundled Inter, Geist and Geist Mono (real 400/500/600/700 faces), the system
font and Go Sans/Go Mono, so it works offline. See the [Inter](../../fonts/inter/README.md)
and [Geist](../../fonts/geist/README.md) sources and licenses. A preset's JSON is the
recipe and the mode; the recipe's inverted menu color is carried but has no tile,
since the preview's menus do not draw it yet. Styles and icon sets show those implemented
by GGUI. Narrow screens reflow the cards rather than retaining a 2400px canvas.

## Verification and screenshots

```sh
go test ./examples/create ./ui/theme
go run ./examples/create -render-dir /tmp/ggui-create-renders
```

The render command captures the actual widget tree in light/dark at desktop,
tablet and phone widths, with baseline, customized, overlay and component-specimen states (24 PNGs). Tests
exercise theme selection, preserved preview drafts, clipboard/code export,
preset validation and narrow-to-wide resizing.

Verified on 2026-11-13: `go test ./...`, `go vet ./...`, macOS app bundling,
JS/WASM compilation and all 24 screenshot renders passed. Representative
desktop, customized, code-dialog and narrow specimen frames were inspected.
