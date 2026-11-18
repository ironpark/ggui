package robustio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRenameReplacesAndRemoveRemoves(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	os.WriteFile(a, []byte("old"), 0o644)
	os.WriteFile(b, []byte("new"), 0o644)
	if err := Rename(b, a); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(a); string(got) != "new" {
		t.Fatalf("a holds %q after the rename, want new", got)
	}
	if err := Remove(a); err != nil {
		t.Fatal(err)
	}
	// What waiting does not change is said at once, as it is.
	if err := Rename(b, a); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("renaming a missing file: %v, want not exist", err)
	}
	if err := Remove(a); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("removing a missing file: %v, want not exist", err)
	}
}
