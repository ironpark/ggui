package ggui

import (
	"os/exec"
	"runtime"
	"slices"
	"testing"

	gofont "github.com/ironpark/ggui/internal/typesetting/font"

	"github.com/ironpark/ggui/internal/text"
)

// The platforms CI runs on have interface fonts to find: SF on macOS, which
// is variable and so draws every weight, Segoe UI on Windows, and on Linux
// whatever fontconfig matches, when it is installed.
func TestSystemUIFontsAreFound(t *testing.T) {
	switch runtime.GOOS {
	case "darwin", "windows":
	case "linux":
		if _, err := exec.LookPath("fc-match"); err != nil {
			t.Skip("no fontconfig")
		}
	default:
		t.Skip("no interface fonts to look for")
	}
	t.Parallel()
	sans, mono := systemUIFonts()
	if sans == nil || mono == nil {
		t.Fatalf("interface fonts %v, mono %v: want both found", sans, mono)
	}
	if runtime.GOOS == "darwin" {
		for _, f := range []*Font{sans, mono} {
			if _, axis := f.forWeight(WeightBold); !axis {
				t.Fatal("a macOS interface font has no weight axis")
			}
		}
	}
}

// A collection's other weights join the family, so the Korean fallback
// draws bold.
func TestLoadFamilyTakesACollectionsWeights(t *testing.T) {
	const path = "/System/Library/Fonts/AppleSDGothicNeo.ttc"
	if !fileExists(path) {
		t.Skip("no " + path)
	}
	t.Parallel()
	f := loadFamily([]fontFile{{path: path}})
	if f == nil || len(f.weights) == 0 {
		t.Fatal("the collection loaded no other weights")
	}
	if got, _ := f.forWeight(WeightBold); got.weight != WeightBold {
		t.Fatalf("bold drew weight %d", got.weight)
	}
}

// The symbols command-line programs print, which interface fonts lack, are
// in a system fallback on macOS: Claude Code's prompt and transcript.
func TestSystemFontsCoverTerminalSymbols(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the symbol fonts are only known to be installed on macOS")
	}
	t.Parallel()
	for _, r := range "❯⏵⏺⎿✳✻" {
		if !slices.ContainsFunc(SystemFonts(), func(f *Font) bool {
			face, ok := f.source().UnsafeInternal().(*gofont.Face)
			if !ok {
				return false
			}
			_, has := face.NominalGlyph(r)
			return has
		}) {
			t.Errorf("no system fallback has %c %U", r, r)
		}
	}
}

// The italics of the interface fonts are found and are italic, and an
// italic family keeps to italic faces.
func TestSystemItalicFontsAreItalic(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("SF's italics are only known to be installed on macOS")
	}
	t.Parallel()
	sans, mono := SystemItalicFonts()
	if sans == nil || mono == nil {
		t.Fatalf("italic interface font %v, mono %v: want both", sans, mono)
	}
	for _, f := range append([]*Font{sans, mono}, append(sans.weights, mono.weights...)...) {
		if f.source().Metadata().Style != text.StyleItalic {
			t.Fatal("an italic family holds an upright face")
		}
	}
}
