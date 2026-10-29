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

// familyFiles returns the upright faces of the normalized family among
// fps, one per file, the one nearest regular weight first.
func familyFiles(fps []fontscan.Footprint, family string) []fontFile {
	var faces []fontscan.Footprint
	for _, fp := range fps {
		if fp.Family == family && fp.Aspect.Style == font.StyleNormal {
			faces = append(faces, fp)
		}
	}
	slices.SortStableFunc(faces, func(a, b fontscan.Footprint) int {
		return weightDistance(a.Aspect.Weight) - weightDistance(b.Aspect.Weight)
	})
	var files []fontFile
	for _, fp := range faces {
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
// upright one, then the one covering the fewest runes, so that a font made
// for r's script wins over a font covering all of Unicode thinly. Placeholder
// fonts and emoji fonts, which have their own place, are never chosen.
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
		if !fp.Runes.Contains(r) || fp.Family == "lastresort" || strings.Contains(fp.Family, "emoji") {
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
			if f = loadFamily(familyFiles(fps, family)); f != nil {
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
	if la, lb := s.runeLen(fps, i), s.runeLen(fps, j); la != lb {
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
