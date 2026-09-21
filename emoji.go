package ggui

import (
	"github.com/ironpark/ggui/internal/reactive"
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/ironpark/ggui/internal/emojidata"
)

// The font state below belongs to the UI goroutine: every entry point that
// writes it runs under checkUIThread, and fontGeneration is read from
// CachedWidget.Layout to decide whether a measurement still holds. A font
// loaded on another goroutine is handed over with App.Post.
var emojiFont *Font
var emojiFontSet bool
var fontGeneration uint64

// SetEmojiFont selects a color emoji font for Text and TextInput. It does not
// replace their Latin/CJK fonts. Use fonts/notoemoji for a portable embedded font,
// or LoadFont for another CBDT, sbix, COLRv0 or OpenType SVG font. Nil disables
// emoji substitution; SetEmojiFont(SystemEmojiFont()) restores system rendering.
// Like SetTheme, call on the UI thread or before creating the app.
func SetEmojiFont(f *Font) {
	reactive.CheckUIThread("SetEmojiFont")
	emojiFont, emojiFontSet = f, true
	fontGeneration++
	reactive.RequestLayout()
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
	size     float64
	emoji    text.Face
	resolved bool
}

func withEmoji(face text.Face, size float64) text.Face {
	if emojiFontSet && emojiFont == nil {
		return face
	}
	return &emojiFace{Face: face, size: size}
}

// colorFace resolves the emoji font the first time a grapheme needs it, so
// text without emoji never loads or parses the (large) system emoji font.
func (ef *emojiFace) colorFace() text.Face {
	if ef.resolved {
		return ef.emoji
	}
	ef.resolved = true
	f := emojiFont
	if !emojiFontSet {
		f = SystemEmojiFont()
	}
	if f == nil {
		return nil
	}
	e := &text.GoTextFace{Source: f.src, Size: ef.size}
	m, em := ef.Face.Metrics(), e.Metrics()
	if h := em.HAscent + em.HDescent; h > 0 {
		e.Size *= (m.HAscent + m.HDescent) / h
	}
	ef.emoji = e
	return e
}

// Keep adjacent text in a single shaping run, so kerning and script ligatures
// are preserved. A whole emoji grapheme always goes to the same font.
func textRuns(s string, face text.Face) iter.Seq2[string, text.Face] {
	return func(yield func(string, text.Face) bool) {
		ef, ok := face.(*emojiFace)
		if !ok || !emojidata.MayHold(s) {
			yield(s, face)
			return
		}
		start := 0
		var current text.Face
		for i := 0; i < len(s); {
			end := nextGrapheme(s, i)
			chosen := ef.Face
			if emojidata.IsCluster(s[i:end]) {
				if e := ef.colorFace(); e != nil {
					chosen = e
				}
			}
			if current != nil && chosen != current {
				if !yield(s[start:i], current) {
					return
				}
				start = i
			}
			current = chosen
			i = end
		}
		if current != nil {
			yield(s[start:], current)
		}
	}
}

func drawText(dst *ebiten.Image, s string, face text.Face, options *text.DrawOptions) {
	if dst == nil || dst.Bounds().Empty() {
		return
	}
	ef, ok := face.(*emojiFace)
	if !ok {
		text.Draw(dst, s, face, options)
		return
	}
	// Text with no emoji in it is one run in the fallback chain the wrapper
	// holds, drawn where it was asked for. Saying so here skips the line
	// metrics and the advance that only matter when runs have to be placed
	// one after another, and most text a frame draws is this.
	if !emojidata.MayHold(s) {
		text.Draw(dst, s, ef.Face, options)
		return
	}
	x := 0.0
	baseline := face.Metrics().HAscent
	for run, f := range textRuns(s, face) {
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
	}
}
