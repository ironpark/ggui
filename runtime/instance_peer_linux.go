//go:build linux

package runtime

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// samePeer reports whether the process at the other end of conn runs as
// this one's user. An abstract socket has no file whose permissions keep
// other users out, so both ends ask.
func samePeer(conn net.Conn) bool {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return false
	}
	var cred *unix.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil || credErr != nil {
		return false
	}
	return int(cred.Uid) == os.Getuid()
}
