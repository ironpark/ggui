// Package fontsys is ggui's font engine: fonts loaded from bytes and files,
// the faces they make at a size and weight and the fallback chains those
// faces draw through, the system's fonts and the fallbacks found among
// them, and the color emoji face that stands beside a chain.
//
// What a world chose, its default, monospaced and emoji fonts, is ggui's:
// a face is asked for here with the emoji choice already resolved.
package fontsys

import (
	"bytes"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/ironpark/ggui/internal/fn"
	"github.com/ironpark/ggui/internal/third_party/text"
	"golang.org/x/image/font/gofont/goregular"
)

// Weight is a CSS font weight, from 100 (thin) to 900 (black). Zero asks
// for nothing.
type Weight int

// weightRegular is the weight between the lighter and the heavier, which
// decides which way a tie between two faces goes; see ForWeight.
const weightRegular Weight = 400

// Font is a loaded TrueType or OpenType face, usable at any size, with the
// faces of other weights and the fallbacks it draws through.
type Font struct {
	src      *text.GoTextFaceSource
	load     func() *text.GoTextFaceSource // parses src on first use, for the built-in fonts and those of files
	loadOnce sync.Once
	// light, for a font read from a file, is its metrics, metadata,
	// variation axes and coverage, read without its glyphs: what a
	// fallback chain asks of a font before text needs one of its glyphs.
	// Until then its glyphs stay unread; see chainFace.
	light      *text.GoTextFaceSource
	lightLoad  func() *text.GoTextFaceSource // reads light on first use, for a font of the system's index
	lightOnce  sync.Once
	loaded     atomic.Bool  // source has parsed src
	weight     Weight       // what src draws, from its metadata
	weights    []*Font      // the faces WithWeight added
	alias      func() *Font // draws in the font this returns, as ggui's default mono font does
	fallbacks  []*Font
	noFallback bool

	// mu guards the cache, which probes on goroutines of their own fill
	// at once from a font they share.
	mu         sync.Mutex
	used       bool // a face has been made, so something may have measured with f
	generation uint64
	faces      map[faceKey]text.Face // reused across frames
}

// faceKey is what a face depends on beside the Font: its size and weight,
// and the emoji font the world measuring with it chose.
type faceKey struct {
	size   float64
	weight Weight
	emoji  *EmojiChoice
}

// generation counts the changes to fonts something has measured with. It
// is shared by probes on goroutines of their own, so it is atomic.
var generation atomic.Uint64

// Generation changes whenever a Font that something measured with
// changes, so a measurement made with it may no longer hold.
func Generation() uint64 { return generation.Load() }

// newFont is the font src draws.
func newFont(src *text.GoTextFaceSource) *Font {
	return &Font{src: src, weight: Weight(src.Metadata().Weight)}
}

// Load parses TTF or OTF bytes.
func Load(data []byte) (*Font, error) {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return newFont(src), nil
}

