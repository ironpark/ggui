package lucide

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ironpark/ggui/ui/icons"
)

// TestGlyphsMatchParser checks that the icons kept as parsed are what the
// parser reads from their sources now. The sources in testdata are from the
// release the package is generated from, and each draws several arcs with
// one command, which the parser once read with the first arc's radii: a
// glyph baked before a parser change like that keeps the old shape until
// the package is generated again (go run ./icons/internal/gen -lib lucide).
func TestGlyphsMatchParser(t *testing.T) {
	for file, g := range map[string]*icons.Glyph{
		"settings.svg":   Settings,
		"refresh-cw.svg": RefreshCw,
		"rotate-ccw.svg": RotateCcw,
		"heart.svg":      Heart,
		"eye.svg":        Eye,
		"globe.svg":      Globe,
	} {
		src, err := os.ReadFile(filepath.Join("testdata", file))
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := icons.Parse(src)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		want, err := parsed.MarshalBinary()
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if g.Data != string(want) {
			t.Errorf("%s: the glyph %q differs from its source as parsed now; generate the package again", file, g.Name)
		}
	}
}

func TestSetResolvesAllRoles(t *testing.T) {
	t.Parallel()
	set := Set()
	for _, role := range icons.Roles() {
		if set.Resolve(role) == nil {
			t.Fatalf("Set().Resolve(%q) = nil", role)
		}
	}
	if set.Resolve(icons.Role("unknown")) != nil {
		t.Fatal("Set().Resolve(unknown) != nil")
	}
}
