//go:build !js

package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"time"
)

// Launch is what a second launch of an app hands the instance already
// running: its command-line arguments, without the program name, and the
// directory it was started in, against which relative paths among the
// arguments resolve.
type Launch struct {
	Args []string
	Dir  string
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
	ln, err := net.Listen("unix", addr)
	if err != nil {
		if handOver(addr) == nil {
			return nil, ErrRunning
		}
		// Nobody answers: the socket was left by an instance that died.
		os.Remove(addr)
		if ln, err = net.Listen("unix", addr); err != nil {
			return nil, err
		}
	}
	go serveInstance(ln, launched)
	return func() { ln.Close() }, nil
}

// instanceAddr is where the instances of the app named id meet. The id is
// hashed so that any string makes a short, valid name.
func instanceAddr(id string) string {
	sum := sha256.Sum256([]byte(id))
	name := "ggui-" + hex.EncodeToString(sum[:8])
	if goruntime.GOOS == "linux" {
		// Abstract, so nothing is left on disk; per user, as a claim is.
		return "@" + name + "-" + strconv.Itoa(os.Getuid())
	}
	return filepath.Join(os.TempDir(), name+".sock")
}

// handOver sends this process's launch to the instance at addr.
func handOver(addr string) error {
	conn, err := net.DialTimeout("unix", addr, time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	if !samePeer(conn) {
		return errors.New("runtime: the instance socket belongs to another user")
	}
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	dir, _ := os.Getwd()
	if err := json.NewEncoder(conn).Encode(Launch{Args: os.Args[1:], Dir: dir}); err != nil {
		return err
	}
	// Wait for the instance to take it, so that this process does not
	// exit before the launch is read.
	var ack [1]byte
	_, err = conn.Read(ack[:])
	return err
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
			if json.NewDecoder(conn).Decode(&l) != nil {
				return
			}
			conn.Write([]byte{1})
			launched(l)
		}()
	}
}
