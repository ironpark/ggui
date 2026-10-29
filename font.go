package ggui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/textedit"

	"github.com/ironpark/ggui/internal/text"
	"golang.org/x/image/font/gofont/goregular"
)

// Font is a loaded TrueType or OpenType face, usable at any size. Glyphs
// it lacks are drawn from its fallbacks: the ones given with Fallback, or
// else the system fonts SystemFonts finds, so Korean, Japanese and Chinese
// text renders with the built-in Latin font on a machine that has a CJK
// font installed.
type Font struct {
	src      *text.GoTextFaceSource
	load     func() *text.GoTextFaceSource // parses src on first use, for the built-in fonts and those of files
	loadOnce sync.Once
	// light, for a font read from a file, is its metrics, metadata,
	// variation axes and coverage, read without its glyphs: what a
	// fallback chain asks of a font before text needs one of its glyphs.
	// Until then its glyphs stay unread; see chainFace.
	light      *text.GoTextFaceSource
	loaded     atomic.Bool  // source has parsed src
	weight     FontWeight   // what src draws, from its metadata
	weights    []*Font      // the faces WithWeight added
	alias      func() *Font // draws in the font this returns, as DefaultMonoFont does
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
	weight FontWeight
	emoji  *emojiChoice
}

// FontWeight is a CSS font weight, from 100 (thin) to 900 (black). Zero
// asks for nothing, so a TextStyle without one inherits it.
type FontWeight int

const (
	WeightThin       FontWeight = 100
	WeightExtraLight FontWeight = 200
	WeightLight      FontWeight = 300
	WeightRegular    FontWeight = 400
	WeightMedium     FontWeight = 500
	WeightSemibold   FontWeight = 600
	WeightBold       FontWeight = 700
	WeightExtraBold  FontWeight = 800
	WeightBlack      FontWeight = 900
)

// LoadFont parses TTF or OTF bytes.
func LoadFont(data []byte) (*Font, error) {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ggui: load font: %w", err)
	}
	return newFont(src), nil
}

func newFont(src *text.GoTextFaceSource) *Font {
	return &Font{src: src, weight: FontWeight(src.Metadata().Weight)}
}

// lazyFont is a font of weight w parsed from data the first time it draws.
func lazyFont(data []byte, w FontWeight) *Font {
	return &Font{weight: w, load: func() *text.GoTextFaceSource {
		src, err := text.NewGoTextFaceSource(bytes.NewReader(data))
		if err != nil {
			panic(err)
		}
		return src
	}}
}

// source returns the parsed face source, parsing it now for a lazy font.
func (f *Font) source() *text.GoTextFaceSource {
	if f.load != nil {
		f.loadOnce.Do(func() {
			f.src = f.load()
			f.loaded.Store(true)
		})
	}
	return f.src
}

// meta is what f's metadata, variation axes and metrics are read from:
// its light source, for a font of a file, which does not parse its glyphs.
func (f *Font) meta() *text.GoTextFaceSource {
	if f.light != nil {
		return f.light
	}
	return f.source()
}

// WithWeight adds face as f's face for weight w: text whose style asks for
// a weight is drawn with the face nearest it, f itself among them. A
// variable font with a weight axis needs none, since it draws every weight
// on the axis.
//
//	inter := ggui.MustFont(interRegular).
//		WithWeight(ggui.WeightMedium, ggui.MustFont(interMedium)).
//		WithWeight(ggui.WeightBold, ggui.MustFont(interBold))
func (f *Font) WithWeight(w FontWeight, face *Font) *Font {
	reactive.CheckUIThread("Font.WithWeight")
	if face == nil || face == f {
		return f
	}
	face.weight = w
	f.mu.Lock()
	f.weights = append(f.weights, face)
	f.mu.Unlock()
	f.changed()
	return f
}

