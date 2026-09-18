package ggui

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

var emojiFont *Font
var emojiFontSet bool
var fontGeneration uint64

// SetEmojiFont selects a color emoji font for Text and TextInput. It does not
// replace their Latin/CJK fonts. Use fonts/notoemoji for a portable embedded font,
// or LoadFont for another CBDT, sbix, COLRv0 or OpenType SVG font. Nil disables
// emoji substitution; SetEmojiFont(SystemEmojiFont()) restores system rendering.
// Like SetTheme, call on the UI thread or before creating the app.
func SetEmojiFont(f *Font) {
	emojiFont, emojiFontSet = f, true
	fontGeneration++
	requestLayout()
}

// SystemEmojiFont finds the platform color emoji font once, or returns nil.
// Browsers have no filesystem font access; use an embedded font on the web.
func SystemEmojiFont() *Font { return systemEmoji() }

var systemEmoji = sync.OnceValue(func() *Font {
	var paths []string
	switch runtime.GOOS {
	case "darwin":
		paths = []string{"/System/Library/Fonts/Apple Color Emoji.ttc"}
	case "windows":
		paths = []string{filepath.Join(os.Getenv("WINDIR"), "Fonts", "seguiemj.ttf")}
	case "js", "ios", "android":
		return nil
	default:
		for _, dir := range []string{"/usr/share/fonts", "/usr/local/share/fonts", filepath.Join(os.Getenv("HOME"), ".local/share/fonts")} {
			paths = append(paths, findFonts(dir, "**/NotoColorEmoji.ttf")...)
			paths = append(paths, findFonts(dir, "**/Twemoji*.ttf")...)
		}
	}
	for _, path := range paths {
		if f, err := LoadFontFile(path); err == nil {
			return f
		}
	}
	return nil
})

// emojiFace deliberately keeps the text metrics. Emoji are fitted into that
// line box and aligned on its baseline. Its Face is the ordinary text fallback
// chain; drawText/lineWidth select an emoji font for an entire grapheme, avoiding
// MultiFace's per-rune split of ZWJ, variation-selector and keycap sequences.
type emojiFace struct {
	text.Face
	emoji text.Face
}

func withEmoji(face text.Face, size float64) text.Face {
	f := emojiFont
	if !emojiFontSet {
		f = SystemEmojiFont()
	}
	if f == nil {
		return face
	}
	ef := &text.GoTextFace{Source: f.src, Size: size}
	m, em := face.Metrics(), ef.Metrics()
	if h := em.HAscent + em.HDescent; h > 0 {
		ef.Size *= (m.HAscent + m.HDescent) / h
	}
	return &emojiFace{Face: face, emoji: ef}
}

func inEmojiRanges(r rune, ranges [][2]rune) bool {
	_, ok := slices.BinarySearchFunc(ranges, r, func(p [2]rune, r rune) int {
		if p[1] < r {
			return -1
		}
		if p[0] > r {
			return 1
		}
		return 0
	})
	return ok
}
func emojiCluster(s string) bool {
	if strings.ContainsRune(s, '\ufe0e') {
		return false
	} // explicit text presentation
	r, _ := utf8.DecodeRuneInString(s)
	if strings.ContainsRune(s, '\u20e3') && (r == '#' || r == '*' || r >= '0' && r <= '9') {
		return true
	}
	if strings.ContainsRune(s, '\ufe0f') {
		return inEmojiRanges(r, emojiCodepoints[:])
	}
	return inEmojiRanges(r, emojiPresentation[:])
}

// Keep adjacent text in a single shaping run, so kerning and script ligatures
// are preserved. A whole emoji grapheme always goes to the same font.
func textRuns(s string, face text.Face, visit func(string, text.Face)) {
	ef, ok := face.(*emojiFace)
	if !ok {
		visit(s, face)
		return
	}
	start := 0
	var current text.Face
	for i := 0; i < len(s); {
		end := nextGrapheme(s, i)
		chosen := ef.Face
		if emojiCluster(s[i:end]) {
			chosen = ef.emoji
		}
		if current != nil && chosen != current {
			visit(s[start:i], current)
			start = i
		}
		current = chosen
		i = end
	}
	if current != nil {
		visit(s[start:], current)
	}
}

func drawText(dst *ebiten.Image, s string, face text.Face, options *text.DrawOptions) {
	if _, ok := face.(*emojiFace); !ok {
		text.Draw(dst, s, face, options)
		return
	}
	x := 0.0
	baseline := face.Metrics().HAscent
	textRuns(s, face, func(run string, f text.Face) {
		op := text.DrawOptions{}
		if options != nil {
			op = *options
		}
		var local ebiten.GeoM
		local.Translate(x, baseline-f.Metrics().HAscent)
		local.Concat(op.GeoM)
		op.GeoM = local
		text.Draw(dst, run, f, &op)
		x += text.Advance(run, f)
	})
}
