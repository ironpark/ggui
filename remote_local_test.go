package ggui

import (
	"errors"
	"os"
	"testing"

	"github.com/ironpark/ggui/internal/platform/remotedisplay"
)

// Outside ggui dev's viewer, the remote notices say nothing and change no
// error.
func TestRemoteNoticesOnlyInARemoteApp(t *testing.T) {
	if remotedisplay.On() {
		t.Skip("runs as a remote app")
	}
	base := errors.New("x")
	if err := errOneWindow(base); err != base {
		t.Errorf("errOneWindow = %v, want the error unchanged", err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := os.Stderr
	os.Stderr = w
	remoteNotice("something does not show")
	os.Stderr = stderr
	w.Close()
	if n, _ := r.Read(make([]byte, 1)); n != 0 {
		t.Error("remoteNotice wrote to stderr in an ordinary app")
	}
	r.Close()
}
