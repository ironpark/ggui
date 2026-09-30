//go:build !js

package runtime

import (
	"fmt"
	"os/exec"
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
