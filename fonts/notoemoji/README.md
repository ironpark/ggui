# Noto Color Emoji

Optional offline color emoji font for ggui, including WebAssembly builds.

Native apps already draw emoji in the platform's own color emoji font, so
this module is for a browser, which has no system fonts, a Linux machine
without an emoji font, or the same emoji on every platform. It is a module of
its own, so the 10 MB font is downloaded only by applications that require
it:

```sh
go get github.com/ironpark/ggui/fonts/notoemoji
```

```go
import "github.com/ironpark/ggui/fonts/notoemoji"

notoemoji.Enable() // before creating the app
```

The font adds about 11 MB to the uncompressed binary. To keep it out of a
WebAssembly binary, serve the file and load it with `fonts/remote` instead;
see [Fonts](../../docs/fonts.md#load-fonts-separately-on-the-web).

Source: https://github.com/googlefonts/noto-emoji/tree/06121655d0e82f9cae6e7ba6feed4fa6fdbfc2a4
File: 2D/fonts/NotoColorEmoji.ttf (CBDT/CBLC). License: SIL OFL, included in OFL.txt.
