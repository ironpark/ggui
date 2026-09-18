# Themeable icons

GGUI follows shadcn's separation between semantic icon placeholders and an icon
library's concrete names. Resolution happens at runtime through `Env` / `Theme`.
The `ui` package defaults to a curated, embedded Lucide set. `ggui` itself has no
icon-library dependency.

```go
import (
    "github.com/ironpark/ggui"
    "github.com/ironpark/ggui/icons"
    "github.com/ironpark/ggui/icons/lucide"
    "github.com/ironpark/ggui/ui"
)

ui.Icon(icons.Search).Size(20)                   // semantic, themeable
lucide.Icon("download").Size(20)                // explicit library icon
ui.Icon(icons.Download).Alt("Download")         // accessible image
ui.ButtonOf(ui.Icon(icons.Close), close).Named("Close") // decorative icon

// Local subtree selection:
ggui.Provide(icons.SetKey, lucide.Set(), content)

// Theme-level selection (configure both themes when using BindTheme):
light := ggui.DefaultTheme().Set(icons.SetKey, customSet)
dark := ggui.DarkTheme().Set(icons.SetKey, customSet)
ggui.BindTheme(isDark, dark, light)
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
supported by oksvg (paths, strokes, fills, transforms). `currentColor` is
supported. Other source colors become alpha coverage too; use `ggui.Image` for
multicolor artwork. Color inherits from text styling, then theme foreground,
unless `.Color(...)` overrides it. A `ButtonOf` supplies its foreground color to
its content, including disabled-state color.

Bundled assets are loaded and parsed on first use. Each shared SVG caches up to
eight device-pixel sizes, capped at 1024 pixels; colors and rotations reuse these
rasters. CPU parsing can happen before a window exists; drawing uses Ebitengine
on the UI thread. Embedding includes every SVG in the directory in the binary,
so this package bundles only a control-oriented subset, not the entire catalog.
Unknown explicit names return an error from `lucide.Asset` and panic from
`lucide.Icon`. Missing semantic roles with no fallback draw nothing.

References:
- https://github.com/shadcn-ui/ui/blob/main/packages/shadcn/src/utils/transformers/transform-icons.ts
- https://ui.shadcn.com/docs/cli#migrate-icons
- [Lucide source and license](lucide/README.md)

## Bundled libraries

Each library exposes the same `Set()`, `Asset(name)`, and `Icon(name)` API and
maps all 17 semantic roles. Each embeds only a curated outline subset.

| Package | Style | Source and license |
| --- | --- | --- |
| `icons/lucide` | Lucide outline | [Details](lucide/README.md) |
| `icons/tabler` | Tabler outline | [Details](tabler/README.md) |
| `icons/heroicons` | Heroicons 24px outline | [Details](heroicons/README.md) |

```go
import "github.com/ironpark/ggui/icons/tabler"

// Swap built-in control icons throughout the theme.
light := ggui.DefaultTheme().Set(icons.SetKey, tabler.Set())
// Or limit the selection to a subtree.
ggui.Provide(icons.SetKey, tabler.Set(), content)
```

Use `heroicons.Set()` the same way. Explicit named icons stay in their selected
library; semantic `ui.Icon` placeholders and built-in controls follow the set.
The gallery's Icons section provides a library selector for comparison.
