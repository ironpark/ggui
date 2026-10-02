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
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
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
