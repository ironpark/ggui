package ggui

import (
	"os/exec"
	"runtime"
	"testing"
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
