package ggui

import (
	"bytes"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui/internal/keygen"
)

func TestKeyConstantsAreCurrent(t *testing.T) {
	want, err := keygen.Generate()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("keys_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("keys_gen.go does not match what Ebitengine reports; run go generate ./...")
	}
}

func TestKeyAliasesAreTheSameValues(t *testing.T) {
	if KeyArrowUp != ebiten.KeyArrowUp || KeyEnter != ebiten.KeyEnter || KeyMax != ebiten.KeyMax {
		t.Fatal("a ggui key constant differs from the Ebitengine one it aliases")
	}
	if CursorShapePointer != ebiten.CursorShapePointer || MouseButtonRight != ebiten.MouseButtonRight {
		t.Fatal("a cursor or button constant differs from the Ebitengine one it aliases")
	}
	// Chord parses the same names the constants are generated from, so
	// every name ParseChord accepts has a constant to go with it.
	if c := MustChord("ctrl+f7"); c.Key != KeyF7 {
		t.Fatalf("ParseChord read f7 as %v, want KeyF7", c.Key)
	}
}
