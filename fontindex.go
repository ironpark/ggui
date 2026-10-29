package ggui

import (
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/fontscan"

	"github.com/ironpark/ggui/internal/text"
)

// systemIndex is the index of the system's fonts that go-text's fontscan
// keeps: each face's family, aspect and the runes it covers. The first scan
// reads every font file and takes seconds; the index is cached on disk, so
// later ones take a fraction of that. Finding a font in it reads no font. A
// browser has no system fonts.
var systemIndex = sync.OnceValue(func() []fontscan.Footprint {
	if runtime.GOOS == "js" {
		return nil
	}
	fps, err := fontscan.SystemFonts(quietLogger{}, "")
	if err != nil {
		return nil
	}
	return fps
})

// quietLogger drops what fontscan warns about: fonts it cannot read are
// left out of the index, which is all ggui needs to know.
type quietLogger struct{}

func (quietLogger) Printf(string, ...any) {}

// systemFamilyFiles returns the upright faces of family among the system's
// fonts, one per file, the one nearest regular weight first; nil when the
// system has no such family.
func systemFamilyFiles(family string) []fontFile {
	return familyFiles(systemIndex(), font.NormalizeFamily(family))
}

// systemFamily returns family among the system's fonts as one Font, nil
// when the system has no such family; see indexedFamily.
func systemFamily(family string) *Font {
	return indexedFamily(systemIndex(), font.NormalizeFamily(family))
}

// indexedFamily returns the upright faces of the normalized family among
// fps as one Font: the face nearest regular weight, with the faces of other
// weights as its weights; nil when fps has no such face. No font file is
// read until a face is asked about, as the index knows each face's weight.
func indexedFamily(fps []fontscan.Footprint, family string) *Font {
	var base *Font
	for _, fp := range familyFaces(fps, family) {
		w := FontWeight(fp.Aspect.Weight)
		if base != nil && (base.weight == w || slices.ContainsFunc(base.weights, func(f *Font) bool { return f.weight == w })) {
			continue
		}
		f := indexedFont(fontFile{path: fp.Location.File, index: int(fp.Location.Index)}, w)
		if base == nil {
			base = f
		} else {
			base.weights = append(base.weights, f)
		}
	}
	return base
}

// familyFaces returns the upright faces of the normalized family among
// fps, the one nearest regular weight first.
func familyFaces(fps []fontscan.Footprint, family string) []fontscan.Footprint {
	var faces []fontscan.Footprint
	for _, fp := range fps {
		if fp.Family == family && fp.Aspect.Style == font.StyleNormal {
			faces = append(faces, fp)
		}
	}
	slices.SortStableFunc(faces, func(a, b fontscan.Footprint) int {
		return weightDistance(a.Aspect.Weight) - weightDistance(b.Aspect.Weight)
	})
	return faces
}

// familyFiles returns the upright faces of the normalized family among
// fps, one per file, the one nearest regular weight first.
func familyFiles(fps []fontscan.Footprint, family string) []fontFile {
	var files []fontFile
	for _, fp := range familyFaces(fps, family) {
		// A collection is read whole, its other weights with it.
		if !slices.ContainsFunc(files, func(f fontFile) bool { return f.path == fp.Location.File }) {
			files = append(files, fontFile{path: fp.Location.File, index: int(fp.Location.Index)})
		}
	}
	return files
}

func weightDistance(w font.Weight) int {
	return int(max(w-font.WeightNormal, font.WeightNormal-w))
}

// fallbackChoices are the fonts systemFallback chose, by normalized family,
// and its answer for each rune asked about.
type fallbackChoices struct {
	sync.Mutex
	families map[string]*Font
	runes    map[rune]*Font
	runeLens map[int]int // a footprint's rune count, by its index in systemIndex
}

var systemFallbacks = fallbackChoices{families: map[string]*Font{}, runes: map[rune]*Font{}, runeLens: map[int]int{}}

