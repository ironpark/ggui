package ui

import (
	"testing"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestKbdIsAtLeastSquare(t *testing.T) {
	t.Parallel()
	th := uitheme.Default()
	env := th.Apply(ggui.Env{})
	c := ggui.Loose(ggui.Sz(400, 200))
	one := Kbd("K").Layout(c, env)
	if one != ggui.Sz(20, 20) {
		t.Fatalf("one key: %v, want 20 square", one)
	}
	word := Kbd("Enter").Layout(c, env)
	if word.H != 20 || word.W <= 20 {
		t.Fatalf("a word: %v", word)
	}
	if got := KbdGroup(Kbd("K"), Kbd("K")).Layout(c, env); got != ggui.Sz(44, 20) {
		t.Fatalf("group of two: %v, want the keys half a Space apart", got)
	}
	if got := Kbd("K").Layout(c, env.With(KbdInverseKey, true)); got != one {
		t.Fatalf("on an inverse surface: %v, want %v", got, one)
	}
}
