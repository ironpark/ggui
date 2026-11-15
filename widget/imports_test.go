package widget

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/ironpark/ggui/internal/gfxtest"
)

// The tests paint with no renderer: see gfxtest.MainHeadless.
func TestMain(m *testing.M) { gfxtest.MainHeadless(m) }

// TestWidgetStaysBelowTheDesignSystem keeps the unstyled widgets free of
// the themed library and what it embeds: ui imports widget, never the
// other way round, and an editor alone carries no theme, icons or fonts.
func TestWidgetStaysBelowTheDesignSystem(t *testing.T) {
	if testing.Short() {
		t.Skip("lists the package's dependencies")
	}
	t.Parallel()
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		ui := dep == "github.com/ironpark/ggui/ui" || strings.HasPrefix(dep, "github.com/ironpark/ggui/ui/")
		if ui || strings.HasPrefix(dep, "github.com/ironpark/ggui/fonts/") {
			t.Errorf("widget depends on %s", dep)
		}
	}
}
