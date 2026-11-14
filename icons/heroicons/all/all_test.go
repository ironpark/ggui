package all_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/ironpark/ggui/icons/heroicons"
	"github.com/ironpark/ggui/icons/heroicons/all"
	"github.com/ironpark/ggui/ui/icons"
)

func TestEveryIconParsesAndIsFoundByName(t *testing.T) {
	t.Parallel()
	glyphs := all.Glyphs()
	if !sort.SliceIsSorted(glyphs, func(i, j int) bool { return glyphs[i].Name < glyphs[j].Name }) {
		t.Fatal("the icons are not in name order")
	}
	for _, g := range glyphs {
		if _, err := g.Parse(); err != nil {
			t.Fatal(err)
		}
		if all.Lookup(g.Name) != g {
			t.Fatalf("%s is not found by its name", g.Name)
		}
	}
	if all.Lookup("no such icon") != nil || len(all.Names()) != len(glyphs) {
		t.Fatal("a name that is no icon's was found, or a name is missing")
	}
	if g := all.Lookup(heroicons.ArrowDownTray.Name); g != heroicons.ArrowDownTray {
		t.Fatalf("looking %s up gives %v", heroicons.ArrowDownTray.Name, g)
	}
}

func TestSetHasEveryRole(t *testing.T) {
	t.Parallel()
	for _, role := range []icons.Role{
		icons.Check, icons.Close, icons.ChevronDown, icons.ChevronUp, icons.ChevronLeft, icons.ChevronRight,
		icons.Search, icons.Alert, icons.Info, icons.Plus, icons.Minus, icons.Download, icons.File, icons.Sun,
		icons.Moon, icons.Grip, icons.Loader, icons.Undo, icons.Redo, icons.MoreHorizontal,
	} {
		if heroicons.Set().Resolve(role) == nil {
			t.Fatalf("no icon for %s", role)
		}
	}
	if heroicons.Set().Resolve("no such role") != nil {
		t.Fatal("an icon for a role that is none")
	}
}

// A program that names one icon is linked with that icon's source and not
// the library's: the point of a variable for each.
func TestAProgramHoldsOnlyTheIconsItNames(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	t.Parallel()
	bin := filepath.Join(t.TempDir(), "one")
	if out, err := exec.Command("go", "build", "-o", bin, "../internal/one").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	body := func(g *icons.Glyph) []byte { return []byte(g.Data) }
	if !bytes.Contains(data, body(heroicons.ArrowDownTray)) {
		t.Fatal("the icon the program names is not in it")
	}
	// A sample of the rest: searching the program for every icon takes a
	// minute, and a library that was linked whole would hold them all. One
	// of them may be drawn as the named icon is.
	held, looked := 0, 0
	for i, g := range all.Glyphs() {
		if i%40 != 0 {
			continue
		}
		looked++
		if bytes.Contains(data, body(g)) {
			held++
		}
	}
	if held > 1 {
		t.Fatalf("a program naming one icon holds %d of the %d others looked for", held, looked)
	}
}
