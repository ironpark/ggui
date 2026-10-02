# Go fonts

`fonts/gofont` draws ggui text in the Go fonts on every platform: Go Regular
with Go Medium and Go Bold, and Go Mono with Go Mono Bold.

```go
import "github.com/ironpark/ggui/fonts/gofont"

gofont.Enable() // before building the app
```

Without it an App draws in the platform's interface font, and a browser or a
`Probe` in Go Regular alone, where text asking for a heavier weight is drawn
regular. Import this package for bold text in the browser, or for the same
look on every platform. `gofont.Font()` and `gofont.Mono()` return the fonts
without making them the defaults.

The fonts add about 650 KB to the binary; an app that does not import the
package does not embed them. They come from `golang.org/x/image/font/gofont`
and are licensed under its BSD-style license.
