# Fonts, emoji, and icons

[Documentation](README.md) · [Project README](../README.md)

Configure text fallbacks, add color emoji, and choose semantic or library-specific icons.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

## Fonts and international text

### Choose text fonts

The built-in font covers Latin, Greek and Cyrillic. Glyphs a font lacks are
drawn from its fallbacks: `SystemFonts()`, the CJK and wide-coverage fonts
found at well-known paths on macOS, Windows and Linux (plus any files in
the `GGUI_FONTS` environment variable), so Korean, Japanese and Chinese
render out of the box on a machine that has such a font. `Fallback(fonts...)`
on a `Font` chooses a chain of your own and `NoFallback()` turns it off.
`LoadFontFile` also reads the first face of a `.ttc`; `LoadFontCollection`
returns them all.

### Add color emoji

`Text` and `TextInput` render color emoji as part of ordinary strings. Emoji
presentation selectors, skin tones, regional-indicator flags, keycaps and ZWJ
sequences stay together when shaping, wrapping, moving the caret or deleting.
Native apps use `SystemEmojiFont()` when available. For a predictable offline
font, including WebAssembly:

```go
import "github.com/ironpark/ggui/fonts/notoemoji"

notoemoji.Enable() // before building the app
label := ggui.Text("Hello 👋🏽 · 🇰🇷 · 👩🏽‍💻 · 1️⃣")
```

The optional Noto package embeds approximately 10.7 MB of font data; apps that
do not import it do not embed it. `SetEmojiFont(font)` selects a caller-owned
CBDT/CBLC, sbix, COLRv0 or OpenType SVG font without replacing the text font.
`SetEmojiFont(nil)` disables substitution; `SetEmojiFont(SystemEmojiFont())`
restores the platform font. `Font.NoFallback()` opts that font out as well.
Actual glyph coverage depends on the selected emoji font. Explicit text
presentation (VS15) stays in the ordinary font; emoji presentation (VS16) uses
the emoji font. Color glyphs retain their colors when text color changes.

## Icons

`ui.Icon(icons.Search)` uses a shared semantic placeholder backed by embedded
Lucide SVGs. Use `lucide.Icon("download")` to request an explicit bundled icon.
Both support `.Size(20)`, `.Color(col)`, and `.Alt("Download")`.

Provide `icons.SetKey` through `ggui.Provide` for a subtree, or store it in a
`Theme` with `.Set(icons.SetKey, set)` to replace built-in control icons. Sets
map roles such as `Check`, `Close`, and `ChevronDown` to parsed SVGs. Missing
roles retain the Lucide defaults. Decorative icons stay out of accessibility;
name the button that contains them, or use `Alt` for a standalone image.

See [icons/README.md](../icons/README.md) for custom SVG sets, precedence and caching.
