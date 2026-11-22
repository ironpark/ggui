//go:build !linux && !darwin && !freebsd && !dragonfly && !js

package runtime

import "net"

// samePeer reports whether the process at the other end of conn runs as
// this one's user. On platforms without SO_PEERCRED or LOCAL_PEERCRED, the
// socket file is scoped to the current user and restricted to 0600 (or in
// the user's private temporary directory on Windows).
func samePeer(net.Conn) bool { return true }
