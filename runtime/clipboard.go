package runtime

import (
	"sync"
)

// Clipboard moves text between the app and the rest of the system: what
// text fields cut, copy and paste with.
type Clipboard interface {
	Read() string
	Write(s string)
}

// NativeClipboard returns the system clipboard. On macOS and Windows it is
// the platform's clipboard API and on Linux and the BSDs the X11 CLIPBOARD
// selection, which XWayland shares with Wayland. In a browser it is an
// in-process copy.
func NativeClipboard() Clipboard { return nativeClipboard() }

// MemoryClipboard is a Clipboard that lives in the process only, for tests
// and platforms with no system clipboard.
type MemoryClipboard struct {
	mu   sync.Mutex
	text string
}

// Read implements Clipboard.
func (m *MemoryClipboard) Read() string { m.mu.Lock(); defer m.mu.Unlock(); return m.text }

// Write implements Clipboard.
func (m *MemoryClipboard) Write(s string) { m.mu.Lock(); defer m.mu.Unlock(); m.text = s }
