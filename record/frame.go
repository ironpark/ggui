package record

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/ironpark/ggui"
)

// FrameStyle is what a recording draws around the app's content.
type FrameStyle uint8

const (
	// FrameNone draws the content alone, the size of the tour: the
	// default.
	FrameNone FrameStyle = iota
	// FrameMac draws the content as a macOS window: a title bar 28 points
	// high with the tour's Title in the middle and the close, minimize and
	// zoom buttons, rounded corners, a hairline border and a soft shadow,
	// on a backdrop. The title bar's colors follow the background the
	// app's theme sets. The other FrameMac styles are the same window
	// with another title bar.
	FrameMac
	// FrameMacTitleless is FrameMac's bar with no title in it, as for a
	// window whose titleVisibility is hidden.
	FrameMacTitleless
	// FrameMacUnified is a window with a unified toolbar
	// (NSWindow.toolbarStyle .unified): one bar 52 points high, the
	// buttons in the middle of its height and the Title after them, bold,
	// with the tour's Subtitle under it.
	FrameMacUnified
	// FrameMacCompact is a window with a compact unified toolbar
	// (.unifiedCompact): one bar 38 points high, the buttons in the middle
	// of its height and the Title after them.
	FrameMacCompact
	// FrameMacExpanded is a window with an expanded toolbar (.expanded):
	// FrameMac's title row, the Title in the middle, over a toolbar row 38
	// points high, left plain.
	FrameMacExpanded
	// FrameMacHidden is a window with a hidden title bar and full-size
	// content, as Config.TitlebarHidden makes: no bar, the content up to
	// the window's top edge and the buttons over it. ButtonInset moves the
	// buttons, as into a sidebar the app draws inset from the window's
	// edges.
	FrameMacHidden
)

var frameNames = [...]string{
	FrameNone:         "none",
	FrameMac:          "mac",
	FrameMacTitleless: "mac-titleless",
	FrameMacUnified:   "mac-unified",
	FrameMacCompact:   "mac-compact",
	FrameMacExpanded:  "mac-expanded",
	FrameMacHidden:    "mac-hidden",
}

func (f FrameStyle) String() string {
	if int(f) < len(frameNames) {
		return frameNames[f]
	}
	return fmt.Sprintf("FrameStyle(%d)", uint8(f))
}

// mac reports whether f draws a macOS window.
func (f FrameStyle) mac() bool { return f >= FrameMac && int(f) < len(frameNames) }

// parseFrame reads a FrameStyle as String writes it.
func parseFrame(s string) (FrameStyle, error) {
	for f, n := range frameNames {
		if n == s {
			return FrameStyle(f), nil
		}
	}
	return 0, fmt.Errorf("record: no frame %q: there are %s", s, strings.Join(frameNames[:], ", "))
}

// macBar is the title bar of a FrameMac style, in points.
type macBar struct {
	height    float64 // 0: no bar, the content up to the top edge
	buttonsY  float64 // the buttons' centres, down from the window's top
	title     titleAt
	separator bool // a hairline under the bar by default
}

// titleAt is where a bar sets the title.
type titleAt uint8

const (
	titleNone    titleAt = iota
	titleCentred         // in the middle of the top 28 points, 13pt semibold
	titleLeading         // after the buttons, 13pt semibold
	titleLarge           // after the buttons, 15pt bold, the subtitle under it
)

func (f FrameStyle) bar() macBar {
	switch f {
	case FrameMac:
		return macBar{height: macTitleBar, buttonsY: macTitleBar / 2, title: titleCentred, separator: true}
	case FrameMacTitleless:
		return macBar{height: macTitleBar, buttonsY: macTitleBar / 2, separator: true}
	case FrameMacUnified:
		return macBar{height: macUnifiedBar, buttonsY: macUnifiedBar / 2, title: titleLarge}
	case FrameMacCompact:
		return macBar{height: macCompactBar, buttonsY: macCompactBar / 2, title: titleLeading}
	case FrameMacExpanded:
		return macBar{height: macTitleBar + macToolbarRow, buttonsY: macTitleBar / 2, title: titleCentred, separator: true}
	}
	return macBar{buttonsY: macTitleBar / 2}
}

