# Geist

`fonts/geist` bundles the official Geist 1.9.2 upright static fonts, Geist and
Geist Mono, for consistent offline rendering. Geist is the font of shadcn/ui's
Nova preset. `Font()` and `Mono()` each load one shared family on first use, with
real regular (400), medium (500), semibold (600) and bold (700) faces. Treat the
shared fonts as immutable. Loading them does not change application defaults.

```go
import (
    "github.com/ironpark/ggui"
    "github.com/ironpark/ggui/fonts/geist"
)

label := ggui.Text("Contribution History").Style(ggui.TextStyle{
    Font: geist.Font(),
    Weight: ggui.WeightMedium,
})
code := ggui.Text("go test ./...").Style(ggui.TextStyle{Font: geist.Mono()})
```

The theme builder exposes the same families through `theme.FontGeist` and
`theme.FontGeistMono` for body or heading fonts, and `theme.ApplyCSS` maps a
`--font-sans`, `--font-heading` or `--font-mono` naming `Geist` or `Geist Mono`
to them. The default theme and zero `theme.Config` retain their existing fonts.
All eight font files total 1,108,484 bytes (about 1.06 MiB); their data is
embedded in binaries that link this package, including `ui/theme` for its Geist
options. No runtime font download or system installation is needed.

## Source and license

The files were retrieved on 2026-11-13 from the official
[vercel/geist-font v1.9.2 release](https://github.com/vercel/geist-font/releases/tag/v1.9.2), using
[geist-font-v1.9.2.zip](https://github.com/vercel/geist-font/releases/download/v1.9.2/geist-font-v1.9.2.zip).
They are copied without modification from `Geist/ttf/` and `GeistMono/ttf/` in
that archive.

Copyright 2026 The Geist Project Authors. These assets use the **SIL Open Font
License 1.1**, reproduced verbatim from the archive's `OFL.txt` in
[OFL.txt](OFL.txt). The font software's license is separate from the repository's
Go source license. Retain the copyright and license when redistributing it.

Archive SHA-256:
`15f066d58dcb99fb10014a49913009fdaf41e464cfb8851a44b44e593c95d3d8`

| File | Weight | SHA-256 |
| --- | --- | --- |
| `Geist-Regular.ttf` | 400 | `2c460a7a71063e53491f343d44b382267bc0cf496e31db64865f962af3152e68` |
| `Geist-Medium.ttf` | 500 | `e9099e006d4934cae4a8c7d14a1ca2ea724ee2f69f9bdd5b2ffe715baf5524fc` |
| `Geist-SemiBold.ttf` | 600 | `01923aa148e5202ae70a2e11b2ab4dc41b80bcc374aedccc913d47bb2f591c06` |
| `Geist-Bold.ttf` | 700 | `1c24df313b8ddc9550e060cfe5321c6374997f2892880af303b5ded45fed2582` |
| `GeistMono-Regular.ttf` | 400 | `686914333e77a620c85b35481064788deb33fc25f69901bdbf296c1bbc26a7fe` |
| `GeistMono-Medium.ttf` | 500 | `34c0828460d47e697655bc9d730027a4b8baf6e6c9aa46f5f32b653b4797cf36` |
| `GeistMono-SemiBold.ttf` | 600 | `70b2e3e4218e8bfeed9bf01049411316457936f965e49291863c16f0dafc59a6` |
| `GeistMono-Bold.ttf` | 700 | `8e4dd6ba6c0d420537f400c64cdc130969ab7a4425bb799aa3e07b961dd3062f` |
