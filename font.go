package ggui

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

// Font is a loaded TrueType or OpenType face, usable at any size. Glyphs
// it lacks are drawn from its fallbacks: the ones given with Fallback, or
// else the system fonts SystemFonts finds, so Korean, Japanese and Chinese
// text renders with the built-in Latin font on a machine that has a CJK
// font installed.
type Font struct {
	src        *text.GoTextFaceSource
	fallbacks  []*Font
	noFallback bool
	generation uint64
	faces      map[float64]text.Face // one face per size, reused across frames
}

// LoadFont parses TTF or OTF bytes.
func LoadFont(data []byte) (*Font, error) {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ggui: load font: %w", err)
	}
	return &Font{src: src}, nil
}

// LoadFontCollection parses a TTC or OTC collection into one Font per face.
func LoadFontCollection(data []byte) ([]*Font, error) {
	srcs, err := text.NewGoTextFaceSourcesFromCollection(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ggui: load font collection: %w", err)
	}
	fonts := make([]*Font, len(srcs))
	for i, src := range srcs {
		fonts[i] = &Font{src: src}
	}
	return fonts, nil
}

// LoadFontFile parses the font file at path: a TTF or OTF, or the first
// face of a TTC or OTC collection.
func LoadFontFile(path string) (*Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ggui: load font: %w", err)
	}
	if strings.HasPrefix(string(data[:min(4, len(data))]), "ttcf") {
		fonts, err := LoadFontCollection(data)
		if err != nil {
			return nil, err
		}
		if len(fonts) == 0 {
			return nil, fmt.Errorf("ggui: load font: %s holds no faces", path)
		}
		return fonts[0], nil
	}
	return LoadFont(data)
}

// Fallback appends fonts to draw the glyphs f lacks, in order of
// preference. A Font with none set falls back to SystemFonts.
func (f *Font) Fallback(fonts ...*Font) *Font {
	f.fallbacks = append(f.fallbacks, fonts...)
	f.faces = nil
	fontGeneration++
	requestLayout()
	return f
}

// NoFallback draws only f's own glyphs, with no system fonts behind it.
func (f *Font) NoFallback() *Font {
	f.noFallback, f.faces = true, nil
	fontGeneration++
	requestLayout()
	return f
}

var (
	systemOnce  sync.Once
	systemFonts []*Font
)

// SystemFonts returns the fonts found on this machine that cover scripts
// the built-in font does not, mainly CJK, loaded once from well-known
// paths per platform. It is what every Font falls back to; call it to
// see what was found, and Fallback to choose differently. The GGUI_FONTS
// environment variable lists extra files, separated like PATH, tried
// first.
func SystemFonts() []*Font {
	systemOnce.Do(func() {
		seen := map[string]bool{}
		for _, path := range systemFontPaths() {
			if seen[path] {
				continue
			}
			seen[path] = true
			if f, err := LoadFontFile(path); err == nil {
				f.noFallback = true
				systemFonts = append(systemFonts, f)
			}
		}
	})
	return systemFonts
}

// systemFontPaths lists candidate files, most specific first: one per
// script where the platform has them, then pan-Unicode fonts.
func systemFontPaths() []string {
	paths := filepath.SplitList(os.Getenv("GGUI_FONTS"))
	switch runtime.GOOS {
	case "darwin":
		paths = append(paths,
			"/System/Library/Fonts/AppleSDGothicNeo.ttc",           // Korean
			"/System/Library/Fonts/ヒラギノ角ゴシック W3.ttc",               // Japanese
			"/System/Library/Fonts/Hiragino Sans GB.ttc",           // Simplified Chinese
			"/System/Library/Fonts/PingFang.ttc",                   // Chinese
			"/System/Library/Fonts/Supplemental/AppleGothic.ttf",   // Korean
			"/System/Library/Fonts/Supplemental/Arial Unicode.ttf", // wide coverage
			"/System/Library/Fonts/Supplemental/NotoSansGothic-Regular.ttf",
		)
	case "windows":
		dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
		for _, name := range []string{"malgun.ttf", "meiryo.ttc", "YuGothM.ttc", "msyh.ttc", "msjh.ttc", "simsun.ttc", "segoeui.ttf"} {
			paths = append(paths, filepath.Join(dir, name))
		}
	default:
		for _, dir := range []string{"/usr/share/fonts", "/usr/local/share/fonts", filepath.Join(os.Getenv("HOME"), ".local/share/fonts"), filepath.Join(os.Getenv("HOME"), ".fonts")} {
			for _, pattern := range []string{
				"**/NotoSansCJK*-Regular.ttc", "**/NotoSansCJK*-Regular.otf", "**/NotoSansKR-Regular.*", "**/NotoSansJP-Regular.*", "**/NotoSansSC-Regular.*",
				"**/NanumGothic.ttf", "**/DroidSansFallback*.ttf", "**/wqy-microhei.ttc", "**/wqy-zenhei.ttc",
			} {
				paths = append(paths, findFonts(dir, pattern)...)
			}
		}
	}
	return paths
}

// findFonts walks dir for files matching the base of pattern, which may
// start with "**/" to search every subdirectory.
func findFonts(dir, pattern string) []string {
	base := strings.TrimPrefix(pattern, "**/")
	var out []string
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if ok, _ := filepath.Match(base, d.Name()); ok {
			out = append(out, path)
		}
		return nil
	})
	return out
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

var defaultFont *Font

// SetDefaultFont replaces the font Text uses when none is set. The built-in
// default is Go Regular, which covers Latin, Greek and Cyrillic; load a font
// with the glyphs you need for anything else.
func SetDefaultFont(f *Font) {
	defaultFont = f
	fontGeneration++
	requestLayout()
}

