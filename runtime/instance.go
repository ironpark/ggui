//go:build !js

package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"syscall"
	"time"
)

// maxLaunchBytes caps the size of a launch message decoded from a peer so a
// runaway or hostile local client cannot exhaust memory.
const maxLaunchBytes = 1 << 20

// Launch is what a second launch of an app hands the instance already
// running: its command-line arguments, without the program name, and the
// directory it was started in, against which relative paths among the
// arguments resolve.
type Launch struct {
	Args []string `json:"args"`
	Dir  string   `json:"dir"`
}

// ErrRunning is returned by ClaimInstance when another process already
// runs the app. That process has been handed this launch.
var ErrRunning = errors.New("runtime: the app is already running")

// ClaimInstance makes this process the one instance of the app named id,
// such as "com.example.editor", for the current user. The first process
// to claim an id keeps it until release is called or the process exits,
// and launched is called, on a goroutine of its own, with every later
// launch. A later process's claim hands its Launch to the first and
// returns ErrRunning; it should then exit.
//
// The instances meet at a local socket: an abstract one on Linux and a
// file in the temporary directory elsewhere, which a crashed instance may
// leave behind and the next claim replaces.
func ClaimInstance(id string, launched func(Launch)) (release func(), err error) {
	addr := instanceAddr(id)
	ln, err := listenInstance(addr)
	if err != nil {
		stale, herr := handOver(addr)
		if herr == nil {
			return nil, ErrRunning
		}
		if !stale {
			return nil, herr
		}
		// Nobody is listening at addr: the socket was left by an instance that died.
		os.Remove(addr)
		if ln, err = listenInstance(addr); err != nil {
			return nil, err
		}
	}
	go serveInstance(ln, launched)
	return func() { ln.Close() }, nil
}

// listenInstance binds addr and restricts file permissions to the current user
// on filesystem-backed Unix sockets.
func listenInstance(addr string) (net.Listener, error) {
	ln, err := net.Listen("unix", addr)
	if err != nil {
		return nil, err
	}
	if goruntime.GOOS != "linux" && goruntime.GOOS != "windows" {
		_ = os.Chmod(addr, 0o600)
	}
	return ln, nil
}

// instanceAddr is where the instances of the app named id meet. The id is
// hashed so that any string makes a short, valid name, scoped by UID on Unix.
func instanceAddr(id string) string {
	sum := sha256.Sum256([]byte(id))
	name := "ggui-" + hex.EncodeToString(sum[:8])
	if goruntime.GOOS == "linux" {
		// Abstract, so nothing is left on disk; per user, as a claim is.
		return "@" + name + "-" + strconv.Itoa(os.Getuid())
	}
	if goruntime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), name+".sock")
	}
	return filepath.Join(os.TempDir(), name+"-"+strconv.Itoa(os.Getuid())+".sock")
}

// isStaleDialErr reports whether a dial failure indicates that no live server
// is listening at addr (rather than a timeout on a busy live instance or a
// permission error).
func isStaleDialErr(err error) bool {
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ENOTSOCK) {
		return true
	}
	if goruntime.GOOS == "windows" {
		// WSAECONNREFUSED (10061), WSAENOTSOCK (10038), WSAEINVAL (10022).
		return errors.Is(err, syscall.Errno(10061)) || errors.Is(err, syscall.Errno(10038)) || errors.Is(err, syscall.Errno(10022))
	}
	return false
}

// handOver sends this process's launch to the instance at addr. stale is true
// only when connecting failed because nothing is listening at addr.
func handOver(addr string) (stale bool, err error) {
	conn, err := net.DialTimeout("unix", addr, time.Second)
	if err != nil {
		return isStaleDialErr(err), err
	}
	defer conn.Close()
	if !samePeer(conn) {
		return false, errors.New("runtime: the instance socket belongs to another user")
	}
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	dir, _ := os.Getwd()
	if err := json.NewEncoder(conn).Encode(Launch{Args: os.Args[1:], Dir: dir}); err != nil {
		return false, err
	}
	// Wait for the instance to take it, so that this process does not
	// exit before the launch is read.
	var ack [1]byte
	_, err = conn.Read(ack[:])
	return false, err
}

// serveInstance hands every launch that arrives at ln to launched, until
// ln is closed.
func serveInstance(ln net.Listener, launched func(Launch)) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			if !samePeer(conn) {
				return
			}
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			var l Launch
			if json.NewDecoder(io.LimitReader(conn, maxLaunchBytes)).Decode(&l) != nil {
				return
			}
			conn.Write([]byte{1})
			if launched != nil {
				launched(l)
			}
		}()
	}
}
