//go:build !windows

package dev

import (
	"net"
	"os"

	"github.com/ironpark/ggui/cmd/ggui/internal/viewer"
)

// listenKeep listens on a unix socket in a directory of its own, which
// only the user may enter, as the viewer's is.
func listenKeep() (keepListener, string, func(), error) {
	dir, socket, err := viewer.SocketPath("k.sock")
	if err != nil {
		return nil, "", nil, err
	}
	ln, err := net.Listen("unix", socket)
	if err != nil {
		os.RemoveAll(dir)
		return nil, "", nil, err
	}
	return unixListener{ln}, socket, func() { os.RemoveAll(dir) }, nil
}

// unixListener is a net.Listener whose connections are keepStreams.
type unixListener struct{ net.Listener }

func (l unixListener) Accept() (keepStream, error) { return l.Listener.Accept() }
