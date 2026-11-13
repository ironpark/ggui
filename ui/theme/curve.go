package theme

import (
	"sync"

	"github.com/ironpark/ggui"
)

// Curve is a timing function as CSS writes it, cubic-bezier(X1, Y1, X2, Y2),
// kept as its four numbers so that a Theme holding one still compares. The
// zero Curve is EaseDefault.
type Curve struct{ X1, Y1, X2, Y2 float64 }

// The curves shadcn's sources name. A Tailwind transition-* utility runs on
// EaseDefault unless an ease-* utility beside it says otherwise, and
// Tailwind's ease-in-out is the same curve; tw-animate-css runs animate-in
// and animate-out on CSS's ease keyword and its accordion on CSS's
// ease-out keyword, which is not Tailwind's ease-out utility.
var (
	EaseDefault = Curve{0.4, 0, 0.2, 1}     // Tailwind: --default-transition-timing-function, --ease-in-out
	EaseCSS     = Curve{0.25, 0.1, 0.25, 1} // CSS: ease
	EaseOutCSS  = Curve{0, 0, 0.58, 1}      // CSS: ease-out
)

var curves struct {
	sync.RWMutex
	m map[Curve]ggui.Easing
}

// Easing is the curve as a Motion, a Tween or Canvas.EaseWith takes it. It
// is made once for each Curve, so a control may ask for it every frame.
func (c Curve) Easing() ggui.Easing {
	if c == (Curve{}) {
		c = EaseDefault
	}
	curves.RLock()
	e, ok := curves.m[c]
	curves.RUnlock()
	if ok {
		return e
	}
	curves.Lock()
	defer curves.Unlock()
	if e, ok := curves.m[c]; ok {
		return e
	}
	if curves.m == nil {
		curves.m = map[Curve]ggui.Easing{}
	}
	e = ggui.CubicBezier(c.X1, c.Y1, c.X2, c.Y2)
	curves.m[c] = e
	return e
}
