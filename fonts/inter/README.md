# Inter

`fonts/inter` bundles the official Inter 4.1 upright static fonts for consistent
offline rendering. `Font()` loads one shared family on first use, with real
regular (400), medium (500), semibold (600) and bold (700) faces. Treat the shared
font as immutable. Loading it does not change application defaults.

```go
import (
    "github.com/ironpark/ggui"
    "github.com/ironpark/ggui/fonts/inter"
)

label := ggui.Text("Contribution History").Style(ggui.TextStyle{
    Font: inter.Font(),
    Weight: ggui.WeightMedium,
})
```

The theme builder exposes the same family through `theme.FontInter` for body or
heading fonts. The default theme and zero `theme.Config` retain their existing
fonts. All four font files total 1,669,112 bytes (about 1.59 MiB); their data is
embedded in binaries that link this package, including `ui/theme` for its Inter
option. No runtime font download or system installation is needed.

## Source and license

The files were retrieved on 2026-11-13 from the official
[rsms/inter v4.1 release](https://github.com/rsms/inter/releases/tag/v4.1), using
[Inter-4.1.zip](https://github.com/rsms/inter/releases/download/v4.1/Inter-4.1.zip).
They are copied without modification from `extras/ttf/` in that archive. The
fonts identify their internal build as `Version 4.001;git-9221beed3`.

Copyright © 2016 The Inter Project Authors. These assets use the **SIL Open Font
License 1.1**, reproduced verbatim from the archive's `LICENSE.txt` in
[OFL.txt](OFL.txt). The font software's license is separate from the repository's
Go source license. Retain the copyright and license when redistributing it.

Archive SHA-256:
`9883fdd4a49d4fb66bd8177ba6625ef9a64aa45899767dde3d36aa425756b11e`

| File | Weight | SHA-256 |
| --- | --- | --- |
| `Inter-Regular.ttf` | 400 | `40d692fce188e4471e2b3cba937be967878f631ad3ebbbdcd587687c7ebe0c82` |
| `Inter-Medium.ttf` | 500 | `97ad806f526e41546d46365bb3a393145f75b7b1568913db74549ad8b8dba872` |
| `Inter-SemiBold.ttf` | 600 | `78a843fade9d4612a5567302fb595b56976eb5fcebf4fea5a5912d638bafcde3` |
| `Inter-Bold.ttf` | 700 | `288316099b1e0a47a4716d159098005eef7c0066921f34e3200393dbdb01947f` |
