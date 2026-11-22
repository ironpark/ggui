//go:build darwin || freebsd || dragonfly

package runtime

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// samePeer reports whether the process at the other end of conn runs as
// this one's user via LOCAL_PEERCRED.
func samePeer(conn net.Conn) bool {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return false
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return false
	}
	var cred *unix.Xucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	}); err != nil || credErr != nil {
		return false
	}
	return int(cred.Uid) == os.Getuid()
}
