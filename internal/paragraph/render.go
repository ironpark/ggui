package paragraph

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"slices"

	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/textlayout"
	"github.com/ironpark/ggui/internal/third_party/typesetting/di"
	"github.com/ironpark/ggui/internal/third_party/typesetting/font"
	"github.com/ironpark/ggui/internal/third_party/typesetting/font/opentype"
	"github.com/ironpark/ggui/internal/third_party/typesetting/language"
	"github.com/ironpark/ggui/internal/third_party/typesetting/render"
	"github.com/ironpark/ggui/internal/third_party/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"github.com/ironpark/ggui/internal/emojidata"
	"github.com/ironpark/ggui/internal/third_party/text"
)

// Render draws s in st and col on the CPU, with no GPU, window or App,
// into an image just large enough to hold it. Lines break at hard breaks
// and, when maxWidth is positive, wrap to maxWidth as Text wraps them. A
// pixel of the image is a pixel of st.Size.
//
// Glyphs come from st.Font and its fallbacks, as Text draws them, and emoji
// from the emoji font, in color. Text is shaped with go-text and drawn with
// go-text/render; emoji are drawn as Text draws them, from sbix, CBDT,
// COLRv0 or SVG glyphs, rasterized on the CPU.
func Render(s string, st Style, col color.Color, maxWidth float64) *image.RGBA {
	r := newTextRenderer(st)
	lines := textlayout.Wrap(s, r.face, max(maxWidth, 0))
	shaped := make([]shaping.Line, len(lines))
	texts := make([][]rune, len(lines))
	width := 0.0
	for i, line := range lines {
		shaped[i], texts[i] = r.shapeLine(line)
		w := 0.0
		for _, run := range shaped[i] {
			w += float64(run.Advance) / 64
		}
		width = max(width, w)
	}
	m := r.face.Metrics()
	lh := st.LineHeight * st.Size
	height := m.HAscent + float64(max(len(lines)-1, 0))*lh + m.HDescent
	img := image.NewRGBA(image.Rect(0, 0, int(math.Ceil(width)), int(math.Ceil(height))))
	pen := render.Renderer{PixScale: 1, Color: col}
	for i, line := range shaped {
		x, y := 0, int(math.Round(m.HAscent+float64(i)*lh))
		for _, run := range line {
			if r.emoji != nil && run.Face == r.emoji {
				x = r.drawEmoji(img, string(texts[i][run.Runes.Offset:run.Runes.Offset+run.Runes.Count]), x, y)
				continue
			}
			pen.FontSize = float32(run.Size) / 64
			x = pen.DrawShapedRunAt(run, img, x, y)
		}
	}
	return img
}

// drawEmoji draws emoji s with its baseline at y from x as Text draws
// them, and returns the x after them.
func (r *textRenderer) drawEmoji(img *image.RGBA, s string, x, y int) int {
	top := y - int(math.Round(r.emojiFace.Metrics().HAscent))
	for _, g := range text.AppendLazyGlyphs(nil, s, r.emojiFace, nil) {
		if px := g.Pixels(); px != nil {
			draw.Draw(img, g.ImageBounds.Add(image.Pt(x, top)), px, px.Bounds().Min, draw.Over)
		}
	}
	return x + int(math.Ceil(text.AdvanceAt(s, len(s), r.emojiFace)))
}

// textRenderer shapes text in a style with go-text's own faces: a Face is
// not safe for concurrent use, so each Render makes its own of the
// sources it draws with.
type textRenderer struct {
	style     Style
	face      text.Face       // the style's face, as Text has it, for wrapping and metrics
	picker    *fontsys.Picker // the font and its fallbacks, at the style's weight
	chain     []*font.Face    // the picker's fonts' faces, in its order
	runes     map[rune]*font.Face
	emoji     *font.Face // the emoji font, nil without one
	emojiFace text.Face  // the emoji font as Text draws it, fitted to the line box
	esize     float64    // the size emoji are shaped at

	seg    shaping.Segmenter
	shaper shaping.HarfbuzzShaper
	wrap   shaping.LineWrapper
}

func newTextRenderer(st Style) *textRenderer {
	r := &textRenderer{style: st, face: st.Face(1), picker: st.Font.Picker(st.Weight), runes: map[rune]*font.Face{}}
	r.grow()
	if ef, ok := r.face.(*fontsys.EmojiFace); ok {
		if e, ok := ef.ColorFace().(*text.GoTextFace); ok && e != nil {
			r.emoji = ownFace(e.Source, 0, false)
			r.emojiFace, r.esize = e, e.Size
		}
	}
	return r
}

var wghtAxis = opentype.MustNewTag("wght")

// ownFace returns a Face of its own for src, drawing weight w on its weight
// axis when axis is set.
func ownFace(src *text.GoTextFaceSource, w fontsys.Weight, axis bool) *font.Face {
	face := font.NewFace(src.UnsafeInternal().(*font.Face).Font)
	if axis {
		face.SetVariations([]font.Variation{{Tag: wghtAxis, Value: float32(w)}})
	}
	return face
}

// grow makes faces of the fonts the picker has added since.
func (r *textRenderer) grow() {
	for len(r.chain) < r.picker.Len() {
		cf := r.picker.Font(len(r.chain))
		r.chain = append(r.chain, ownFace(cf.Font.Source(), r.style.Weight, cf.Axis))
	}
}

// ResolveFace implements shaping.Fontmap: the face of the font the picker
// finds for r, else the font's own.
func (r *textRenderer) ResolveFace(c rune) *font.Face {
	if f, ok := r.runes[c]; ok {
		return f
	}
	face := r.chain[0]
	if i, ok := r.picker.Pick(c); ok {
		r.grow()
		face = r.chain[i]
	}
	r.runes[c] = face
	return face
}

// shapeLine shapes line into runs in visual order, with the runes their
// offsets index: each emoji grapheme in the emoji font, the rest split by
// direction, script and the face that has each rune.
func (r *textRenderer) shapeLine(line string) (shaping.Line, []rune) {
	runes := []rune(line)
	runeAt := make([]int, len(line)+1) // the index in runes of each byte offset that starts a rune
	n := 0
	for i := range line {
		runeAt[i] = n
		n++
	}
	runeAt[len(line)] = n
	var outs []shaping.Output
	size := fixed.Int26_6(math.Round(r.style.Size * 64))
	pick := func(g string) bool { return r.emoji != nil && emojidata.IsCluster(g) }
	textedit.GraphemeRuns(line, pick, func(start, end int, emoji bool) bool {
		in := shaping.Input{Text: runes, RunStart: runeAt[start], RunEnd: runeAt[end], Direction: di.DirectionLTR, Size: size}
		if emoji {
			in.Face, in.Size, in.Script = r.emoji, fixed.Int26_6(math.Round(r.esize*64)), language.Common
			outs = append(outs, r.shaper.Shape(in))
			return true
		}
		for _, part := range r.seg.Split(in, r) {
			outs = append(outs, r.shaper.Shape(part))
		}
		return true
	})
	if len(outs) == 0 {
		return nil, runes
	}
	r.wrap.Prepare(shaping.WrapConfig{Direction: di.DirectionLTR}, runes, shaping.NewSliceIterator(outs))
	wrapped, _ := r.wrap.WrapNextLine(math.MaxInt32)
	visual := slices.Clone(wrapped.Line)
	slices.SortStableFunc(visual, func(a, b shaping.Output) int { return int(a.VisualIndex - b.VisualIndex) })
	return visual, runes
}