// fallbackFont returns the font Text uses when none is set, parsing the
// built-in one on first use so a program that draws no text never pays for it.
func fallbackFont() *Font {
	if defaultFont == nil {
		defaultFont = MustFont(goregular.TTF)
	}
	return defaultFont
}

func (f *Font) face(size float64) text.Face {
	if f.generation != fontGeneration {
		f.faces = nil
		f.generation = fontGeneration
	}
	if face, ok := f.faces[size]; ok {
		return face
	}
	var face text.Face = &text.GoTextFace{Source: f.src, Size: size}
	fallbacks := f.fallbacks
	if fallbacks == nil && !f.noFallback {
		fallbacks = SystemFonts()
	}
	if len(fallbacks) > 0 {
		faces := []text.Face{face}
		for _, fb := range fallbacks {
			if fb != f {
				faces = append(faces, &text.GoTextFace{Source: fb.src, Size: size})
			}
		}
		if m, err := text.NewMultiFace(faces...); err == nil {
			face = m
		}
	}
	if !f.noFallback {
		face = withEmoji(face, size)
	}
	if f.faces == nil {
		f.faces = make(map[float64]text.Face)
	}
	f.faces[size] = face
	return face
}

func lineWidth(s string, face text.Face) float64 {
	width := 0.0
	textRuns(s, face, func(run string, f text.Face) { width += text.Advance(run, f) })
	return width
}

// wrapText breaks s into lines no wider than maxW. Hard line breaks are kept;
// soft breaks fall on spaces, or between grapheme clusters when a single word is wider
// than the line, which is what scripts written without spaces need.
func wrapText(s string, face text.Face, maxW float64) []string {
	var out []string
	for para := range strings.SplitSeq(s, "\n") {
		out = append(out, wrapLine(para, face, maxW)...)
	}
	return out
}

func wrapLine(line string, face text.Face, maxW float64) []string {
	if maxW <= 0 || lineWidth(line, face) <= maxW {
		return []string{line}
	}
	var lines []string
	cur := ""
	for word := range strings.FieldsSeq(line) {
		cand := word
		if cur != "" {
			cand = cur + " " + word
		}
		if lineWidth(cand, face) <= maxW {
			cur = cand
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		pieces := breakRunes(word, face, maxW)
		lines = append(lines, pieces[:len(pieces)-1]...)
		cur = pieces[len(pieces)-1]
	}
	if cur != "" || len(lines) == 0 {
		lines = append(lines, cur)
	}
	return lines
}

// breakRunes splits word into pieces no wider than maxW, never emptier than
// one grapheme so progress is guaranteed.
func breakRunes(word string, face text.Face, maxW float64) []string {
	var pieces []string
	start := 0
	for i := 0; i < len(word); {
		end := nextGrapheme(word, i)
		if i > start && lineWidth(word[start:end], face) > maxW {
			pieces = append(pieces, word[start:i])
			start = i
		}
		i = end
	}
	return append(pieces, word[start:])
}

// lineSpan is one drawn line of a wrapped string as byte offsets: the text
// drawn is s[start:end]. A hard line break or the spaces a soft break fell
// on sit between one span's end and the next one's start.
type lineSpan struct{ start, end int }

// wrapSpans is wrapText for an editor: the same breaks, as offsets into s
// rather than copies, so a caret position maps to a line and back.
func wrapSpans(s string, face text.Face, maxW float64) []lineSpan {
	var out []lineSpan
	start := 0
	for {
		stop := len(s)
		if i := strings.IndexByte(s[start:], '\n'); i >= 0 {
			stop = start + i
		}
		for _, sp := range wrapParagraph(s[start:stop], face, maxW) {
			out = append(out, lineSpan{start + sp.start, start + sp.end})
		}
		if stop == len(s) {
			return out
		}
		start = stop + 1
	}
}

// wrapParagraph wraps text without hard breaks: greedily by words, and
// between grapheme clusters when a word alone is wider than maxW.
func wrapParagraph(p string, face text.Face, maxW float64) []lineSpan {
	if maxW <= 0 || lineWidth(p, face) <= maxW {
		return []lineSpan{{0, len(p)}}
	}
	var lines []lineSpan
	lineStart, lineEnd := 0, 0 // the line being filled: its first byte and the end of its last word
	i := 0
	for i < len(p) {
		for i < len(p) && (p[i] == ' ' || p[i] == '\t') {
			i++
		}
		if i == len(p) {
			break
		}
		wstart := i
		for i < len(p) && p[i] != ' ' && p[i] != '\t' {
			i++
		}
		wend := i
		if lineEnd > lineStart && lineWidth(p[lineStart:wend], face) > maxW {
			// Does not fit after what is on the line: break before it.
			lines = append(lines, lineSpan{lineStart, lineEnd})
			lineStart, lineEnd = wstart, wstart
		}
		if lineWidth(p[lineStart:wend], face) <= maxW {
			lineEnd = wend
			continue
		}
		// A word wider than the line, alone on it: break between grapheme clusters.
		for j := lineStart; j < wend; {
			k := nextGrapheme(p, j)
			if k < wend && lineWidth(p[lineStart:nextGrapheme(p, k)], face) > maxW {
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

// lineOf returns the index of the span that holds byte offset pos: the
// last one starting at or before it.
func lineOf(spans []lineSpan, pos int) int {
	for i := len(spans) - 1; i > 0; i-- {
		if spans[i].start <= pos {
			return i
		}
	}
	return 0
}
