package record

import (
	"image"
	"image/color"
	"math"

	"github.com/ironpark/ggui"
)

// cursorKind is a cursor drawn: the shapes an app asks for that a
// recording draws, each other shape drawn as the arrow.
type cursorKind uint8

const (
	cursorArrow cursorKind = iota
	cursorHand
	cursorBeam
)

func kindOf(s ggui.CursorShape) cursorKind {
	switch s {
	case ggui.CursorShapePointer:
		return cursorHand
	case ggui.CursorShapeText:
		return cursorBeam
	}
	return cursorArrow
}

// pressedScale is how much smaller the cursor is drawn while a button is
// held.
const pressedScale = 0.85

// A sprite reaches from spriteLeft, spriteTop to spriteRight,
// spriteBottom of the cursor's hot spot, in logical pixels: every shape
// with its outline and shadow.
const (
	spriteLeft   = -8.0
	spriteTop    = -11.0
	spriteRight  = 16.0
	spriteBottom = 23.0
)

// sprite is a cursor drawn, its hot spot at hot.
type sprite struct {
	img *image.RGBA
	hot pt
}

// cursors draws and keeps the sprites of one scale.
type cursors struct {
	scale  float64
	sprite map[[2]int]*sprite
}

func (c *cursors) get(k cursorKind, pressed bool) *sprite {
	key := [2]int{int(k), 0}
	if pressed {
		key[1] = 1
	}
	if sp, ok := c.sprite[key]; ok {
		return sp
	}
	if c.sprite == nil {
		c.sprite = map[[2]int]*sprite{}
	}
	sp := drawCursor(k, c.scale, pressed)
	c.sprite[key] = sp
	return sp
}

// The cursors' shapes, in logical pixels from the hot spot, drawn as a
// standard desktop's are: an outline in one color around a shape in the
// other, and a soft shadow below.
var (
	// The arrow: its tip is the hot spot.
	arrowShape = []pt{{0, 0}, {0, 16.2}, {3.9, 12.5}, {6.4, 18.2}, {8.7, 17.2}, {6.3, 11.6}, {11.4, 11.4}}

	cursorWhite = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	cursorBlack = color.NRGBA{A: 0xff}
)

// handShape returns the pointing hand's parts, grown by d: a glove, its
// index finger up, the tip of which is the hot spot.
func handShape(d float64) [][]pt {
	rr := func(x0, y0, x1, y1, r float64) []pt { return roundRectPoly(x0-d, y0-d, x1+d, y1+d, r+d) }
	return [][]pt{
		rr(-1.75, 0, 1.75, 12, 1.75),                       // index finger
		rr(1.75, 5.8, 5.1, 12.5, 1.7),                      // middle
		rr(5.1, 6.6, 8.4, 12.5, 1.65),                      // ring
		rr(8.4, 7.8, 11.2, 13, 1.4),                        // little
		rr(-1.75, 9.6, 11.2, 18.6, 3.4),                    // palm
		capsulePoly(pt{-0.8, 14.6}, pt{-4.4, 10.6}, 1.6+d), // thumb
	}
}

// handCreases are the lines between the hand's folded fingers.
var handCreases = [][4]float64{{1.75, 7.6, 1.75, 11.4}, {5.1, 8.4, 5.1, 11.4}, {8.4, 9.6, 8.4, 11.6}}

// beamShape returns the I-beam's parts, grown by d: a stem and its two
// serifs, the middle of the stem the hot spot.
func beamShape(d float64) [][]pt {
	rr := func(x0, y0, x1, y1, r float64) []pt { return roundRectPoly(x0-d, y0-d, x1+d, y1+d, r+d) }
	return [][]pt{
		rr(-0.6, -7.6, 0.6, 7.6, 0),
		rr(-3.2, -8.6, 3.2, -7.4, 0.6),
		rr(-3.2, 7.4, 3.2, 8.6, 0.6),
	}
}

