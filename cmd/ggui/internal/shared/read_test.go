package shared

import (
	"os"
	"path/filepath"
	"testing"
)

// A file being read can be deleted and renamed over, on every system.
func TestReadFileSharesDeletion(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "a.go")
	if err := os.WriteFile(name, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadFile(name)
	if err != nil || string(got) != "package a\n" {
		t.Fatalf("ReadFile = %q, %v", got, err)
	}
	if _, err := ReadFile(filepath.Join(dir, "missing.go")); !os.IsNotExist(err) {
		t.Errorf("a missing file: %v, want not exist", err)
	}

	// Hold the file open as ReadFile does, then delete and replace it.
	if err := os.WriteFile(name, []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	tmp := filepath.Join(dir, "a.go.tmp")
	if err := os.WriteFile(tmp, []byte("package c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, name); err != nil {
		t.Errorf("renaming over a file being read: %v", err)
	}
	if err := os.Remove(name); err != nil {
		t.Errorf("deleting a file being read: %v", err)
	}
}
