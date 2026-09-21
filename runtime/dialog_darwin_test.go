//go:build darwin && !ios

package runtime

import (
	"slices"
	"testing"
)

func TestExtensionsFlattenAndUnrestrict(t *testing.T) {
	exts, restricted := extensions([]FileFilter{{Name: "Images", Extensions: []string{"png", "jpg"}}, {Name: "Text", Extensions: []string{"txt"}}})
	if !restricted || !slices.Equal(exts, []string{"png", "jpg", "txt"}) {
		t.Fatalf("extensions = %q, %v", exts, restricted)
	}
	if _, restricted := extensions([]FileFilter{{Name: "Images", Extensions: []string{"png"}}, {Name: "All files"}}); restricted {
		t.Fatal("a filter without extensions should lift the restriction")
	}
	if _, restricted := extensions(nil); restricted {
		t.Fatal("no filters should mean no restriction")
	}
}
