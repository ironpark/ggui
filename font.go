package ggui

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"
)

// Font is a loaded TrueType or OpenType face, usable at any size.
type Font struct {
	src   *text.GoTextFaceSource
	faces map[float64]text.Face // one face per size, reused across frames
}

// LoadFont parses TTF or OTF bytes.
func LoadFont(data []byte) (*Font, error) {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ggui: load font: %w", err)
	}
	return &Font{src: src}, nil
}

// LoadFontFile parses the TTF or OTF file at path.
func LoadFontFile(path string) (*Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ggui: load font: %w", err)
	}
	return LoadFont(data)
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
func SetDefaultFont(f *Font) { defaultFont = f }

// fallbackFont returns the font Text uses when none is set, parsing the
// built-in one on first use so a program that draws no text never pays for it.
func fallbackFont() *Font {
	if defaultFont == nil {
		defaultFont = MustFont(goregular.TTF)
	}
	return defaultFont
}

func (f *Font) face(size float64) text.Face {
	if face, ok := f.faces[size]; ok {
		return face
	}
	face := &text.GoTextFace{Source: f.src, Size: size}
	if f.faces == nil {
		f.faces = make(map[float64]text.Face)
	}
	f.faces[size] = face
	return face
}

func lineWidth(s string, face text.Face) float64 { return text.Advance(s, face) }

// wrapText breaks s into lines no wider than maxW. Hard line breaks are kept;
// soft breaks fall on spaces, or between runes when a single word is wider
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
	for _, word := range strings.Fields(line) {
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
// one rune so progress is guaranteed.
func breakRunes(word string, face text.Face, maxW float64) []string {
	var pieces []string
	var cur []rune
	for _, r := range word {
		cand := append(cur, r)
		if len(cur) > 0 && lineWidth(string(cand), face) > maxW {
			pieces = append(pieces, string(cur))
			cur = []rune{r}
			continue
		}
		cur = cand
	}
	return append(pieces, string(cur))
}