// The measures of a framed window, in logical pixels: a macOS window's
// points.
const (
	// DefaultCornerRadius is the radius of a framed window's corners,
	// the one macOS 26 gives a window with a title bar. CornerRadius
	// sets another.
	DefaultCornerRadius = 16.0
	// DefaultMargin is the backdrop left around a framed window, enough
	// for its shadow. Margin sets another.
	DefaultMargin = 48.0

	macTitleBar   = 28.0 // the title bar of a window with no toolbar
	macTitleSize  = 13.0 // its title, in the system font's semibold
	macUnifiedBar = 52.0 // a unified toolbar
	macCompactBar = 38.0 // a compact unified toolbar
	macToolbarRow = 38.0 // an expanded toolbar's row under the title
	macLargeTitle = 15.0 // a unified toolbar's title, bold
	macSubtitle   = 11.0 // and its subtitle, regular
	// The baselines of the two, from the middle of the bar: 14 apart,
	// the title's cap height and the subtitle's baseline even about it.
	macTitleBaseline    = -2.0
	macSubtitleBaseline = 12.0
	macTitleGap         = 14.0 // between the zoom button and a title after it
	macButton           = 12.0 // a window button's diameter
	macButtonGap        = 8.0  // between two buttons
	macButtonX          = 14.0 // the close button's left edge from the window's
	macRim              = 0.5  // a button's darker rim
)

// DefaultBackdrop is what a framed window is drawn on: a neutral gray
// that a light window and a dark one both stand out from.
var DefaultBackdrop color.Color = color.NRGBA{R: 0xd9, G: 0xdc, B: 0xe1, A: 0xff}

// macShadow is a layer of a framed window's shadow: how far down it
// falls, how soft it is, and how dark, in logical pixels.
type macShadow struct{ dy, sigma, alpha float64 }

// The shadow is two layers: a wide ambient one all around, and a key
// light's, nearer and darker, falling below.
var macShadows = [...]macShadow{
	{dy: 0, sigma: 15, alpha: 0.16},
	{dy: 10, sigma: 8, alpha: 0.26},
}

// layout is where a recording's parts go, in device pixels.
type layout struct {
	style   FrameStyle
	scale   float64
	size    image.Point     // of the image written
	content image.Rectangle // where the app is drawn
	window  image.Rectangle // the window around it, its title bar included
	radius  float64         // the window's corners
	bar     macBar
	// separator draws a hairline under the bar.
	separator bool
	// inset moves the buttons right and down, in points.
	inset float64
}

func (t Tour) layout(o *settings) layout {
	cw, ch := t.pixels()
	l := layout{style: t.Frame, scale: t.scale()}
	if o.frame != nil {
		l.style = *o.frame
	}
	if !l.style.mac() {
		l.style = FrameNone
		l.size = image.Pt(cw, ch)
		l.content = image.Rect(0, 0, cw, ch)
		l.window = l.content
		return l
	}
	if t.TitlebarHidden {
		l.style = FrameMacHidden
	}
	l.bar = l.style.bar()
	l.separator = l.bar.separator
	if o.separator != nil {
		l.separator = *o.separator
	}
	if l.bar.height == 0 {
		l.separator = false
		l.inset = max(t.ButtonInset, 0)
		if o.buttonInset >= 0 {
			l.inset = o.buttonInset
		}
	}
	s := l.scale
	margin := o.margin
	if margin < 0 {
		margin = DefaultMargin
	}
	m := int(math.Round(margin * s))
	bar := int(math.Round(l.bar.height * s))
	l.window = image.Rect(m, m, m+cw, m+bar+ch)
	l.content = image.Rect(m, m+bar, m+cw, m+bar+ch)
	l.size = image.Pt(cw+2*m, ch+bar+2*m)
	radius := o.radius
	if radius < 0 {
		radius = DefaultCornerRadius
	}
	l.radius = min(radius*s, float64(min(l.window.Dx(), l.window.Dy()))/2)
	return l
}

// buttons returns the centres of the close, minimize and zoom buttons and
// their radius, in device pixels.
func (l layout) buttons() (c [3]pt, r float64) {
	s := l.scale
	y := float64(l.window.Min.Y) + (l.bar.buttonsY+l.inset)*s
	for i := range c {
		x := l.inset + macButtonX + macButton/2 + float64(i)*(macButton+macButtonGap)
		c[i] = pt{float64(l.window.Min.X) + x*s, y}
	}
	return c, macButton / 2 * s
}

// device returns where the logical point p of the content is in the
// image written.
func (l layout) device(p ggui.Point) pt {
	return pt{float64(l.content.Min.X) + p.X*l.scale, float64(l.content.Min.Y) + p.Y*l.scale}
}

