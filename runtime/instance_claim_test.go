//go:build !js

package runtime

import (
	"errors"
	"io"
	"net"
	"os"
	goruntime "runtime"
	"testing"
	"time"
)

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
