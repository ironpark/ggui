package ggui

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
)

// Clipboard moves text between the app and the rest of the system. The
// default talks to the OS clipboard through the platform's command-line
// tool (pbcopy/pbpaste, wl-clipboard or xclip, PowerShell) and keeps an
// in-process copy where none is available. SetClipboard replaces it.
type Clipboard interface {
	Read() string
	Write(s string)
}

var (
	clipboardMu sync.Mutex
	clipboard   Clipboard = &execClipboard{}
)

// SetClipboard replaces the clipboard text fields cut, copy and paste with.
func SetClipboard(c Clipboard) {
	clipboardMu.Lock()
	defer clipboardMu.Unlock()
	clipboard = c
}

func currentClipboard() Clipboard {
	clipboardMu.Lock()
	defer clipboardMu.Unlock()
	return clipboard
}

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

// execClipboard shells out to the platform tool and falls back to memory.
type execClipboard struct {
	mem MemoryClipboard
}

func (c *execClipboard) commands() (read, write []string) {
	switch runtime.GOOS {
	case "darwin":
		return []string{"pbpaste"}, []string{"pbcopy"}
	case "windows":
		return []string{"powershell", "-NoProfile", "-Command", "Get-Clipboard -Raw"},
			[]string{"powershell", "-NoProfile", "-Command", "$input | Set-Clipboard"}
	case "linux", "freebsd", "openbsd", "netbsd":
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			if _, err := exec.LookPath("wl-paste"); err == nil {
				return []string{"wl-paste", "--no-newline"}, []string{"wl-copy"}
			}
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			return []string{"xclip", "-selection", "clipboard", "-o"}, []string{"xclip", "-selection", "clipboard", "-i"}
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			return []string{"xsel", "--clipboard", "--output"}, []string{"xsel", "--clipboard", "--input"}
		}
	}
	return nil, nil
}

func (c *execClipboard) Read() string {
	read, _ := c.commands()
	if read == nil {
		return c.mem.Read()
	}
	out, err := exec.Command(read[0], read[1:]...).Output()
	if err != nil {
		return c.mem.Read()
	}
	return strings.TrimSuffix(string(out), "\r\n")
}

func (c *execClipboard) Write(s string) {
	c.mem.Write(s)
	_, write := c.commands()
	if write == nil {
		return
	}
	cmd := exec.Command(write[0], write[1:]...)
	cmd.Stdin = strings.NewReader(s)
	_ = cmd.Run()
}
