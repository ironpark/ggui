//go:build !js

package runtime

import (
	"errors"
	"io"
	"net"
	"os"
	goruntime "runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestClaimInstanceHandsLaterLaunchesToTheFirst(t *testing.T) {
	id := "ggui.test." + t.Name() + "." + time.Now().Format("150405.000000")
	got := make(chan Launch, 1)
	release, err := ClaimInstance(id, func(l Launch) { got <- l })
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := ClaimInstance(id, func(Launch) { t.Error("second instance was handed a launch") }); !errors.Is(err, ErrRunning) {
		t.Fatalf("second claim = %v, want ErrRunning", err)
	}
	select {
	case l := <-got:
		dir, _ := os.Getwd()
		if l.Dir != dir || !slices.Equal(l.Args, os.Args[1:]) {
			t.Fatalf("launch = %+v", l)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the first instance was not handed the launch")
	}
	release()
	// Released, the id is free again, even with the old socket left.
	again, err := ClaimInstance(id, func(Launch) {})
	if err != nil {
		t.Fatalf("claim after release = %v", err)
	}
	again()
}

// instanceID is an app id no other test or run of this one claims.
func instanceID(t *testing.T) string {
	return "ggui.test." + t.Name() + "." + time.Now().Format("150405.000000000")
}

func TestClaimInstanceIgnoresAMalformedLaunch(t *testing.T) {
	t.Parallel()
	id := instanceID(t)
	got := make(chan Launch, 2)
	release, err := ClaimInstance(id, func(l Launch) { got <- l })
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	conn, err := net.Dial("unix", instanceAddr(id))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write([]byte("{not a launch")); err != nil {
		t.Fatal(err)
	}
	conn.(*net.UnixConn).CloseWrite()
	// The instance hangs up without the acknowledgement a launch gets.
	if b, err := io.ReadAll(conn); err != nil || len(b) != 0 {
		t.Fatalf("a malformed launch was answered with %q, %v; want the connection closed", b, err)
	}
	conn.Close()

	if _, err := ClaimInstance(id, func(Launch) {}); !errors.Is(err, ErrRunning) {
		t.Fatalf("a claim after the malformed one = %v, want ErrRunning", err)
	}
	select {
	case l := <-got:
		if dir, _ := os.Getwd(); l.Dir != dir {
			t.Fatalf("the first launch handed over is %+v, want the real one", l)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the real launch was not handed over")
	}
}

func TestClaimInstanceReplacesAStaleSocket(t *testing.T) {
	t.Parallel()
	if goruntime.GOOS == "linux" {
		t.Skip("Linux uses an abstract socket, which leaves nothing behind")
	}
	id := instanceID(t)
	addr := instanceAddr(id)
	// What a crashed instance leaves: a file at the address nobody listens on.
	if err := os.WriteFile(addr, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(addr) })
	release, err := ClaimInstance(id, func(Launch) {})
	if err != nil {
		t.Fatalf("claim over a stale socket = %v, want it replaced", err)
	}
	release()
}

func TestClaimInstanceAllowsNilCallback(t *testing.T) {
	t.Parallel()
	id := instanceID(t)
	release, err := ClaimInstance(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := ClaimInstance(id, nil); !errors.Is(err, ErrRunning) {
		t.Fatalf("second claim with nil callback = %v, want ErrRunning", err)
	}
}

func TestClaimInstanceScopedByUIDAndRestricted(t *testing.T) {
	t.Parallel()
	if goruntime.GOOS == "windows" {
		t.Skip("Windows uses per-user TempDir without POSIX UID/mode")
	}
	id := instanceID(t)
	addr := instanceAddr(id)
	if !strings.Contains(addr, "-"+strconv.Itoa(os.Getuid())) {
		t.Fatalf("instanceAddr = %q, want UID %d in path", addr, os.Getuid())
	}
	if goruntime.GOOS == "linux" {
		return
	}
	release, err := ClaimInstance(id, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	info, err := os.Stat(addr)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket mode = %04o, want 0600", perm)
	}
}

func TestClaimInstanceDoesNotRemoveLiveBusySocket(t *testing.T) {
	t.Parallel()
	if goruntime.GOOS == "linux" {
		t.Skip("Linux abstract sockets have no file to remove")
	}
	id := instanceID(t)
	addr := instanceAddr(id)
	ln, err := net.Listen("unix", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Accept and immediately close without writing ack, simulating a live
	// peer that rejects or fails handOver without being a dead socket.
	accepted := make(chan struct{}, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- struct{}{}
		conn.Close()
	}()

	if release, err := ClaimInstance(id, nil); err == nil {
		release()
		t.Fatal("ClaimInstance replaced a live listening socket")
	}
	<-accepted
	if _, err := os.Stat(addr); err != nil {
		t.Fatalf("live socket file was removed: %v", err)
	}
}