// drawCursor draws a cursor of kind k at scale device pixels to the
// logical pixel, smaller when pressed.
func drawCursor(k cursorKind, scale float64, pressed bool) *sprite {
	f := scale
	if pressed {
		f *= pressedScale
	}
	w := int(math.Ceil((spriteRight - spriteLeft) * scale))
	h := int(math.Ceil((spriteBottom - spriteTop) * scale))
	hot := pt{-spriteLeft * scale, -spriteTop * scale}
	place := func(polys ...[]pt) [][]pt {
		out := make([][]pt, len(polys))
		for i, p := range polys {
			q := make([]pt, len(p))
			for j, v := range p {
				q[j] = pt{hot.x + v.x*f, hot.y + v.y*f}
			}
			out[i] = q
		}
		return out
	}
	outer := image.NewAlpha(image.Rect(0, 0, w, h))
	inner := image.NewAlpha(image.Rect(0, 0, w, h))
	creases := image.NewAlpha(image.Rect(0, 0, w, h))
	var outColor, inColor color.NRGBA
	switch k {
	case cursorHand:
		// A white glove outlined in black.
		fillPolys(outer, place(handShape(1)...)...)
		fillPolys(inner, place(handShape(0)...)...)
		for _, c := range handCreases {
			fillPolys(creases, place(roundRectPoly(c[0]-0.35, c[1], c[2]+0.35, c[3], 0.35))...)
		}
		outColor, inColor = cursorBlack, cursorWhite
	case cursorBeam:
		fillPolys(outer, place(beamShape(0.9)...)...)
		fillPolys(inner, place(beamShape(0)...)...)
		outColor, inColor = cursorWhite, cursorBlack
	default:
		// macOS's arrow: black, outlined in white.
		fillPolys(outer, place(offsetPoly(arrowShape, 1.1))...)
		fillPolys(inner, place(arrowShape)...)
		outColor, inColor = cursorWhite, cursorBlack
	}

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// The shadow: the outline's shape, soft, a little below.
	mask := make([]float32, w*h)
	dy := int(math.Round(1 * f))
	for y := range h {
		for x := range w {
			if sy := y - dy; sy >= 0 {
				mask[y*w+x] = float32(outer.Pix[sy*outer.Stride+x]) / 255
			}
		}
	}
	gaussBlur(mask, w, h, 1.2*f)
	shadow := color.NRGBA{A: 0x66}
	for i, a := range mask {
		blend(img.Pix, i*4, shadow, a)
	}
	for i := range w * h {
		blend(img.Pix, i*4, outColor, float32(outer.Pix[i])/255)
		blend(img.Pix, i*4, inColor, float32(inner.Pix[i])/255)
		blend(img.Pix, i*4, cursorBlack, float32(creases.Pix[i])/255)
	}
	return &sprite{img: img, hot: hot}
}

// draw draws sp with its hot spot at p, clipped to dst.
func (sp *sprite) draw(dst *image.RGBA, p pt) {
	at := image.Pt(int(math.Round(p.x-sp.hot.x)), int(math.Round(p.y-sp.hot.y)))
	drawOver(dst, sp.img, at)
}

// The ring a click leaves: it widens from ringFrom to ringTo logical
// pixels and fades over ringTime.
const (
	ringFrom  = 5.0
	ringTo    = 18.0
	ringWidth = 2.0
	ringAlpha = 0.45
)

// drawRing draws the ring a click at c, in device pixels, leaves, t of
// the way through its fade, in col, at scale.
func drawRing(dst *image.RGBA, c pt, t, scale float64, col color.NRGBA) {
	if t < 0 || t >= 1 {
		return
	}
	e := 1 - (1-t)*(1-t) // easing out
	r := (ringFrom + (ringTo-ringFrom)*e) * scale
	hw := ringWidth / 2 * scale
	a := float32(ringAlpha * (1 - t))
	b := image.Rect(int(c.x-r-hw)-1, int(c.y-r-hw)-1, int(c.x+r+hw)+2, int(c.y+r+hw)+2).Intersect(dst.Rect)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			d := math.Abs(math.Hypot(float64(x)+0.5-c.x, float64(y)+0.5-c.y) - r)
			cov := float32(clamp01(hw - d + 0.5))
			if cov > 0 {
				blend(dst.Pix, dst.PixOffset(x, y), col, cov*a)
			}
		}
	}
}
