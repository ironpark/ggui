package ggui

import (
	"bytes"
	"os"
	"testing"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggip"
	"github.com/ironpark/ggui/internal/tools/keygen"
)

func TestKeyConstantsAreCurrent(t *testing.T) {
	t.Parallel()
	want, err := keygen.Generate()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("keys_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("keys_gen.go does not match the keys ggip defines; run go generate ./...")
	}
}

func TestKeyAliasesAreTheSameValues(t *testing.T) {
	t.Parallel()
	if KeyArrowUp != ggip.KeyArrowUp || KeyEnter != ggip.KeyEnter || KeyMax != ggip.KeyMax {
		t.Fatal("a ggui key constant differs from the ggip one it aliases")
	}
	if CursorShapePointer != ggfx.CursorShapePointer || MouseButtonRight != ggip.MouseButtonRight {
		t.Fatal("a cursor or button constant differs from the ggfx one it aliases")
	}
	// Chord parses the same names the constants are generated from, so
	// every name ParseChord accepts has a constant to go with it.
	if c := MustChord("ctrl+f7"); c.Key != KeyF7 {
		t.Fatalf("ParseChord read f7 as %v, want KeyF7", c.Key)
	}
}
