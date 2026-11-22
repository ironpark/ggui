//go:build darwin && !ios

package cocoa

import (
	"slices"
	"testing"
	"unsafe"
)

func TestStringAndGoStringRoundTrip(t *testing.T) {
	if got := GoString(0); got != "" {
		t.Fatalf("GoString(0) = %q, want empty", got)
	}
	WithAutoreleasePool(func() {
		for _, want := range []string{"", "hello", "한글 🎉"} {
			if got := GoString(String(want)); got != want {
				t.Fatalf("GoString(String(%q)) = %q", want, got)
			}
		}
	})
}

func TestStringArrayAndObjects(t *testing.T) {
	WithAutoreleasePool(func() {
		want := []string{"alpha", "beta", "gamma"}
		arr := StringArray(want)
		var got []string
		for item := range Objects(arr) {
			got = append(got, GoString(item))
		}
		if !slices.Equal(got, want) {
			t.Fatalf("Objects = %q, want %q", got, want)
		}
	})
}

func TestGeometryABILayout(t *testing.T) {
	if unsafe.Sizeof(Point{}) != 16 {
		t.Fatalf("sizeof(Point) = %d, want 16", unsafe.Sizeof(Point{}))
	}
	if unsafe.Sizeof(Size{}) != 16 {
		t.Fatalf("sizeof(Size) = %d, want 16", unsafe.Sizeof(Size{}))
	}
	if unsafe.Sizeof(Rect{}) != 32 {
		t.Fatalf("sizeof(Rect) = %d, want 32", unsafe.Sizeof(Rect{}))
	}
}
