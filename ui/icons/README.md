# Themeable icons

GGUI follows shadcn's separation between semantic icon placeholders and an icon
library's concrete names. Resolution happens at runtime through `Env` / `Theme`.
The `ui` package defaults to a curated, embedded Lucide set. `ggui` itself has no
icon-library dependency.

```go
import (
    "github.com/ironpark/ggui"
    "github.com/ironpark/ggui/ui/icons"
    "github.com/ironpark/ggui/ui/icons/lucide"
    "github.com/ironpark/ggui/ui"
    "github.com/ironpark/ggui/ui/theme"
)

ui.Icon(icons.Search).Size(20)                   // semantic, themeable
lucide.Icon("download").Size(20)                // explicit library icon
ui.Icon(icons.Download).Alt("Download")         // accessible image
ui.ButtonOf(ui.Icon(icons.Close), close).Name("Close") // decorative icon

// Local subtree selection:
ggui.Provide(icons.SetKey, lucide.Set(), content)

// Theme-level selection (configure both themes when using theme.Bind):
light := theme.Default().Set(icons.SetKey, customSet)
dark := theme.Dark().Set(icons.SetKey, customSet)
app.Setup(func() { theme.Bind(isDark, dark, light) })
```

A set implements `Resolve(icons.Role) *icons.SVG`. Return nil for missing roles.
Resolution checks local Env, theme, then the placeholder's fallback per role.
`ui.Icon(role)` supplies Lucide as fallback; `icons.Placeholder(role)` is library
independent and can use `.Fallback(set)`. `icons.Map` provides a partial override:

```go
//go:embed svg/*.svg
var files embed.FS

check, err := icons.Load(files, "svg/brand-check.svg")
if err != nil { return err }
var customSet icons.Set = icons.Map{icons.Check: check}
content := ggui.Provide(icons.SetKey, customSet,
    ui.Checkbox(enabled, "Enabled"),
)
```

SVG files are trusted local, monochrome assets, using the static SVG subset
that `ggfx/svg` draws (paths, shapes, strokes, fills, transforms, clip paths,
masks). `currentColor` is
supported. Other source colors become alpha coverage too; use `ggui.Image` for
multicolor artwork. Color inherits from text styling, then theme foreground,
unless `.Color(...)` overrides it. A `ButtonOf` supplies its foreground color to
its content, including disabled-state color.

Bundled assets are loaded and parsed on first use. Each shared SVG caches up to
eight device-pixel sizes, capped at 1024 pixels; colors and rotations reuse these
rasters. CPU parsing can happen before a window exists; drawing uses ggfx
on the UI thread. Embedding includes every SVG in the directory in the binary,
so this package bundles only a control-oriented subset, not the entire catalog.
Unknown explicit names return an error from `lucide.Asset` and panic from
`lucide.Icon`. Missing semantic roles with no fallback draw nothing.

References:
- https://github.com/shadcn-ui/ui/blob/main/packages/shadcn/src/utils/transformers/transform-icons.ts
- https://ui.shadcn.com/docs/cli#migrate-icons
- [Lucide source and license](lucide/README.md)

## Whole libraries

The sets bundled here hold only the roles the controls draw. The whole of each
library is a module of its own, a variable for each icon, and a program is
linked with only the icons it names:

| Module | Icons |
| --- | --- |
| [`github.com/ironpark/ggui/icons/lucide`](../../icons/lucide/README.md) | Lucide, 2055 |
| [`github.com/ironpark/ggui/icons/tabler`](../../icons/tabler/README.md) | Tabler outline and filled, 6287 |
| [`github.com/ironpark/ggui/icons/heroicons`](../../icons/heroicons/README.md) | Heroicons 24px outline and solid, 648 |

```go
import "github.com/ironpark/ggui/icons/lucide"

lucide.Rocket.Icon().Size(20)                       // only this icon is linked
icons.Map{icons.Download: lucide.HardDriveDownload.SVG()}
```

An icon is an `*icons.Glyph`, read the first time it is drawn. The libraries
keep each icon already parsed (`Data`, the form `SVG.MarshalBinary` writes), so
drawing one reads no SVG; your own can be declared from the source,
`var Logo = &icons.Glyph{Name: "logo", Source: "<svg …>"}`. Each module's `all`
package lists every icon for a lookup by name at run time, which links them
all; see the module's README.

## Bundled libraries

Each library exposes the same `Set()`, `Asset(name)`, and `Icon(name)` API and
maps all 20 semantic roles. Each embeds only a curated outline subset.

| Package | Style | Source and license |
| --- | --- | --- |
| `ui/icons/lucide` | Lucide outline | [Details](lucide/README.md) |
| `ui/icons/tabler` | Tabler outline | [Details](tabler/README.md) |
| `ui/icons/heroicons` | Heroicons 24px outline | [Details](heroicons/README.md) |

```go
import "github.com/ironpark/ggui/ui/icons/tabler"

// Swap built-in control icons throughout the theme.
light := theme.Default().Set(icons.SetKey, tabler.Set())
// Or limit the selection to a subtree.
ggui.Provide(icons.SetKey, tabler.Set(), content)
```

Use `heroicons.Set()` the same way. Explicit named icons stay in their selected
library; semantic `ui.Icon` placeholders and built-in controls follow the set.
The gallery's Icons section provides a library selector for comparison.
