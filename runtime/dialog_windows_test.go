//go:build windows

package runtime

import (
	"slices"
	"testing"

	"golang.org/x/sys/windows"
)

func TestSplitzSingleAndMultiple(t *testing.T) {
	single := utf16z(`C:\a\b.txt`)
	if got := splitz(single); !slices.Equal(got, []string{`C:\a\b.txt`}) {
		t.Fatalf("splitz(single) = %q", got)
	}
	var multi []uint16
	for _, s := range []string{`C:\dir`, "x.txt", "y.txt"} {
		multi = append(multi, utf16z(s)...)
	}
	multi = append(multi, 0)
	if got := splitz(multi); !slices.Equal(got, []string{`C:\dir`, "x.txt", "y.txt"}) {
		t.Fatalf("splitz(multi) = %q", got)
	}
}

func TestFilterSpec(t *testing.T) {
	spec := filterSpec([]FileFilter{{Name: "Images", Extensions: []string{"png", "jpg"}}, {Name: "All"}})
	parts := splitz(spec)
	want := []string{"Images (*.png;*.jpg)", "*.png;*.jpg", "All (*.*)", "*.*"}
	if !slices.Equal(parts, want) {
		t.Fatalf("filterSpec = %q, want %q", parts, want)
	}
	if spec[len(spec)-1] != 0 || spec[len(spec)-2] != 0 {
		t.Fatal("filterSpec is not double-terminated")
	}
	if filterSpec(nil) != nil {
		t.Fatal("filterSpec(nil) should be nil")
	}
}

func TestUTF16Ptr(t *testing.T) {
	if utf16ptr("") != nil {
		t.Fatal("empty string should be a nil pointer")
	}
	if got := windows.UTF16PtrToString(utf16ptr("hi")); got != "hi" {
		t.Fatalf("utf16ptr round trip = %q", got)
	}
}
