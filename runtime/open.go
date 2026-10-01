//go:build !js

package runtime

import (
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

// OpenURL hands url to the desktop: a web address opens in the default
// browser, a mailto: link in the mail client, and a file or folder path in
// the application the desktop associates with it. It returns once the
// desktop has the request, not when the application opens.
func OpenURL(url string) error {
	if url == "" || strings.HasPrefix(url, "-") {
		// The openers take a leading dash as one of their own options.
		return fmt.Errorf("runtime: cannot open %q", url)
	}
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		cmd = exec.Command("xdg-open", url)
	default:
		return ErrUnsupported
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// The opener exits once it has handed the request on; reap it.
	go cmd.Wait()
	return nil
}

// Reveal shows path in the desktop's file manager: selected in a Finder
// window on macOS and an Explorer window on Windows. Elsewhere it asks the
// file manager over D-Bus to select it, as file managers that follow the
// freedesktop.org FileManager1 interface do, and opens the folder holding
// it when none answers. It returns once the desktop has the request.
func Reveal(path string) error {
	if path == "" || strings.HasPrefix(path, "-") {
		return fmt.Errorf("runtime: cannot reveal %q", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", "-R", abs)
	case "windows":
		cmd = explorerSelect(abs)
	case "linux", "freebsd", "openbsd", "netbsd", "dragonfly":
		uri := (&url.URL{Scheme: "file", Path: abs}).String()
		err := exec.Command("dbus-send", "--session", "--print-reply", "--dest=org.freedesktop.FileManager1",
			"/org/freedesktop/FileManager1", "org.freedesktop.FileManager1.ShowItems",
			"array:string:"+uri, "string:").Run()
		if err == nil {
			return nil
		}
		return OpenURL(filepath.Dir(abs))
	default:
		return ErrUnsupported
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
