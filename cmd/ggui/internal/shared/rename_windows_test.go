package shared

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// A file held open for a moment, as an antivirus or a watcher holds one it
// reads, is renamed over once it is let go: os.Rename alone is refused.
func TestRenameWaitsOutAnOpenFile(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.WriteFile(a, []byte("old"), 0o644)
	os.WriteFile(b, []byte("new"), 0o644)
	p, err := syscall.UTF16PtrFromString(a)
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(b, a); err == nil {
		// This Windows replaces an open file after all; nothing to wait out.
		syscall.CloseHandle(h)
		return
	} else if !transient(err) {
		syscall.CloseHandle(h)
		t.Fatalf("os.Rename over an open file failed with %v, which Rename would not wait out", err)
	}
	time.AfterFunc(50*time.Millisecond, func() { syscall.CloseHandle(h) })
	if err := Rename(b, a); err != nil {
		t.Fatalf("Rename over a file open for 50 ms: %v", err)
	}
	if got, _ := os.ReadFile(a); string(got) != "new" {
		t.Fatalf("a holds %q, want new", got)
	}
}
