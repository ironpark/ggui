// Package gofont draws ggui text in the Go fonts on every platform: Go
// Regular with Go Medium and Go Bold, and Go Mono with Go Mono Bold.
//
// An App otherwise draws in the platform's interface font, and a browser,
// which has none, in Go Regular alone, where text asking for a heavier
// weight is drawn regular. Import this package for the same look
// everywhere, or for bold text in the browser; the fonts add about 650 KB
// to the binary, and an app that does not import it does not embed them.
package gofont

import (
	"sync"

	"github.com/ironpark/ggui"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/gomediumitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonobolditalic"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
)

var (
	sans = sync.OnceValue(func() *ggui.Font {
		return ggui.MustFont(goregular.TTF).
			WithWeight(ggui.WeightMedium, ggui.MustFont(gomedium.TTF)).
			WithWeight(ggui.WeightBold, ggui.MustFont(gobold.TTF))
	})
	mono = sync.OnceValue(func() *ggui.Font {
		return ggui.MustFont(gomono.TTF).WithWeight(ggui.WeightBold, ggui.MustFont(gomonobold.TTF))
	})
)

// Font returns Go Regular, with Go Medium and Go Bold as its weights.
func Font() *ggui.Font { return sans() }

// Mono returns Go Mono, with Go Mono Bold as its bold.
func Mono() *ggui.Font { return mono() }

// Enable makes Font and Mono the default text and monospaced fonts, as
// SetDefaultFont and SetDefaultMonoFont do: call it before building the
// app for every App and Probe made on the goroutine.
func Enable() {
	ggui.SetDefaultFont(Font())
	ggui.SetDefaultMonoFont(Mono())
}

// EnableItalic gives Font and Mono their italics, Go Italic with Go Medium
// Italic and Go Bold Italic, and Go Mono Italic with Go Mono Bold Italic,
// for text styled Italic (see ggui.TextStyle.Italic), which is otherwise
// drawn upright in them. The italics add about 850 KB more to the binary,
// only where it is called.
func EnableItalic() { enableItalic() }

var enableItalic = sync.OnceFunc(func() {
	sans().WithItalic(ggui.MustFont(goitalic.TTF).
		WithWeight(ggui.WeightMedium, ggui.MustFont(gomediumitalic.TTF)).
		WithWeight(ggui.WeightBold, ggui.MustFont(gobolditalic.TTF)))
	mono().WithItalic(ggui.MustFont(gomonoitalic.TTF).WithWeight(ggui.WeightBold, ggui.MustFont(gomonobolditalic.TTF)))
})
