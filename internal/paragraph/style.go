// Package paragraph is ggui's text layout engine: a line cut into pieces
// by direction and style and ordered by the Unicode Bidirectional
// Algorithm, each right-to-left piece shaped by its fallback chain in
// turn; a paragraph of styled runs wrapped, cut and spaced into such
// lines; offsets and carets mapped to x and back; and text drawn on the
// CPU with go-text alone.
//
// It takes its styles resolved: a font, size and weight, and the emoji
// font chosen for them. What resolves them, a widget's style, its Env and
// the world's defaults, is ggui's.
package paragraph

import (
	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/text"
)

// Style is what text is set in: a font at a size and weight, LineHeight
// times the size between baselines, and the emoji font chosen beside it,
// nil for the system's.
type Style struct {
	Font       *fontsys.Font
	Size       float64
	Weight     fontsys.Weight
	LineHeight float64
	Emoji      *fontsys.EmojiChoice
}

// Face is the style's face at scale times its size.
func (s Style) Face(scale float64) text.Face {
	return s.Font.FaceFor(s.Size*scale, s.Weight, s.Emoji)
}
