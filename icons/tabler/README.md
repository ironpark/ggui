# Tabler icons

The whole of [tabler/tabler-icons](https://github.com/tabler/tabler-icons), 6287 icons, as a
Go module of its own: `github.com/ironpark/ggui/icons/tabler`. See [LICENSE](LICENSE)
for the upstream MIT license. `tabler.Version` is the release the icons are from.

```go
import "github.com/ironpark/ggui/icons/tabler"

tabler.Download.Icon().Size(20)                           // a widget
ui.ButtonOf(tabler.Download.Icon(), save).Name("Save")   // in a control
icons.Map{icons.Download: tabler.Download.SVG()}        // in a custom set
ggui.Provide(icons.SetKey, tabler.Set(), content)          // the controls' roles
```

The outline icons are named as Tabler names them, `tabler.Download`, and the filled ones have Filled after the name, `tabler.DownloadFilled` (`"download-filled"` by name). A name that starts with a digit has
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
import tablerall "github.com/ironpark/ggui/icons/tabler/all"

if g := tablerall.Lookup("download"); g != nil {
    row = g.Icon()
}
tablerall.Glyphs() // every icon, in name order
```

## Updating

The `glyphs*.go` files are generated from the release, each icon parsed as it
is written. From the ggui repository,
after changing the version in `icons/internal/gen`:

```sh
go run ./icons/internal/gen -lib tabler
```