// chrome is the colors of a framed window, from the background the
// app's theme sets.
type chrome struct {
	dark      bool
	bar       color.NRGBA
	title     color.NRGBA
	subtitle  color.NRGBA
	separator color.NRGBA
	border    color.NRGBA
	highlight color.NRGBA // inside the border, in a dark window
}

func chromeFor(bg color.Color) chrome {
	c := color.NRGBAModel.Convert(opaque(bg)).(color.NRGBA)
	lum := (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
	if lum < 0.5 {
		return chrome{
			dark:      true,
			bar:       mix(c, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, 0.07),
			title:     color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xd9},
			subtitle:  color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x8c},
			separator: color.NRGBA{A: 0x80},
			border:    color.NRGBA{A: 0x99},
			highlight: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x24},
		}
	}
	return chrome{
		bar:       mix(c, color.NRGBA{A: 0xff}, 0.055),
		title:     color.NRGBA{A: 0xd9},
		subtitle:  color.NRGBA{A: 0x80},
		separator: color.NRGBA{A: 0x1a},
		border:    color.NRGBA{A: 0x3b},
	}
}

func mix(a, b color.NRGBA, t float64) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	return color.NRGBA{R: f(a.R, b.R), G: f(a.G, b.G), B: f(a.B, b.B), A: 0xff}
}

// The window buttons' colors: the fill and the darker rim.
var macButtons = [3][2]color.NRGBA{
	{{R: 0xff, G: 0x5f, B: 0x57, A: 0xff}, {R: 0xe2, G: 0x46, B: 0x3f, A: 0xff}}, // close
	{{R: 0xfe, G: 0xbc, B: 0x2e, A: 0xff}, {R: 0xe1, G: 0xa1, B: 0x16, A: 0xff}}, // minimize
	{{R: 0x28, G: 0xc8, B: 0x40, A: 0xff}, {R: 0x14, G: 0xae, B: 0x2a, A: 0xff}}, // zoom
}

// framer draws the frame around each frame of content. What is around
// the content is the same from frame to frame until the theme changes,
// and drawn once; a frame copies it and the content, and works out only
// the pixels where the two meet: the rounded corners and the border.
type framer struct {
	l        layout
	title    string
	subtitle string
	backdrop color.NRGBA

	under *image.RGBA // the backdrop and the shadow
	base  *image.RGBA // the whole frame, the content left out
	out   *image.RGBA
	edges []edge

	built bool
	bg    color.RGBA // the background base was drawn for
}

// edge is a pixel of the content that the frame changes: the content is
// covered m/255 by the window, over under, with frame drawn over it.
type edge struct {
	out, in int // offsets in out and in the content
	m       uint32
	under   [4]uint8
	frame   [4]uint8
}

func newFramer(l layout, title, subtitle string, o *settings) *framer {
	bd := o.backdrop
	if bd == nil {
		bd = DefaultBackdrop
	}
	f := &framer{l: l, title: title, subtitle: subtitle, backdrop: color.NRGBAModel.Convert(bd).(color.NRGBA)}
	if f.backdrop.A < 0xff && o.flatten {
		// Over white, as a GIF is.
		f.backdrop = color.NRGBAModel.Convert(opaque(f.backdrop)).(color.NRGBA)
	}
	f.out = image.NewRGBA(image.Rectangle{Max: l.size})
	f.drawUnder()
	return f
}

// windowCov returns how much of the pixel at (x, y) the window covers,
// moved down dy pixels.
func (f *framer) windowCov(x, y int, dy float64) float32 {
	w := f.l.window
	return roundRectCov(float64(x)+0.5, float64(y)+0.5,
		float64(w.Min.X), float64(w.Min.Y)+dy, float64(w.Max.X), float64(w.Max.Y)+dy, f.l.radius)
}

