package heroicons

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/ironpark/ggui/ui/icons"
)

func TestBundledSVGsAndRoles(t *testing.T) {
	names, err := fs.Glob(files, "svg/*.svg")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range names {
		name := strings.TrimSuffix(strings.TrimPrefix(path, "svg/"), ".svg")
		a, err := Asset(name)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Asset(name)
		if err != nil || a != b {
			t.Fatalf("%s was not cached", name)
		}
	}
	for role := range roles {
		if Set().Resolve(role) == nil {
			t.Fatalf("missing role %s", role)
		}
	}
	if Set().Resolve(icons.Role("unknown")) != nil {
		t.Fatal("unknown role should fall through")
	}
	if _, err := Asset("unknown"); err == nil {
		t.Fatal("unknown asset should return an error")
	}
}
