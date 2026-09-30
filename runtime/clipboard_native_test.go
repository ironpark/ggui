//go:build darwin && !ios

package runtime

import (
	"sync"
	"testing"
)

func TestPasteboardRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("touches the user's clipboard")
	}
	c := NativeClipboard()
	saved := c.Read()
	defer c.Write(saved)
	const text = "ggui clipboard ✓ 한글\nsecond line"
	c.Write(text)
	if got := c.Read(); got != text {
		t.Fatalf("Read = %q, want %q", got, text)
	}
}

// Calls from many goroutines at once, each moved between threads by the
// scheduler, must neither overlap in AppKit nor drain a pool on a thread
// that did not make it. Either crashed the process.
func TestPasteboardFromManyGoroutines(t *testing.T) {
	if testing.Short() {
		t.Skip("touches the user's clipboard")
	}
	c := NativeClipboard()
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for range 200 {
				_ = c.Read()
			}
		})
	}
	wg.Wait()
}
