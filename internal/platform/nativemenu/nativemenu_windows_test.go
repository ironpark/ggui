//go:build windows

package nativemenu

import "testing"

func TestResolveTrayIconSize(t *testing.T) {
	// Explicit DPI (e.g., from WM_DPICHANGED) uses GetSystemMetricsForDpi directly.
	var gotIndex int
	var gotDPI uint32
	size := resolveTrayIconSize(
		144,
		func() uint32 { t.Fatal("getWindowDPI should not be called when dpi is given"); return 96 },
		func(index int, dpi uint32) int {
			gotIndex, gotDPI = index, dpi
			return 24
		},
		func(int) int { t.Fatal("fallback GetSystemMetrics should not be called"); return 16 },
	)
	if size != 24 || gotIndex != smCxSmIcon || gotDPI != 144 {
		t.Fatalf("explicit DPI: size=%d index=%d dpi=%d, want 24, %d, 144", size, gotIndex, gotDPI, smCxSmIcon)
	}

	// Zero DPI queries GetDpiForWindow first, then GetSystemMetricsForDpi.
	gotDPI = 0
	size = resolveTrayIconSize(
		0,
		func() uint32 { return 192 },
		func(index int, dpi uint32) int {
			gotDPI = dpi
			return 32
		},
		func(int) int { return 16 },
	)
	if size != 32 || gotDPI != 192 {
		t.Fatalf("window DPI: size=%d dpi=%d, want 32, 192", size, gotDPI)
	}

	// Missing GetSystemMetricsForDpi falls back to GetSystemMetrics.
	size = resolveTrayIconSize(144, nil, nil, func(index int) int {
		if index != smCxSmIcon {
			t.Fatalf("fallback index=%d, want %d", index, smCxSmIcon)
		}
		return 20
	})
	if size != 20 {
		t.Fatalf("fallback metrics: size=%d, want 20", size)
	}

	// All queries returning 0 fall back to 16.
	size = resolveTrayIconSize(0, func() uint32 { return 0 }, func(int, uint32) int { return 0 }, func(int) int { return 0 })
	if size != 16 {
		t.Fatalf("default fallback: size=%d, want 16", size)
	}
}

func TestDPIFromWParam(t *testing.T) {
	// WM_DPICHANGED packs X DPI in LOWORD and Y DPI in HIWORD.
	wparam := uintptr(144)<<16 | uintptr(144)
	if got := dpiFromWParam(wparam); got != 144 {
		t.Fatalf("dpiFromWParam = %d, want 144", got)
	}
}
