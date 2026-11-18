//go:build (unix && !ios && !android) || windows

package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The overlay is what the viewer draws over the app's last frame to say
// what the launcher told it: a build running, a build that failed, an app
// that ended. It is drawn on the CPU into an image of its own, which the
// window's frame draws over the app's: it changes seldom, and it needs no
// more of a font than the Go fonts give.

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayBuilding
	overlayBuildFailed
	overlayAppExited
)

type overlay struct {
	kind   overlayKind
	status string
	output string
	// scroll is the first line of the output shown.
	scroll int
}

// overlayLayout is where the overlay went, in the window's pixels.
type overlayLayout struct {
	// rect is the overlay's rectangle; empty when there is none.
	rect image.Rectangle
	// lines is how many lines the output has, and rows how many of them are
	// shown, from overlay.scroll.
	lines, rows int
	// lineHeight is a line's height.
	lineHeight int
}

func (l overlayLayout) scrollable() bool { return l.rows < l.lines }

var (
	overlayBackground = color.NRGBA{0x18, 0x18, 0x1b, 0xe6}
	pillBackground    = color.NRGBA{0x18, 0x18, 0x1b, 0xd0}
	overlayText       = color.NRGBA{0xf4, 0xf4, 0xf5, 0xff}
	overlayDim        = color.NRGBA{0xa1, 0xa1, 0xaa, 0xff}
	accentBuilding    = color.NRGBA{0x60, 0xa5, 0xfa, 0xff}
	accentFailed      = color.NRGBA{0xef, 0x44, 0x44, 0xff}
	accentExited      = color.NRGBA{0xf5, 0x9e, 0x0b, 0xff}
)

// The panel of a failure covers at most this much of the window's height:
// the app's last frame stays in sight above it.
const panelMaxHeight = 0.55

// render draws o for a window of w by h pixels at scale, returning the
// image and where it goes; a nil image when there is nothing to draw. It
// clamps o.scroll to the output's lines.
func (o *overlay) render(w, h int, scale float64) (*image.RGBA, overlayLayout) {
	if o.kind == overlayNone || w <= 0 || h <= 0 {
		return nil, overlayLayout{}
	}
	if scale <= 0 {
		scale = 1
	}
	px := func(v float64) int { return int(math.Round(v * scale)) }
	if o.kind == overlayBuilding {
		return o.renderPill(w, scale, px)
	}

	title := faceOf(fontRegular, 13*scale)
	mono := faceOf(fontMono, 12*scale)
	margin, pad := px(12), px(12)
	lineHeight := int(math.Ceil(12 * scale * 1.4))
	titleHeight := int(math.Ceil(13 * scale * 1.5))

	pw := max(w-2*margin, px(120))
	adv := mono.advance
	cols := max((pw-2*pad-px(4))/max(adv, 1), 8)
	lines := wrapText(cleanText(o.output), cols)
	header := pad + titleHeight
	rowsMax := max((int(float64(h)*panelMaxHeight)-header-pad)/lineHeight, 1)
	rows := min(len(lines), rowsMax)
	o.scroll = max(min(o.scroll, len(lines)-rows), 0)
	ph := header + pad
	if rows > 0 {
		ph += rows*lineHeight + pad/2
	}

	img := image.NewRGBA(image.Rect(0, 0, pw, ph))
	fillRoundRect(img, img.Bounds(), px(8), overlayBackground)
	accent := accentFailed
	heading := "Build failed"
	if o.kind == overlayAppExited {
		accent = accentExited
		heading = "The app exited"
		if o.status != "" {
			heading += ": " + o.status
		}
	}
	fillRoundRect(img, image.Rect(0, 0, px(4), ph), px(2), accent)
	x := pad + px(4)
	drawText(img, title.face, x, pad+title.ascent, heading, accent)
	if len(lines) > rows {
		hint := fmt.Sprintf("lines %d–%d of %d · scroll", o.scroll+1, o.scroll+rows, len(lines))
		hw := font.MeasureString(title.face, hint).Ceil()
		drawText(img, title.face, pw-pad-hw, pad+title.ascent, hint, overlayDim)
	}
	for i := range rows {
		y := header + i*lineHeight + mono.ascent + (lineHeight-mono.height)/2
		drawText(img, mono.face, x, y, lines[o.scroll+i], overlayText)
	}
	r := image.Rect(margin, h-margin-ph, margin+pw, h-margin)
	return img, overlayLayout{rect: r, lines: len(lines), rows: rows, lineHeight: lineHeight}
}