// systemFallback returns a system font that has r, for text whose fonts
// lack it, or nil if none has. Of the fonts that have r it prefers an
// upright one, then one that reads as interface text (see fallbackRank),
// then the one covering the fewest runes, so that a font made for r's
// script wins over a font covering all of Unicode, which is also large to
// load. Placeholder fonts, emoji fonts, which have their own place, and
// fonts private to the platform are never chosen.
func systemFallback(r rune) *Font {
	s := &systemFallbacks
	s.Lock()
	defer s.Unlock()
	if f, ok := s.runes[r]; ok {
		return f
	}
	fps := systemIndex()
	best := -1
	for i := range fps {
		fp := &fps[i]
		// A family starting with a dot is private to macOS: its own
		// interface faces, helpers such as .Keyboard, and script faces.
		if !fp.Runes.Contains(r) || fp.Family == "lastresort" || strings.Contains(fp.Family, "emoji") || strings.HasPrefix(fp.Family, ".") {
			continue
		}
		if best < 0 || s.better(fps, i, best) {
			best = i
		}
	}
	var f *Font
	if best >= 0 {
		family := fps[best].Family
		var ok bool
		if f, ok = s.families[family]; !ok {
			if f = indexedFamily(fps, family); f != nil {
				f.noFallback = true
			}
			s.families[family] = f
		}
	}
	s.runes[r] = f
	return f
}

// better reports whether footprint i of fps makes a better fallback than
// footprint j. s must be locked.
func (s *fallbackChoices) better(fps []fontscan.Footprint, i, j int) bool {
	a, b := &fps[i], &fps[j]
	if ua, ub := a.Aspect.Style == font.StyleNormal, b.Aspect.Style == font.StyleNormal; ua != ub {
		return ua
	}
	la, lb := s.runeLen(fps, i), s.runeLen(fps, j)
	if ra, rb := fallbackRank(a.Family, la), fallbackRank(b.Family, lb); ra != rb {
		return ra > rb
	}
	if la != lb {
		return la < lb
	}
	if a.Family != b.Family {
		return a.Family < b.Family
	}
	return weightDistance(a.Aspect.Weight) < weightDistance(b.Aspect.Weight)
}

func (s *fallbackChoices) runeLen(fps []fontscan.Footprint, i int) int {
	n, ok := s.runeLens[i]
	if !ok {
		n = fps[i].Runes.Len()
		s.runeLens[i] = n
	}
	return n
}

// panUnicodeRunes is the coverage past which a font is taken to cover all
// of Unicode thinly, as Arial Unicode MS does with 38,000 runes in 22 MB:
// a font for one script, even CJK, holds fewer.
const panUnicodeRunes = 30000

// fallbackRank ranks a font of the normalized family covering runes runes
// as a fallback for interface text, higher first: the platform's own text
// family, then one made for interface text, then any other, then one whose
// name says it is serif, calligraphic, a symbol font or a variant such as a
// narrow or rounded cut, and one covering all of Unicode last. It goes by
// the name, since the index holds no font's classification.
func fallbackRank(family string, runes int) int {
	if runes > panUnicodeRunes {
		return -2
	}
	if slices.Contains(platformTextFamilies, family) {
		return 2
	}
	for _, word := range []string{
		"serif", "mincho", "myungjo", "batang", "song", "ming", "kai", "thuluth", "corsiva", "script",
		"symbol", "math", "size", "variants", "dingbat", "webdings", "wingdings", "braille", "brush",
		"hand", "chalk", "marker", "diwan", "farisi", "mishafi", "nastaliq", "chancery", "fraktur",
		"comic", "rounded", "narrow", "condensed", "compressed", "black", "mono",
	} {
		if strings.Contains(family, word) && !strings.Contains(family, "sans") {
			return -1
		}
	}
	for _, word := range []string{
		"sans", "gothic", "sangam", "kohinoor", "geeza", "arial", "helvetica", "verdana", "tahoma",
		"segoe", "dejavu", "liberation", "roboto", "hiragino", "pingfang", "thonburi", "ui",
	} {
		if strings.Contains(family, word) {
			return 1
		}
	}
	return 0
}

// platformTextFamilies are the normalized families each platform sets text
// in, which cover the scripts written with Latin letters, Greek and
// Cyrillic, and many symbols.
var platformTextFamilies = func() []string {
	var names []string
	switch runtime.GOOS {
	case "darwin":
		names = []string{"Helvetica Neue", "Helvetica", "Arial"}
	case "windows":
		names = []string{"Segoe UI", "Arial"}
	default:
		names = []string{"Noto Sans", "DejaVu Sans", "Liberation Sans"}
	}
	for i, n := range names {
		names[i] = font.NormalizeFamily(n)
	}
	return names
}()

// systemFallbackFace returns a FallbackFace that draws, at size and
// weight, what the faces before it lack with systemFallback's fonts, and
// what no font has with last.
func systemFallbackFace(size float64, weight FontWeight, last text.Face) text.Face {
	return text.NewFallbackFace(func(r rune) text.Face {
		if f := systemFallback(r); f != nil {
			return f.face(size, weight)
		}
		return nil
	}, last)
}
