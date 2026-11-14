# Heroicons icons

The whole of [tailwindlabs/heroicons](https://github.com/tailwindlabs/heroicons), 648 icons, as a
Go module of its own: `github.com/ironpark/ggui/icons/heroicons`. See [LICENSE](LICENSE)
for the upstream MIT license. `heroicons.Version` is the release the icons are from.

```go
import "github.com/ironpark/ggui/icons/heroicons"

heroicons.ArrowDownTray.Icon().Size(20)                           // a widget
ui.ButtonOf(heroicons.ArrowDownTray.Icon(), save).Name("Save")   // in a control
icons.Map{icons.Download: heroicons.ArrowDownTray.SVG()}        // in a custom set
ggui.Provide(icons.SetKey, heroicons.Set(), content)          // the controls' roles
```

The 24px outline icons are named as Heroicons names them, `heroicons.ArrowDownTray`, and the 24px solid ones have Solid after the name, `heroicons.ArrowDownTraySolid` (`"arrow-down-tray-solid"` by name). The sources are the ones under `src/`, as drawn, which the SVG renderer is known to draw; the optimized ones pack their arc flags. A name that starts with a digit has
Icon before it.

## Only what is named is linked

Each icon is a variable holding the icon already parsed: the paths and strokes
reading its SVG arrives at, in the compact form `icons.SVG.MarshalBinary` writes.
The linker drops the variables a program does not name, so the catalog adds
nothing to a binary until an icon is used, and then that icon's few hundred
bytes. Drawing one for the first time reads those bytes, about seven times
faster than parsing its SVG, with no XML read at run time. A test builds a
program naming one icon and checks the others are not in it.

Looking an icon up by a name known only at run time takes every icon. That is
the `all` package, for an icon picker or names read from a file:

```go
import heroiconsall "github.com/ironpark/ggui/icons/heroicons/all"

if g := heroiconsall.Lookup("arrow-down-tray"); g != nil {
    row = g.Icon()
}
heroiconsall.Glyphs() // every icon, in name order
```

## Updating

The `glyphs*.go` files are generated from the release, each icon parsed as it
is written. From the ggui repository,
after changing the version in `icons/internal/gen`:

```sh
go run ./icons/internal/gen -lib heroicons
```
