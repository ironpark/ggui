package theme_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestThemeDoesNotEmbedFontPacks keeps ui/theme free of the embedded font
// packages (fonts/inter, fonts/geist, fonts/gofont): applications opt in via
// theme.RegisterFont rather than carrying several megabytes unconditionally.
func TestThemeDoesNotEmbedFontPacks(t *testing.T) {
	if testing.Short() {
		t.Skip("lists the package's dependencies")
	}
	t.Parallel()
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for dep := range strings.FieldsSeq(string(out)) {
		if strings.HasPrefix(dep, "github.com/ironpark/ggui/fonts/") {
			t.Errorf("ui/theme depends on %s", dep)
		}
	}
}
