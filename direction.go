package ggui

import "github.com/ironpark/ggui/pkg/layout"

// TextDirection is the direction a subtree reads in: left to right, as
// English does, or right to left, as Arabic and Hebrew do. The zero value
// is DirLTR.
type TextDirection = layout.Direction

const (
	DirLTR = layout.LTR
	DirRTL = layout.RTL
)

// DirectionKey holds the direction of the subtree below it; Directionality
// provides it and Env.Direction reads it, DirLTR by default.
var DirectionKey = NewEnvKey[TextDirection]("text direction")

// LanguageKey holds the BCP 47 language of the subtree below it, such as
// "ar" or "he-IL", which text hands the shaper with each right-to-left
// run so that it picks the language's forms. Env.Language reads it.
var LanguageKey = NewEnvKey[string]("language")

// Direction returns the direction the tree under e reads in.
func (e Env) Direction() TextDirection {
	d, _ := e.Get(DirectionKey)
	return d
}

// Language returns the BCP 47 language set for the tree under e, or "".
func (e Env) Language() string {
	l, _ := e.Get(LanguageKey)
	return l
}

// Directionality lays child out reading in dir. Under DirRTL a paragraph
// orders its runs right to left, text starts at the right edge, a Row
// places its first child rightmost and a Column puts AlignStart children
// on the right; Align's Left and Right, and an explicit Text.Align, stay
// where they say.
//
//	ggui.Directionality(ggui.DirRTL, ggui.Text("שלום עולם"))
func Directionality(dir TextDirection, child Widget) *EnvWidget {
	return Provide(DirectionKey, dir, child)
}

// startAlign is where a line of text starts by default: the left edge, or
// the right one under DirRTL.
func startAlign(d TextDirection) float64 {
	if d == DirRTL {
		return 1
	}
	return 0
}
