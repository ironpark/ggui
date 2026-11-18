//go:build !windows && !(js && wasm)

package devstate

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// listenTest listens on a unix socket, as ggui dev does here, and returns
// its address and a function that takes the next connection.
func listenTest(t *testing.T) (string, func() (io.ReadWriteCloser, error)) {
	t.Helper()
	dir, err := os.MkdirTemp("", "ggui-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "k.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("no unix socket here: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	return sock, func() (io.ReadWriteCloser, error) { return ln.Accept() }
}
