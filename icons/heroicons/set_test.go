package heroicons

import (
	"testing"

	"github.com/ironpark/ggui/ui/icons"
)

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

func TestSetIsComparable(t *testing.T) {
	t.Parallel()
	if Set() != Set() {
		t.Fatal("Set() != Set()")
	}
}
