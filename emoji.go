package ggui

import (
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggfx/text/v2"
	"github.com/ironpark/ggui/internal/emojidata"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/textedit"
)

// fontGeneration counts the changes to a Font itself, Fallback and
// NoFallback, which reach every world that measures with it. A Font is
// shared by probes on goroutines of their own, so it is atomic. What a
// world chose with SetDefaultFont and SetEmojiFont is counted in the world;
// see fontGen.
var fontGeneration atomic.Uint64

// fontGen changes whenever a measurement made in the active world with the
// fonts it resolves may no longer hold: a Font changed, or the active world
// or one it inherits from chose another default or emoji font. Every part
// only grows, so their sum changes whenever one does. CachedWidget and
// Text read it to decide whether a measurement still holds.
func fontGen() uint64 {
	g := fontGeneration.Load()
	for w := range activeWorld().chain() {
		g += w.fontGen
	}
	return g
}

// emojiChoice is what SetEmojiFont chose, nil font included; a nil
// *emojiChoice means it was never called and the system font is used.
type emojiChoice struct{ font *Font }

// activeEmoji is the active world's emoji choice.
func activeEmoji() *emojiChoice {
	for w := range activeWorld().chain() {
		if w.emoji != nil {
			return w.emoji
		}
	}
	return nil
}

// SetEmojiFont selects a color emoji font for Text and TextInput. It does not
// replace their Latin/CJK fonts. Use fonts/notoemoji for a portable embedded font,
// or LoadFont for another CBDT, sbix, COLRv0 or OpenType SVG font. Nil disables
// emoji substitution; SetEmojiFont(SystemEmojiFont()) restores system rendering.
// Like SetEnv, call on the UI thread or before creating the app: it sets the
// emoji font of the App or Probe whose frames run on this goroutine, or,
// before any has, the one every App and Probe made on it uses until given
// its own.
func SetEmojiFont(f *Font) {
	reactive.CheckUIThread("SetEmojiFont")
	setEmoji(activeWorld(), &emojiChoice{font: f})
}

// setEmoji makes c w's emoji choice, nil to inherit its parent's.
func setEmoji(w *world, c *emojiChoice) {
	w.emoji = c
	w.fontGen++
}

// SystemEmojiFont finds the platform color emoji font once, or returns nil.
// Browsers have no filesystem font access; load a remote or embedded font there.
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
	choice   *emojiChoice // the emoji font to resolve; nil for the system's
	emoji    text.Face
	resolved sync.Once // a face is cached per Font, size and choice, so shared
}

func withEmoji(face text.Face, size float64, choice *emojiChoice) text.Face {
	if choice != nil && choice.font == nil {
		return face
	}
	return &emojiFace{Face: face, size: size, choice: choice}
}

// colorFace resolves the emoji font the first time a grapheme needs it, so
// text without emoji never loads or parses the (large) system emoji font.
func (ef *emojiFace) colorFace() text.Face {
	ef.resolved.Do(func() {
		var f *Font
		if ef.choice != nil {
			f = ef.choice.font
		} else {
			f = SystemEmojiFont()
		}
		if f == nil {
			return
		}
		e := &text.GoTextFace{Source: f.source(), Size: ef.size}
		m, em := ef.Face.Metrics(), e.Metrics()
		if h := em.HAscent + em.HDescent; h > 0 {
			e.Size *= (m.HAscent + m.HDescent) / h
		}
		ef.emoji = e
	})
	return ef.emoji
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
			end := textedit.NextGrapheme(s, i)
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

func drawText(dst *ggfx.Image, s string, face text.Face, options *text.DrawOptions) {
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
		var local ggfx.GeoM
		local.Translate(x, baseline-f.Metrics().HAscent)
		local.Concat(op.GeoM)
		op.GeoM = local
		text.Draw(dst, run, f, &op)
		x += text.AdvanceAt(run, len(run), f)
	}
}
