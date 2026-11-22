//go:build darwin && !ios

package runtime

import (
	"slices"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
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

func TestRunSheetModalHandlesSynchronousCompletion(t *testing.T) {
	// Invoke the Objective-C block synchronously inside begin; runSheetModal
	// must observe s.done and return the response without calling runModalForWindow:.
	got := runSheetModal(0, func(handler objc.Block) {
		// struct Block_literal: {isa, flags+reserved, invoke, descriptor}
		layout := *(**[3]uintptr)(unsafe.Pointer(&handler))
		purego.SyscallN(layout[2], uintptr(handler), uintptr(nsModalResponseOK))
	})
	if got != nsModalResponseOK {
		t.Fatalf("runSheetModal = %d, want %d", got, nsModalResponseOK)
	}
}