// forWeight returns the face of f nearest w, and whether it has a weight
// axis to draw w exactly. Between two equally near, the heavier wins above
// regular and the lighter below it, as CSS matches weights.
func (f *Font) forWeight(w FontWeight) (*Font, bool) {
	if w == 0 {
		return f, false
	}
	if lo, hi, ok := f.weightAxis(); ok && float32(w) >= lo && float32(w) <= hi {
		return f, true
	}
	best := f
	for _, c := range f.weights {
		d, bd := abs(int(c.weight-w)), abs(int(best.weight-w))
		if d < bd || d == bd && (c.weight > best.weight) == (w > WeightRegular) {
			best = c
		}
	}
	_, _, axis := best.weightAxis()
	return best, axis
}

var wghtTag = text.MustParseTag("wght")

// weightAxis reports the range of f's weight axis, if it is a variable font.
func (f *Font) weightAxis() (lo, hi float32, ok bool) {
	for _, a := range f.meta().AppendVariationAxes(nil) {
		if a.Tag == wghtTag {
			return a.Min, a.Max, true
		}
	}
	return 0, 0, false
}

// goFace is a face of f's own glyphs at size, set to weight w along a
// weight axis when axis is true.
func (f *Font) goFace(size float64, w FontWeight, axis bool) *text.GoTextFace {
	g := &text.GoTextFace{Source: f.source(), Size: size}
	if axis {
		lo, hi, _ := f.weightAxis()
		g.SetVariation(wghtTag, min(max(float32(w), lo), hi))
	}
	return g
}

// chainFace is goFace as a fallback chain holds it: a font of a file
// stands in as a LazyFace, answering from its light source, until text
// needs one of its glyphs, so a chain of fonts for many scripts reads
// only those of the scripts drawn.
func (f *Font) chainFace(size float64, w FontWeight, axis bool) text.Face {
	if f.light == nil || f.loaded.Load() {
		return f.goFace(size, w, axis)
	}
	light := &text.GoTextFace{Source: f.light, Size: size}
	if axis {
		lo, hi, _ := f.weightAxis()
		light.SetVariation(wghtTag, min(max(float32(w), lo), hi))
	}
	return text.NewLazyFace(light, func() text.Face { return f.goFace(size, w, axis) })
}

func abs(n int) int { return max(n, -n) }

// LoadFontCollection parses a TTC or OTC collection into one Font per face.
func LoadFontCollection(data []byte) ([]*Font, error) {
	srcs, err := text.NewGoTextFaceSourcesFromCollection(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ggui: load font collection: %w", err)
	}
	fonts := make([]*Font, len(srcs))
	for i, src := range srcs {
		fonts[i] = newFont(src)
	}
	return fonts, nil
}

// LoadFontFile loads the font file at path: a TTF or OTF, or the first
// face of a TTC or OTC collection. The file is read as the font draws,
// not into memory, and stays open: a font's glyphs are parsed once text
// needs them, and a color emoji font's bitmaps read as they are drawn.
func LoadFontFile(path string) (*Font, error) {
	r, size, err := openFontFile(path)
	if err != nil {
		return nil, fmt.Errorf("ggui: load font: %w", err)
	}
	return fileFont(r, size, 0)
}

// Fallback appends fonts to draw the glyphs f lacks, in order of
// preference. A Font with none set falls back to SystemFonts.
func (f *Font) Fallback(fonts ...*Font) *Font {
	reactive.CheckUIThread("Font.Fallback")
	f.mu.Lock()
	f.fallbacks = append(f.fallbacks, fonts...)
	f.mu.Unlock()
	f.changed()
	return f
}

// NoFallback draws only f's own glyphs, with no system fonts behind it.
func (f *Font) NoFallback() *Font {
	reactive.CheckUIThread("Font.NoFallback")
	f.mu.Lock()
	f.noFallback = true
	f.mu.Unlock()
	f.changed()
	return f
}

