//go:build (!darwin && !windows) || ios

package runtime

import (
	"os"
	"os/exec"
	goruntime "runtime"
	"strings"
)

func nativeClipboard() Clipboard { return &execClipboard{} }

// execClipboard shells out to the platform tool and falls back to memory.
type execClipboard struct {
	mem MemoryClipboard
}

func (c *execClipboard) commands() (read, write []string) {
	switch goruntime.GOOS {
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
