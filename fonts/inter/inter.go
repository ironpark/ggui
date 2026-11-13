// Package inter supplies the official Inter 4.1 font, embedded for offline use.
// Its regular, medium, semibold and bold faces have the same metrics and glyphs
// across platforms. Importing the package does not change any default font.
package inter

import (
	"embed"
	"sync"

	"github.com/ironpark/ggui"
)

//go:embed Inter-Regular.ttf Inter-Medium.ttf Inter-SemiBold.ttf Inter-Bold.ttf
var files embed.FS

var load = sync.OnceValue(func() *ggui.Font {
	face := func(name string) *ggui.Font {
		data, err := files.ReadFile(name)
		if err != nil {
			panic(err)
		}
		return ggui.MustFont(data)
	}
	return face("Inter-Regular.ttf").
		WithWeight(ggui.WeightMedium, face("Inter-Medium.ttf")).
		WithWeight(ggui.WeightSemibold, face("Inter-SemiBold.ttf")).
		WithWeight(ggui.WeightBold, face("Inter-Bold.ttf"))
})

// Font returns the shared Inter family, parsed once on first use. Treat it as
// immutable. It includes real 400, 500, 600 and 700 weight faces; other weights
// use the nearest face according to GGUI's normal font matching.
func Font() *ggui.Font { return load() }