// renderPill draws the unobtrusive note of a build running, in the top
// right corner.
func (o *overlay) renderPill(w int, scale float64, px func(float64) int) (*image.RGBA, overlayLayout) {
	f := faceOf(fontRegular, 12*scale)
	const text = "Building…"
	tw := font.MeasureString(f.face, text).Ceil()
	padX, padY, dot := px(10), px(6), px(7)
	pw, ph := padX+dot+px(7)+tw+padX, padY+f.height+padY
	img := image.NewRGBA(image.Rect(0, 0, pw, ph))
	fillRoundRect(img, img.Bounds(), ph/2, pillBackground)
	cy := ph / 2
	fillRoundRect(img, image.Rect(padX, cy-dot/2, padX+dot, cy-dot/2+dot), dot/2, accentBuilding)
	drawText(img, f.face, padX+dot+px(7), padY+f.ascent, text, overlayText)
	margin := px(10)
	r := image.Rect(w-margin-pw, margin, w-margin, margin+ph)
	return img, overlayLayout{rect: r}
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[@-Z\\-_]`)

// cleanText makes output fit to draw: no terminal escapes, no carriage
// returns, tabs as spaces, no other control characters, valid UTF-8, and
// no trailing empty lines.
func cleanText(s string) string {
	s = strings.ToValidUTF8(s, "�")
	s = ansiEscape.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	var b strings.Builder
	col := 0
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteRune(r)
			col = 0
		case r == '\t':
			n := 4 - col%4
			b.WriteString(strings.Repeat(" ", n))
			col += n
		case r < 0x20 || r == 0x7f:
		default:
			b.WriteRune(r)
			col++
		}
	}
	return strings.TrimRight(b.String(), "\n ")
}

// wrapText breaks s into lines of at most cols runes.
func wrapText(s string, cols int) []string {
	if s == "" {
		return nil
	}
	var lines []string
	for l := range strings.SplitSeq(s, "\n") {
		for utf8.RuneCountInString(l) > cols {
			i := 0
			for range cols {
				_, size := utf8.DecodeRuneInString(l[i:])
				i += size
			}
			lines = append(lines, l[:i])
			l = l[i:]
		}
		lines = append(lines, l)
	}
	return lines
}

func drawText(dst *image.RGBA, f font.Face, x, baseline int, s string, c color.Color) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, baseline)}
	d.DrawString(s)
}

// fillRoundRect fills r of dst, transparent so far, with c, rounding its
// corners by radius with antialiased edges.
func fillRoundRect(dst *image.RGBA, r image.Rectangle, radius int, c color.NRGBA) {
	r = r.Intersect(dst.Bounds())
	rad := float64(min(radius, r.Dx()/2, r.Dy()/2))
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cover := 1.0
			// The distance from the centre of the corner's circle, if the
			// pixel is in a corner.
			cx, cy := float64(x)+0.5, float64(y)+0.5
			ox := max(float64(r.Min.X)+rad-cx, cx-(float64(r.Max.X)-rad), 0)
			oy := max(float64(r.Min.Y)+rad-cy, cy-(float64(r.Max.Y)-rad), 0)
			if ox > 0 && oy > 0 {
				cover = max(min(rad-math.Hypot(ox, oy)+0.5, 1), 0)
			}
			a := float64(c.A) / 255 * cover
			i := dst.PixOffset(x, y)
			// Over what is there, premultiplied.
			p := dst.Pix[i : i+4 : i+4]
			inv := 1 - a
			p[0] = uint8(math.Round(float64(c.R)*a + float64(p[0])*inv))
			p[1] = uint8(math.Round(float64(c.G)*a + float64(p[1])*inv))
			p[2] = uint8(math.Round(float64(c.B)*a + float64(p[2])*inv))
			p[3] = uint8(math.Round(255*a + float64(p[3])*inv))
		}
	}
}

type fontID int

const (
	fontRegular fontID = iota
	fontMono
)

// sizedFace is a face of a font at a size, with its metrics in pixels.
type sizedFace struct {
	face           font.Face
	ascent, height int
	// advance is the width of a character of a monospaced face.
	advance int
}

var faces struct {
	sync.Mutex
	fonts map[fontID]*opentype.Font
	faces map[[2]float64]sizedFace
}

// faceOf returns the face of font id at size pixels.
func faceOf(id fontID, size float64) sizedFace {
	faces.Lock()
	defer faces.Unlock()
	key := [2]float64{float64(id), size}
	if f, ok := faces.faces[key]; ok {
		return f
	}
	if faces.fonts == nil {
		faces.fonts = map[fontID]*opentype.Font{}
		faces.faces = map[[2]float64]sizedFace{}
	}
	ft := faces.fonts[id]
	if ft == nil {
		src := map[fontID][]byte{fontRegular: goregular.TTF, fontMono: gomono.TTF}[id]
		var err error
		if ft, err = opentype.Parse(src); err != nil {
			panic(err)
		}
		faces.fonts[id] = ft
	}
	face, err := opentype.NewFace(ft, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	m := face.Metrics()
	adv, _ := face.GlyphAdvance('M')
	f := sizedFace{face: face, ascent: m.Ascent.Ceil(), height: m.Height.Ceil(), advance: adv.Ceil()}
	faces.faces[key] = f
	return f
}