// drawUnder draws the backdrop and the shadow.
func (f *framer) drawUnder() {
	size := f.l.size
	img := image.NewRGBA(image.Rectangle{Max: size})
	if f.backdrop.A > 0 {
		c := color.RGBAModel.Convert(f.backdrop).(color.RGBA)
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
	mask := make([]float32, size.X*size.Y)
	for _, sh := range macShadows {
		dy := sh.dy * f.l.scale
		for y := range size.Y {
			for x := range size.X {
				mask[y*size.X+x] = f.windowCov(x, y, dy)
			}
		}
		gaussBlur(mask, size.X, size.Y, sh.sigma*f.l.scale)
		black := color.NRGBA{A: uint8(sh.alpha*255 + 0.5)}
		for i, a := range mask {
			blend(img.Pix, i*4, black, a)
		}
	}
	f.under = img
}

// draw returns content framed, the window's colors following bg and its
// title set in text's font.
func (f *framer) draw(content *image.RGBA, bg color.Color, text ggui.TextStyle) *image.RGBA {
	key := color.RGBAModel.Convert(opaque(bg)).(color.RGBA)
	if !f.built || key != f.bg {
		f.drawBase(chromeFor(bg), text)
		f.built, f.bg = true, key
	}
	copy(f.out.Pix, f.base.Pix)
	c := f.l.content
	n := c.Dx() * 4
	for y := range c.Dy() {
		o := f.out.PixOffset(c.Min.X, c.Min.Y+y)
		copy(f.out.Pix[o:o+n], content.Pix[y*content.Stride:y*content.Stride+n])
	}
	for _, e := range f.edges {
		px := f.out.Pix[e.out : e.out+4]
		in := content.Pix[e.in : e.in+4]
		for j := range 4 {
			px[j] = uint8((uint32(in[j])*e.m + uint32(e.under[j])*(255-e.m) + 127) / 255)
		}
		over(f.out.Pix, e.out, e.frame[:])
	}
	return f.out
}

// drawBase draws all of the frame, in ch's colors, and works out the
// pixels where it meets the content.
func (f *framer) drawBase(ch chrome, text ggui.TextStyle) {
	l := f.l
	s := l.scale
	base := image.NewRGBA(f.under.Rect)
	copy(base.Pix, f.under.Pix)
	w := l.window
	for y := w.Min.Y; y < w.Max.Y; y++ {
		for x := w.Min.X; x < w.Max.X; x++ {
			blend(base.Pix, base.PixOffset(x, y), ch.bar, f.windowCov(x, y, 0))
		}
	}
	if l.separator {
		y := l.content.Min.Y - 1
		for x := w.Min.X; x < w.Max.X; x++ {
			blend(base.Pix, base.PixOffset(x, y), ch.separator, f.windowCov(x, y, 0))
		}
	}
	f.drawTitle(base, ch, text)

	// What is drawn over the content as well as the bar: the buttons,
	// and the border.
	top := image.NewRGBA(base.Rect)
	centres, r := l.buttons()
	rim := macRim * s
	for i, c := range centres {
		b := image.Rect(int(c.x-r)-1, int(c.y-r)-1, int(c.x+r)+2, int(c.y+r)+2)
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				px, py := float64(x)+0.5, float64(y)+0.5
				o := top.PixOffset(x, y)
				blend(top.Pix, o, macButtons[i][1], circleCov(px, py, c.x, c.y, r))
				blend(top.Pix, o, macButtons[i][0], circleCov(px, py, c.x, c.y, r-rim))
			}
		}
	}
	bw := max(math.Round(s/2), 1)
	ring := func(x, y int, in float64) float32 {
		px, py := float64(x)+0.5, float64(y)+0.5
		a := roundRectCov(px, py, float64(w.Min.X)+in, float64(w.Min.Y)+in, float64(w.Max.X)-in, float64(w.Max.Y)-in, l.radius-in)
		b := roundRectCov(px, py, float64(w.Min.X)+in+bw, float64(w.Min.Y)+in+bw, float64(w.Max.X)-in-bw, float64(w.Max.Y)-in-bw, l.radius-in-bw)
		return max(a-b, 0)
	}
	// The border lies within a corner's reach of the window's edge.
	reach := int(math.Ceil(max(l.radius, 2*bw))) + 2
	for y := w.Min.Y; y < w.Max.Y; y++ {
		for x := w.Min.X; x < w.Max.X; x++ {
			if x >= w.Min.X+reach && x < w.Max.X-reach && y >= w.Min.Y+reach && y < w.Max.Y-reach {
				continue
			}
			o := top.PixOffset(x, y)
			blend(top.Pix, o, ch.border, ring(x, y, 0))
			if ch.dark {
				blend(top.Pix, o, ch.highlight, ring(x, y, bw))
			}
		}
	}
	for i := 0; i < len(base.Pix); i += 4 {
		over(base.Pix, i, top.Pix[i:i+4])
	}

	// The content's pixels the frame changes.
	f.edges = f.edges[:0]
	c := l.content
	for y := c.Min.Y; y < c.Max.Y; y++ {
		for x := c.Min.X; x < c.Max.X; x++ {
			m := f.windowCov(x, y, 0)
			o := top.PixOffset(x, y)
			if m >= 1 && top.Pix[o+3] == 0 {
				continue
			}
			e := edge{
				out: o,
				in:  (y-c.Min.Y)*c.Dx()*4 + (x-c.Min.X)*4,
				m:   uint32(m*255 + 0.5),
			}
			copy(e.under[:], f.under.Pix[o:o+4])
			copy(e.frame[:], top.Pix[o:o+4])
			f.edges = append(f.edges, e)
		}
	}
	f.base = base
}

