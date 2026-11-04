package fontsys

import (
	"iter"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/internal/emojidata"
	"github.com/ironpark/ggui/internal/textedit"
	"github.com/ironpark/ggui/internal/third_party/text"
)

// EmojiChoice is an emoji font chosen for text, nil Font included, which
// draws no emoji of their own. A nil *EmojiChoice chose nothing, and the
// system's emoji font is used. Faces are cached by the choice, so one
// choice stands for one setting.
type EmojiChoice struct{ Font *Font }

// SystemEmoji finds the platform color emoji font once, or returns nil.
// Browsers have no filesystem font access.
var SystemEmoji = sync.OnceValue(func() *Font {
	var paths []string
	switch runtime.GOOS {
	case "darwin":
		paths = []string{"/System/Library/Fonts/Apple Color Emoji.ttc"}
	case "windows":
		paths = []string{filepath.Join(os.Getenv("WINDIR"), "Fonts", "seguiemj.ttf")}
	case "js", "ios", "android":
		return nil
	default:
		for _, family := range []string{"Noto Color Emoji", "Twemoji", "Twemoji Mozilla"} {
			for _, file := range systemFamilyFiles(family) {
				paths = append(paths, file.path)
			}
		}
	}
	for _, path := range paths {
		if f, err := LoadFile(path); err == nil {
			return f
		}
	}
	return nil
})

// EmojiFace deliberately keeps the text metrics. Emoji are fitted into that
// line box and aligned on its baseline. Its Face is the ordinary text fallback
// chain; DrawText and TextRuns select an emoji font for an entire grapheme,
// avoiding MultiFace's per-rune split of ZWJ, variation-selector and keycap
// sequences.
type EmojiFace struct {
	text.Face
	size     float64
	choice   *EmojiChoice // the emoji font to resolve; nil for the system's
	emoji    text.Face
	resolved sync.Once // a face is cached per Font, size and choice, so shared
}

// WithEmoji puts face beside the emoji font choice resolves to, at size.
func WithEmoji(face text.Face, size float64, choice *EmojiChoice) text.Face {
	if choice != nil && choice.Font == nil {
		return face
	}
	return &EmojiFace{Face: face, size: size, choice: choice}
}

// ColorFace resolves the emoji font the first time a grapheme needs it, so
// text without emoji never loads or parses the (large) system emoji font.
// It is nil when there is no emoji font.
func (ef *EmojiFace) ColorFace() text.Face {
	ef.resolved.Do(func() {
		var f *Font
		if ef.choice != nil {
			f = ef.choice.Font
		} else {
			f = SystemEmoji()
		}
		if f == nil {
			return
		}
		e := &text.GoTextFace{Source: f.Source(), Size: ef.size}
		m, em := ef.Face.Metrics(), e.Metrics()
		if h := em.HAscent + em.HDescent; h > 0 {
			e.Size *= (m.HAscent + m.HDescent) / h
		}
		ef.emoji = e
	})
	return ef.emoji
}

// TextRuns splits s into the runs face draws with one face each. Adjacent
// text stays in a single shaping run, so kerning and script ligatures are
// preserved, and a whole emoji grapheme always goes to the same font.
func TextRuns(s string, face text.Face) iter.Seq2[string, text.Face] {
	return func(yield func(string, text.Face) bool) {
		ef, ok := face.(*EmojiFace)
		if !ok || !emojidata.MayHold(s) {
			yield(s, face)
			return
		}
		pick := func(g string) text.Face {
			if emojidata.IsCluster(g) {
				if e := ef.ColorFace(); e != nil {
					return e
				}
			}
			return ef.Face
		}
		textedit.GraphemeRuns(s, pick, func(start, end int, f text.Face) bool { return yield(s[start:end], f) })
	}
}

// PlainFace is the face s is drawn in as one run, as DrawText draws it, or
// nil when s holds an emoji, drawn in runs of faces of their own.
func PlainFace(face text.Face, s string) text.Face {
	ef, ok := face.(*EmojiFace)
	if !ok {
		return face
	}
	if emojidata.MayHold(s) {
		return nil
	}
	return ef.Face
}

// DrawText draws s in face onto dst, each emoji grapheme in the emoji font.
func DrawText(dst *ggfx.Image, s string, face text.Face, options *text.DrawOptions) {
	if dst == nil || dst.Bounds().Empty() {
		return
	}
	ef, ok := face.(*EmojiFace)
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
	for run, f := range TextRuns(s, face) {
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