// changed drops f's faces and, if anything has measured with f, every
// measurement everywhere, which is what the font generation guards. A font
// configured before its first use, as a loaded font nearly always is, has
// nothing measured with it, and a probe setting one up on its own goroutine
// must not lay out every other probe again.
func (f *Font) changed() {
	f.mu.Lock()
	used := f.used
	f.faces = nil
	f.mu.Unlock()
	if used {
		fontGeneration.Add(1)
	}
}

var (
	systemOnce  sync.Once
	systemFonts []*Font
)

// SystemFonts returns the fonts found on this machine that cover scripts
// the built-in font does not, mainly CJK, and then symbols interface fonts
// lack, such as the arrows, dingbats and technical symbols command-line
// programs print (❯ ⏵ ⏺ ⎿), found once by family in the index of the
// system's fonts that go-text's fontscan keeps. It is what every Font
// falls back to first; a glyph none of them has is then looked for in
// every font the system has (see systemFallback). Call it to see what was
// found, and Fallback to choose differently. The GGUI_FONTS environment
// variable lists extra files, separated like PATH, tried first.
func SystemFonts() []*Font {
	systemOnce.Do(func() {
		seen := map[string]bool{}
		load := func(files []fontFile) {
			if len(files) == 0 || seen[files[0].path] {
				return
			}
			seen[files[0].path] = true
			// Every weight of the family, so text asking for one draws it
			// in these scripts too.
			if f := loadFamily(files); f != nil {
				f.noFallback = true
				systemFonts = append(systemFonts, f)
			}
		}
		for _, path := range filepath.SplitList(os.Getenv("GGUI_FONTS")) {
			if path != "" {
				load([]fontFile{{path: path}})
			}
		}
		for _, family := range systemFallbackFamilies() {
			load(systemFamilyFiles(family))
		}
	})
	return systemFonts
}

// systemFallbackFamilies lists the families SystemFonts looks for, most
// specific first: one per script where the platform has them, then
// symbols, then pan-Unicode fonts.
func systemFallbackFamilies() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"Apple SD Gothic Neo", // Korean
			"Hiragino Sans",       // Japanese
			"Hiragino Sans GB",    // Simplified Chinese
			"PingFang SC",         // Chinese
			"AppleGothic",         // Korean
			"Noto Sans Gothic",
			"Menlo",         // dingbats such as ❯ ✳ ✻
			"Apple Symbols", // technical symbols such as ⎿
			"STIX Two Math", // media symbols such as ⏵ ⏺
			// Wide coverage, and large: last, so a glyph the fonts above
			// have does not read it.
			"Arial Unicode MS",
		}
	case "windows":
		return []string{
			"Malgun Gothic", "Meiryo", "Yu Gothic", "Microsoft YaHei", "Microsoft JhengHei", "SimSun",
			"Segoe UI", "Segoe UI Symbol", "Cambria Math",
		}
	case "js":
		return nil
	}
	return []string{
		"Noto Sans CJK KR", "Noto Sans CJK JP", "Noto Sans CJK SC", "Noto Sans CJK TC",
		"Noto Sans KR", "Noto Sans JP", "Noto Sans SC",
		"NanumGothic", "Droid Sans Fallback", "WenQuanYi Micro Hei", "WenQuanYi Zen Hei",
		"Noto Sans Symbols", "Noto Sans Symbols 2", "Noto Sans Math", "DejaVu Sans",
	}
}

// MustFont is LoadFont for embedded data that is known to be valid.
func MustFont(data []byte) *Font {
	f, err := LoadFont(data)
	if err != nil {
		panic(err)
	}
	return f
}

// DefaultTextSize is the size Text uses until Size is set.
const DefaultTextSize = 14

// builtinFont is Go Regular, parsed on first use so a program that draws
// no text never pays for it. It is the floor under every default: the font
// of a Probe, and of an App where no system font is found.
var builtinFont = sync.OnceValue(func() *Font { return lazyFont(goregular.TTF, WeightRegular) })