// drawTitle sets the title where the bar's style has it, in the theme's
// font at macOS's size and weight: in the middle of the title row, or
// after the buttons, with the subtitle under it in a unified toolbar. It
// is cut short where it would reach the buttons, or as near the right
// edge.
func (f *framer) drawTitle(dst *image.RGBA, ch chrome, text ggui.TextStyle) {
	l := f.l
	if f.title == "" || l.bar.title == titleNone {
		return
	}
	s := l.scale
	style := func(size float64, weight ggui.FontWeight, c color.NRGBA) ggui.TextStyle {
		st := text
		st.Size = size * s
		st.Weight = weight
		st.Color = c
		st.LineHeight = 0
		st.Tracking = 0
		st.Italic = false
		return st
	}
	render := func(str string, st ggui.TextStyle) *image.RGBA {
		img := ggui.RenderText(str, st, 0)
		if img == nil || img.Rect.Empty() {
			return nil
		}
		return img
	}
	w := l.window
	centres, r := l.buttons()
	gap := int(math.Ceil(centres[2].x + r + 8*s))
	clip := image.Rect(gap, w.Min.Y, w.Max.X-(gap-w.Min.X), l.content.Min.Y-1)
	sub, ok := dst.SubImage(clip).(*image.RGBA)
	if !ok || sub.Rect.Empty() {
		return
	}
	// A single line is centred on the buttons.
	cy := centres[0].y
	middle := func(img *image.RGBA) int { return int(math.Round(cy - float64(img.Rect.Dy())/2)) }
	switch l.bar.title {
	case titleCentred:
		if img := render(f.title, style(macTitleSize, ggui.WeightSemibold, ch.title)); img != nil {
			cx := float64(w.Min.X+w.Max.X) / 2
			drawOver(sub, img, image.Pt(int(math.Round(cx-float64(img.Rect.Dx())/2)), middle(img)))
		}
		return
	case titleLeading:
		if img := render(f.title, style(macTitleSize, ggui.WeightSemibold, ch.title)); img != nil {
			drawOver(sub, img, image.Pt(l.titleX(), middle(img)))
		}
		return
	}
	tst := style(macLargeTitle, ggui.WeightBold, ch.title)
	title := render(f.title, tst)
	if title == nil {
		return
	}
	var subtitle *image.RGBA
	sst := style(macSubtitle, ggui.WeightRegular, ch.subtitle)
	if f.subtitle != "" {
		subtitle = render(f.subtitle, sst)
	}
	if subtitle == nil {
		drawOver(sub, title, image.Pt(l.titleX(), middle(title)))
		return
	}
	// Two lines, set by their baselines as AppKit sets them, the pair
	// in the middle of the bar.
	tb, sb := baseline(tst), baseline(sst)
	drawOver(sub, title, image.Pt(l.titleX(), int(math.Round(cy+macTitleBaseline*s))-tb))
	drawOver(sub, subtitle, image.Pt(l.titleX(), int(math.Round(cy+macSubtitleBaseline*s))-sb))
}

// baseline returns how far below the top of the image RenderText draws in
// st its baseline lies: where the ink of an H ends.
func baseline(st ggui.TextStyle) int {
	img := ggui.RenderText("H", st, 0)
	if img == nil {
		return 0
	}
	b := img.Rect
	for y := b.Max.Y - 1; y >= b.Min.Y; y-- {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.Pix[img.PixOffset(x, y)+3] >= 0x80 {
				return y - b.Min.Y + 1
			}
		}
	}
	return b.Dy()
}

// titleX is the left edge of a title set after the buttons, in device
// pixels.
func (l layout) titleX() int {
	centres, r := l.buttons()
	return int(math.Round(centres[2].x + r + macTitleGap*l.scale))
}
