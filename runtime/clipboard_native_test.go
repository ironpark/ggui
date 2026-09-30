//go:build darwin && !ios

package runtime

import "testing"

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
