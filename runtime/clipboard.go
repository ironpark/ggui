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
// selection, which XWayland shares with Wayland. In a browser it is a copy
// in the page that a paste the user starts fills from the system and that
// writes through to the system with navigator.clipboard.
func NativeClipboard() Clipboard { return nativeClipboard() }

// ImageClipboard is a Clipboard that can also hold an image, as a
// screenshot taken to the clipboard leaves one. The macOS clipboard is one.
type ImageClipboard interface {
	Clipboard
	// ReadImage is the clipboard's image, encoded as PNG, if it holds one.
	ReadImage() ([]byte, bool)
}

// ReadImage is the image on c as PNG, if c can hold one and does.
func ReadImage(c Clipboard) ([]byte, bool) {
	if ic, ok := c.(ImageClipboard); ok {
		return ic.ReadImage()
	}
	return nil, false
}

// MemoryClipboard is a Clipboard that lives in the process only, for tests
// and platforms with no system clipboard. It holds text or an image, as
// the system clipboard does: writing one clears the other.
type MemoryClipboard struct {
	mu    sync.Mutex
	text  string
	image []byte
}

// Read implements Clipboard.
func (m *MemoryClipboard) Read() string { m.mu.Lock(); defer m.mu.Unlock(); return m.text }

// Write implements Clipboard.
func (m *MemoryClipboard) Write(s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.text, m.image = s, nil
}

// ReadImage implements ImageClipboard.
func (m *MemoryClipboard) ReadImage() ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.image, m.image != nil
}

// WriteImage puts png, an image encoded as PNG, on the clipboard.
func (m *MemoryClipboard) WriteImage(png []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.text, m.image = "", png
}
