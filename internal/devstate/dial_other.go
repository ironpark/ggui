//go:build !windows && !(js && wasm)

package devstate

import (
	"net"
	"time"
)

// dial connects to ggui dev's unix socket.
func dial(addr string, timeout time.Duration) (stream, error) {
	return net.DialTimeout("unix", addr, timeout)
}
