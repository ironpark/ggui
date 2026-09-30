//go:build !linux && !js

package runtime

import "net"

// samePeer reports whether the process at the other end of conn runs as
// this one's user. Elsewhere the socket is a file in the user's own
// temporary directory, which other users cannot reach.
func samePeer(net.Conn) bool { return true }
