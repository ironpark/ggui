// Package geist supplies the official Geist 1.9.2 fonts, Geist and Geist
// Mono, embedded for offline use. Their regular, medium, semibold and bold
// faces have the same metrics and glyphs across platforms. Importing the
// package does not change any default font.
package geist

import (
	"embed"
	"sync"

	"github.com/ironpark/ggui"
)

//go:embed Geist-Regular.ttf Geist-Medium.ttf Geist-SemiBold.ttf Geist-Bold.ttf
//go:embed GeistMono-Regular.ttf GeistMono-Medium.ttf GeistMono-SemiBold.ttf GeistMono-Bold.ttf
var files embed.FS

// family loads the four faces of one family, named by its file prefix.
func family(prefix string) *ggui.Font {
	face := func(name string) *ggui.Font {
		data, err := files.ReadFile(prefix + "-" + name + ".ttf")
		if err != nil {
			panic(err)
		}
		return ggui.MustFont(data)
	}
	return face("Regular").
		WithWeight(ggui.WeightMedium, face("Medium")).
		WithWeight(ggui.WeightSemibold, face("SemiBold")).
		WithWeight(ggui.WeightBold, face("Bold"))
}

var (
	sans = sync.OnceValue(func() *ggui.Font { return family("Geist") })
	mono = sync.OnceValue(func() *ggui.Font { return family("GeistMono") })
)

// Font returns the shared Geist family, parsed once on first use. Treat it as
// immutable. It includes real 400, 500, 600 and 700 weight faces; other weights
// use the nearest face according to GGUI's normal font matching.
func Font() *ggui.Font { return sans() }

// Mono returns the shared Geist Mono family, parsed once on first use, with
// the same four weights as Font. Treat it as immutable.
func Mono() *ggui.Font { return mono() }
