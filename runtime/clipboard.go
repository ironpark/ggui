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
// the platform's clipboard API; elsewhere it talks to the OS through
// wl-clipboard, xclip or xsel and keeps an in-process copy where none is
// available, as in a browser.
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