// LoadCollection parses a TTC or OTC collection into one Font per face.
func LoadCollection(data []byte) ([]*Font, error) {
	srcs, err := text.NewGoTextFaceSourcesFromCollection(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	fonts := make([]*Font, len(srcs))
	for i, src := range srcs {
		fonts[i] = newFont(src)
	}
	return fonts, nil
}

// LoadFile loads the font file at path: a TTF or OTF, or the first face of
// a TTC or OTC collection, read as it draws; see openFontFile.
func LoadFile(path string) (*Font, error) {
	r, size, err := openFontFile(path)
	if err != nil {
		return nil, err
	}
	return fileFont(r, size, 0)
}

// lazy is a font of weight w parsed from data the first time it draws.
func lazy(data []byte, w Weight) *Font {
	return &Font{weight: w, load: func() *text.GoTextFaceSource {
		src, err := text.NewGoTextFaceSource(bytes.NewReader(data))
		if err != nil {
			panic(err)
		}
		return src
	}}
}

// Alias is a Font that draws in whichever font to returns when it draws.
func Alias(to func() *Font) *Font { return &Font{alias: to} }

// Builtin is Go Regular, parsed on first use, which covers Latin, Greek
// and Cyrillic: the floor under every default, and what a font of the
// system's index draws as when its file is gone.
var Builtin = sync.OnceValue(func() *Font { return lazy(goregular.TTF, weightRegular) })

// Resolve follows f's aliases to the font it draws in now.
func (f *Font) Resolve() *Font {
	for f.alias != nil {
		f = f.alias()
	}
	return f
}

// Source returns the parsed face source, parsing it now for a lazy font.
func (f *Font) Source() *text.GoTextFaceSource {
	if f.load != nil {
		f.loadOnce.Do(func() {
			f.src = f.load()
			f.loaded.Store(true)
		})
	}
	return f.src
}

// Meta is what f's metadata, variation axes and metrics are read from:
// its light source, for a font of a file, which does not parse its glyphs.
func (f *Font) Meta() *text.GoTextFaceSource {
	if l := f.lightSource(); l != nil {
		return l
	}
	return f.Source()
}

// lightSource returns f's light source, reading it now for a font of the
// system's index; nil for a font with none.
func (f *Font) lightSource() *text.GoTextFaceSource {
	if f.lightLoad != nil {
		f.lightOnce.Do(func() { f.light = f.lightLoad() })
	}
	return f.light
}

// WithWeight adds face as f's face for weight w.
func (f *Font) WithWeight(w Weight, face *Font) {
	if face == nil || face == f {
		return
	}
	face.weight = w
	f.mu.Lock()
	f.weights = append(f.weights, face)
	f.mu.Unlock()
	f.changed()
}

// AddFallbacks appends fonts to draw the glyphs f lacks, in order of
// preference. A Font with none set falls back to SystemFonts.
func (f *Font) AddFallbacks(fonts ...*Font) {
	f.mu.Lock()
	f.fallbacks = append(f.fallbacks, fonts...)
	f.mu.Unlock()
	f.changed()
}

// SetNoFallback makes f draw only its own glyphs, with no system fonts
// behind it.
func (f *Font) SetNoFallback() {
	f.mu.Lock()
	f.noFallback = true
	f.mu.Unlock()
	f.changed()
}

// changed drops f's faces and, if anything has measured with f, every
// measurement everywhere, which is what the generation guards. A font
// configured before its first use, as a loaded font nearly always is, has
// nothing measured with it, and a probe setting one up on its own goroutine
// must not lay out every other probe again.
func (f *Font) changed() {
	f.mu.Lock()
	used := f.used
	f.faces = nil
	f.mu.Unlock()
	if used {
		generation.Add(1)
	}
}

// ForWeight returns the face of f nearest w, and whether it has a weight
// axis to draw w exactly. Between two equally near, the heavier wins above
// regular and the lighter below it, as CSS matches weights.
func (f *Font) ForWeight(w Weight) (*Font, bool) {
	if w == 0 {
		return f, false
	}
	if lo, hi, ok := f.weightAxis(); ok && float32(w) >= lo && float32(w) <= hi {
		return f, true
	}
	best := f
	for _, c := range f.weights {
		d, bd := abs(int(c.weight-w)), abs(int(best.weight-w))
		if d < bd || d == bd && (c.weight > best.weight) == (w > weightRegular) {
			best = c
		}
	}
	_, _, axis := best.weightAxis()
	return best, axis
}

var wghtTag = text.MustParseTag("wght")

// weightAxis reports the range of f's weight axis, if it is a variable font.
func (f *Font) weightAxis() (lo, hi float32, ok bool) {
	for _, a := range f.Meta().AppendVariationAxes(nil) {
		if a.Tag == wghtTag {
			return a.Min, a.Max, true
		}
	}
	return 0, 0, false
}

// goFace is a face of f's own glyphs at size, set to weight w along a
// weight axis when axis is true.
func (f *Font) goFace(size float64, w Weight, axis bool) *text.GoTextFace {
	g := &text.GoTextFace{Source: f.Source(), Size: size}
	f.setWeight(g, w, axis)
	return g
}

// setWeight sets g to weight w along f's weight axis, clamped to it, when
// axis is true.
func (f *Font) setWeight(g *text.GoTextFace, w Weight, axis bool) {
	if axis {
		lo, hi, _ := f.weightAxis()
		g.SetVariation(wghtTag, fn.Clamp(float32(w), lo, hi))
	}
}

// chainFace is goFace as a fallback chain holds it: a font of a file
// stands in as a LazyFace, answering from its light source, until text
// needs one of its glyphs, so a chain of fonts for many scripts reads
// only those of the scripts drawn.
func (f *Font) chainFace(size float64, w Weight, axis bool) text.Face {
	return f.ChainFaceWith(size, w, axis, nil)
}

// ChainFaceWith is chainFace with set, when not nil, applied to each
// GoTextFace made, the light one and the loaded one alike: to shape right
// to left, say.
func (f *Font) ChainFaceWith(size float64, w Weight, axis bool, set func(*text.GoTextFace)) text.Face {
	face := func() text.Face {
		g := f.goFace(size, w, axis)
		if set != nil {
			set(g)
		}
		return g
	}
	if f.light == nil && f.lightLoad == nil || f.loaded.Load() {
		return face()
	}
	light := &text.GoTextFace{Source: f.lightSource(), Size: size}
	f.setWeight(light, w, axis)
	if set != nil {
		set(light)
	}
	return text.NewLazyFace(light, face)
}

func abs(n int) int { return max(n, -n) }

// Face returns f's face at size and weight: its own glyphs, then its
// fallbacks', then, unless it has fallbacks of its own, the system's, and
// beside them the emoji font emoji returns, nil for the system's. emoji is
// asked only for a font that draws emoji at all, and may itself be nil.
// Faces are cached until f or the generation changes.
func (f *Font) Face(size float64, weight Weight, emoji func() *EmojiChoice) text.Face {
	f = f.Resolve()
	var choice *EmojiChoice
	if !f.noFallback && emoji != nil {
		choice = emoji()
	}
	return f.FaceFor(size, weight, choice)
}

// FaceFor is Face with the emoji choice already made: emoji is the font's
// beside f's faces, nil for the system's, and ignored for a font that
// draws no emoji.
func (f *Font) FaceFor(size float64, weight Weight, emoji *EmojiChoice) text.Face {
	f = f.Resolve()
	key := faceKey{size: size, weight: weight}
	if !f.noFallback {
		key.emoji = emoji
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if gen := generation.Load(); f.generation != gen {
		f.faces = nil
		f.generation = gen
	}
	f.used = true
	if face, ok := f.faces[key]; ok {
		return face
	}
	fonts, multi := f.chainLocked(weight)
	var face text.Face = fonts[0].Font.chainFace(size, weight, fonts[0].Axis)
	if multi {
		faces := []text.Face{face}
		for _, cf := range fonts[1:] {
			faces = append(faces, cf.Font.chainFace(size, weight, cf.Axis))
		}
		if f.systemFallback() {
			faces = append(faces, systemFallbackFace(size, weight, face))
		}
		if m, err := text.NewMultiFace(faces...); err == nil {
			face = m
		}
	}
	if !f.noFallback {
		face = WithEmoji(face, size, key.emoji)
	}
	if f.faces == nil {
		f.faces = make(map[faceKey]text.Face)
	}
	f.faces[key] = face
	return face
}

// ChainFont is one font of a fallback chain, as ForWeight picks it.
type ChainFont struct {
	Font *Font
	Axis bool // it draws the weight on its weight axis
}

// systemFallback reports whether f falls back to the system's fonts: to
// SystemFonts, and then to any font that has a glyph they lack.
func (f *Font) systemFallback() bool {
	return f.fallbacks == nil && !f.noFallback && runtime.GOOS != "js"
}

// chainLocked returns the fonts f draws weight w with, in order: its own
// for w, then each fallback's for w. multi reports whether f has
// fallbacks at all, its own or the system's, which makes the face a
// fallback chain even when the only one is f itself. f.mu must be held.
func (f *Font) chainLocked(w Weight) (fonts []ChainFont, multi bool) {
	own, axis := f.ForWeight(w)
	fonts = []ChainFont{{own, axis}}
	fallbacks := f.fallbacks
	if fallbacks == nil && !f.noFallback {
		fallbacks = SystemFonts()
	}
	for _, fb := range fallbacks {
		if fb != f {
			fbFont, fbAxis := fb.ForWeight(w)
			fonts = append(fonts, ChainFont{fbFont, fbAxis})
		}
	}
	return fonts, len(fallbacks) > 0 || f.systemFallback()
}