// defaultMono stands for the default monospaced font, which is chosen
// where it draws: see DefaultMonoFont.
var defaultMono = &Font{alias: monoFont}

// DefaultMonoFont returns a Font that draws in the default monospaced font
// of whatever App or Probe it draws in: the one SetDefaultMonoFont set, or
// in an App the platform's own, such as SF Mono on macOS, or else the
// default text font. The theme's Mono style uses it.
func DefaultMonoFont() *Font { return defaultMono }

// SetDefaultFont replaces the font Text uses when none is set. An App
// draws in the platform's interface font, SF Pro on macOS, Segoe UI on
// Windows and fontconfig's sans-serif elsewhere, and a Probe, a browser, or
// an App on a machine without one, in Go Regular, which covers Latin, Greek
// and Cyrillic. fonts/gofont adds Go Medium and Go Bold for the same look
// everywhere.
//
// Like SetEnv, it sets the font of the App or Probe whose frames run on
// this goroutine, or, before any has, the one every App and Probe made on
// it uses until given its own. Nil goes back to that inherited font, or
// to the default above.
func SetDefaultFont(f *Font) {
	reactive.CheckUIThread("SetDefaultFont")
	w := activeWorld()
	w.font = f
	w.fontGen++
}

// SetDefaultMonoFont replaces the font DefaultMonoFont draws in, in the
// same scope SetDefaultFont sets the text font in. Nil goes back to the
// default.
func SetDefaultMonoFont(f *Font) {
	reactive.CheckUIThread("SetDefaultMonoFont")
	w := activeWorld()
	w.mono = f
	w.fontGen++
}

// fallbackFont returns the font Text uses when none is set.
func fallbackFont() *Font {
	native := false
	for w := range activeWorld().chain() {
		if w.font != nil {
			return w.font
		}
		native = native || w.nativeFonts
	}
	if native {
		if f, _ := systemUIFonts(); f != nil {
			return f
		}
	}
	return builtinFont()
}

// monoFont returns the font DefaultMonoFont draws in.
func monoFont() *Font {
	native := false
	for w := range activeWorld().chain() {
		if w.mono != nil {
			return w.mono
		}
		native = native || w.nativeFonts
	}
	if native {
		if _, f := systemUIFonts(); f != nil {
			return f
		}
	}
	return fallbackFont()
}

