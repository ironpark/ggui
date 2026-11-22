package theme

import (
	"image/color"
	"math"
	"sync"

	"github.com/ironpark/ggui"
)

// Mix returns a moved amount of the way toward b, the way the state mixes
// tint: Mix(fill, t.Fg, t.HoverMix) is fill under the pointer. A nil color
// gives the other one.
func Mix(a, b color.Color, amount float64) color.Color {
	// At either end the mix is one of the two, as it is: a control at rest
	// mixes no tint, every frame, and needs no new color for it.
	if amount <= 0 && a != nil {
		return a
	}
	if amount >= 1 && b != nil {
		return b
	}
	if ggui.IsEnvColor(a) || ggui.IsEnvColor(b) {
		return mixed{a, b, amount}
	}
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	blend := func(x, y uint32) uint8 { return uint8((float64(x)*(1-amount) + float64(y)*amount) / 257) }
	return color.RGBA{blend(ar, br), blend(ag, bg), blend(ab, bb), blend(aa, ba)}
}

// Fade returns c at opacity alpha, from 0 to 1: Fade(fg, 1-t.DisabledMix)
// is fg disabled. A nil color stays nil.
func Fade(c color.Color, alpha float64) color.Color {
	if c == nil || alpha == 1 {
		return c
	}
	if math.IsNaN(alpha) {
		alpha = 0
	}
	switch e := c.(type) {
	case Color:
		return e.Alpha(alpha)
	case ggui.EnvColor:
		return faded{e, alpha}
	}
	r, g, b, a := c.RGBA()
	k := fadeKey{r, g, b, a, alpha}
	fadesMu.Lock()
	defer fadesMu.Unlock()
	if f, ok := fades[k]; ok {
		return f
	}
	if f, ok := fadesPrev[k]; ok {
		putFade(k, f)
		return f
	}
	// Every channel scales, alpha included, because a premultiplied color
	// stays premultiplied only if they all do.
	scale := func(v uint32) uint8 { return uint8(min(max(float64(v>>8)*alpha, 0), 255)) }
	var f color.Color = color.RGBA{scale(r), scale(g), scale(b), scale(a)}
	putFade(k, f)
	return f
}

// fades keeps the colors Fade made, boxed: a widget fades the same colors
// every frame it paints, and each a fresh color.Color is an allocation.
// Entries live in two generations (matching ggui's envMemo): a color hit in
// fadesPrev moves back to fades, so animated opacities cycling through the
// current generation do not wipe static cached colors.
var (
	fadesMu   sync.Mutex
	fades     = map[fadeKey]color.Color{}
	fadesPrev map[fadeKey]color.Color
)

func putFade(k fadeKey, f color.Color) {
	if len(fades) >= mostFades {
		fadesPrev, fades = fades, fadesPrev
		if fades == nil {
			fades = make(map[fadeKey]color.Color, mostFades)
		} else {
			clear(fades)
		}
	}
	fades[k] = f
}

// fadeKey is a color Fade faded, and by how much.
type fadeKey struct {
	r, g, b, a uint32
	alpha      float64
}

// mostFades is how many faded colors Fade keeps.
const mostFades = 256

// Disabled is c as a disabled control draws it, faded by DisabledMix.
func (t Theme) Disabled(c color.Color) color.Color { return Fade(c, 1-t.DisabledMix) }

// Hovered is c under the pointer, moved HoverMix toward Fg.
func (t Theme) Hovered(c color.Color) color.Color { return Mix(c, t.Fg, t.HoverMix) }

// Pressed is c while pressed, moved PressMix toward Fg.
func (t Theme) Pressed(c color.Color) color.Color { return Mix(c, t.Fg, t.PressMix) }

// Halo is ring as the soft halo around a focused field, at RingAlpha.
func (t Theme) Halo(ring color.Color) color.Color { return Fade(ring, t.RingAlpha) }
