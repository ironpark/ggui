package chat_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/gfxtest"
)

// The tests paint with no renderer: see gfxtest.MainHeadless.
func TestMain(m *testing.M) { gfxtest.MainHeadless(m) }

func node(t *testing.T, tree *ggui.SemTree, role ggui.Role, name string) ggui.SemNode {
	t.Helper()
	n, ok := tree.Find(role, name)
	if !ok {
		t.Fatalf("no %s named %q in\n%s", role, name, tree)
	}
	return n
}
