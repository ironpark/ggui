package ggui

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// TestEnginePackagesStayBelowTheUI keeps the engines ggui is built on free
// of what ggui itself is: widgets, the reactive runtime and the frame. An
// engine that needs one of them has its boundary in the wrong place, and
// loses the benchmarks and tests that run it alone.
func TestEnginePackagesStayBelowTheUI(t *testing.T) {
	if testing.Short() {
		t.Skip("lists the module's packages")
	}
	t.Parallel()
	// The Taskfile's bench default, ENGINES, is these with reactive, less
	// gem, which has nothing to measure.
	engines := []string{"internal/fontsys", "internal/textlayout", "internal/paragraph", "internal/imgcache", "internal/render", "internal/region", "pkg/kinetics", "pkg/virtual", "pkg/geom", "pkg/gem", "pkg/layout"}
	forbidden := []string{"github.com/ironpark/ggui", "github.com/ironpark/ggui/internal/reactive", "github.com/ironpark/ggui/internal/property"}
	for _, pkg := range engines {
		out, err := exec.Command("go", "list", "-deps", "./"+pkg).Output()
		if err != nil {
			t.Fatalf("go list %s: %v", pkg, err)
		}
		deps := strings.Fields(string(out))
		for _, f := range forbidden {
			if slices.Contains(deps, f) {
				t.Errorf("%s depends on %s", pkg, f)
			}
		}
	}
}