func (f *Font) face(size float64, weight FontWeight) text.Face {
	if f.alias != nil {
		return f.alias().face(size, weight)
	}
	key := faceKey{size: size, weight: weight}
	if !f.noFallback {
		key.emoji = activeEmoji()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if gen := fontGeneration.Load(); f.generation != gen {
		f.faces = nil
		f.generation = gen
	}
	f.used = true
	if face, ok := f.faces[key]; ok {
		return face
	}
	fonts, multi := f.chainLocked(weight)
	var face text.Face = fonts[0].font.chainFace(size, weight, fonts[0].axis)
	if multi {
		faces := []text.Face{face}
		for _, cf := range fonts[1:] {
			faces = append(faces, cf.font.chainFace(size, weight, cf.axis))
		}
		if f.systemFallback() {
			faces = append(faces, systemFallbackFace(size, weight, face))
		}
		if m, err := text.NewMultiFace(faces...); err == nil {
			face = m
		}
	}
	if !f.noFallback {
		face = withEmoji(face, size, key.emoji)
	}
	if f.faces == nil {
		f.faces = make(map[faceKey]text.Face)
	}
	f.faces[key] = face
	return face
}

// chainFont is one font of a fallback chain, as forWeight picks it.
type chainFont struct {
	font *Font
	axis bool // it draws the weight on its weight axis
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
func (f *Font) chainLocked(w FontWeight) (fonts []chainFont, multi bool) {
	own, axis := f.forWeight(w)
	fonts = []chainFont{{own, axis}}
	fallbacks := f.fallbacks
	if fallbacks == nil && !f.noFallback {
		fallbacks = SystemFonts()
	}
	for _, fb := range fallbacks {
		if fb != f {
			fbFont, fbAxis := fb.forWeight(w)
			fonts = append(fonts, chainFont{fbFont, fbAxis})
		}
	}
	return fonts, len(fallbacks) > 0 || f.systemFallback()
}

func lineWidth(s string, face text.Face) float64 {
	width := 0.0
	for run, f := range textRuns(s, face) {
		width += text.AdvanceAt(run, len(run), f)
	}
	return width
}

// prefixWidths shapes s once and returns the width of s[:b] for any byte
// offset b on a rune boundary, which measuring each prefix would shape
// once per offset. An offset inside a glyph cluster, a ligature say, gets
// the share of the cluster's width that the runes before it have.
func prefixWidths(s string, face text.Face) func(b int) float64 {
	type cluster struct {
		start, end int
		x, w       float64 // the width before the cluster, and its own
	}
	var cs []cluster
	var glyphs []text.LazyGlyph
	off := 0
	for run, f := range textRuns(s, face) {
		glyphs = text.AppendLazyGlyphs(glyphs[:0], run, f, nil)
		for _, g := range glyphs {
			cs = append(cs, cluster{start: off + g.StartIndexInBytes, end: off + g.EndIndexInBytes, w: g.AdvanceX})
		}
		off += len(run)
	}
	// Glyphs come in visual order and a cluster may have several; put
	// them in the order of the text, one entry a cluster.
	slices.SortStableFunc(cs, func(a, b cluster) int { return a.start - b.start })
	merged := cs[:0]
	x := 0.0
	for _, c := range cs {
		if n := len(merged); n > 0 && merged[n-1].start == c.start {
			merged[n-1].w += c.w
		} else {
			c.x = x
			merged = append(merged, c)
		}
		x += c.w
	}
	return func(b int) float64 {
		i := sort.Search(len(merged), func(i int) bool { return merged[i].end > b })
		if i == len(merged) {
			return x
		}
		c := merged[i]
		if b <= c.start {
			return c.x
		}
		return c.x + c.w*float64(utf8.RuneCountInString(s[c.start:b]))/float64(utf8.RuneCountInString(s[c.start:c.end]))
	}
}

// wrapText breaks s into lines no wider than maxW. Hard line breaks are
// kept; soft breaks fall where UAX #14 allows, or between grapheme clusters
// when a single word is wider than the line.
func wrapText(s string, face text.Face, maxW float64) []string {
	var out []string
	for para := range strings.SplitSeq(s, "\n") {
		out = append(out, wrapLine(para, face, maxW)...)
	}
	return out
}

// wrapLine wraps a line without hard breaks as wrapParagraphBy does, the
// spaces a break falls on left out.
func wrapLine(line string, face text.Face, maxW float64) []string {
	spans := wrapParagraphBy(line, func(a, b int) float64 { return lineWidth(line[a:b], face) }, maxW)
	lines := make([]string, len(spans))
	for i, sp := range spans {
		lines[i] = line[sp.start:sp.end]
	}
	return lines
}

// ellipsisText ends a line cut short.
const ellipsisText = "…"

// ellipsize cuts s at a grapheme boundary and ends it with "…", so that it
// fits in maxW. When cut is false, s that already fits comes back whole.
// Spaces before the cut go, and the ellipsis is all that is left when not
// even one grapheme fits beside it.
func ellipsize(s string, face text.Face, maxW float64, cut bool) string {
	if !cut && lineWidth(s, face) <= maxW {
		return s
	}
	keep := ellipsizeBy(s, 0, len(s), prefixWidths(s, face), maxW-lineWidth(ellipsisText, face))
	return strings.TrimRight(s[:keep], " \t") + ellipsisText
}

// ellipsizeBy returns where s[a:b] ends so that it fits in room, at a
// grapheme boundary of s, width(x) measuring s[a:x].
func ellipsizeBy(s string, a, b int, width func(int) float64, room float64) int {
	keep := a
	for i := a; i < b; {
		end := textedit.NextGrapheme(s, i)
		if end > b || width(end) > room {
			break
		}
		keep, i = end, end
	}
	return keep
}

// lineSpan is one drawn line of a wrapped string as byte offsets: the text
// drawn is s[start:end]. A hard line break or the spaces a soft break fell
// on sit between one span's end and the next one's start.
type lineSpan struct{ start, end int }

// wrapSpans is wrapText for an editor: the same breaks, as offsets into s
// rather than copies, so a caret position maps to a line and back.
func wrapSpans(s string, face text.Face, maxW float64) []lineSpan {
	return wrapSpansBy(s, func(a, b int) float64 { return lineWidth(s[a:b], face) }, maxW)
}

// wrapSpansBy is wrapSpans with width measuring s[a:b] however the caller
// draws it, so text in several styles wraps as one paragraph.
func wrapSpansBy(s string, width func(a, b int) float64, maxW float64) []lineSpan {
	var out []lineSpan
	start := 0
	for {
		stop := len(s)
		if i := strings.IndexByte(s[start:], '\n'); i >= 0 {
			stop = start + i
		}
		off := start
		for _, sp := range wrapParagraphBy(s[start:stop], func(a, b int) float64 { return width(off+a, off+b) }, maxW) {
			out = append(out, lineSpan{start + sp.start, start + sp.end})
		}
		if stop == len(s) {
			return out
		}
		start = stop + 1
	}
}

// wrapParagraphBy wraps text without hard breaks, width measuring p[a:b]:
// greedily at the break opportunities of UAX #14, and between grapheme
// clusters when a word alone is wider than maxW. The spaces before a soft
// break end no line: they lie between it and the next.
func wrapParagraphBy(p string, width func(a, b int) float64, maxW float64) []lineSpan {
	if maxW <= 0 || width(0, len(p)) <= maxW {
		return []lineSpan{{0, len(p)}}
	}
	var lines []lineSpan
	lineStart, lineEnd := 0, 0 // the line being filled: its first byte and the end of its last word
	wstart := 0
	for _, brk := range lineBreaks(p) {
		wend := brk
		for wend > wstart && (p[wend-1] == ' ' || p[wend-1] == '\t') {
			wend--
		}
		start := wstart
		wstart = brk
		if wend == start {
			continue // spaces alone, before the first word
		}
		if lineEnd > lineStart && width(lineStart, wend) > maxW {
			// Does not fit after what is on the line: break before it.
			lines = append(lines, lineSpan{lineStart, lineEnd})
			lineStart, lineEnd = start, start
		}
		if width(lineStart, wend) <= maxW {
			lineEnd = wend
			continue
		}
		// A word wider than the line, alone on it: break between grapheme clusters.
		for j := lineStart; j < wend; {
			k := textedit.NextGrapheme(p, j)
			if k < wend && width(lineStart, textedit.NextGrapheme(p, k)) > maxW {
				lines = append(lines, lineSpan{lineStart, k})
				lineStart, j = k, k
				continue
			}
			j = k
		}
		lineEnd = wend
	}
	return append(lines, lineSpan{lineStart, lineEnd})
}

// breakSpans splits s at its line breaks only.
func breakSpans(s string) []lineSpan {
	spans := make([]lineSpan, 0, strings.Count(s, "\n")+1)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			spans = append(spans, lineSpan{start, i})
			start = i + 1
		}
	}
	return append(spans, lineSpan{start, len(s)})
}

// lineOf returns the index of the span that holds byte offset pos: the
// last one starting at or before it.
func lineOf(spans []lineSpan, pos int) int {
	i := sort.Search(len(spans), func(i int) bool { return spans[i].start > pos })
	return max(i-1, 0)
}
