//go:build !windows

package runtime

import (
	"slices"
	"testing"
)

func TestZenityArgs(t *testing.T) {
	d := FileDialog{
		Title:     "Pick",
		Directory: "/tmp",
		Filters:   []FileFilter{{Name: "Images", Extensions: []string{"png", "jpg"}}},
	}
	got := zenityArgs(kindOpenMultiple, d)
	want := []string{
		"--file-selection", "--title=Pick", "--multiple", "--separator=\n",
		"--filename=/tmp/", "--file-filter=Images | *.png *.jpg",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("zenityArgs = %q, want %q", got, want)
	}
	got = zenityArgs(kindSave, FileDialog{Directory: "/tmp", FileName: "out.txt"})
	want = []string{"--file-selection", "--save", "--confirm-overwrite", "--filename=/tmp/out.txt"}
	if !slices.Equal(got, want) {
		t.Fatalf("zenityArgs(save) = %q, want %q", got, want)
	}
	got = zenityArgs(kindFolder, d)
	if slices.ContainsFunc(got, func(s string) bool { return len(s) > 13 && s[:13] == "--file-filter" }) {
		t.Fatalf("folder dialog carries a file filter: %q", got)
	}
	got = zenityArgs(kindOpen, FileDialog{Filters: []FileFilter{{Name: "All files"}}})
	if !slices.Contains(got, "--file-filter=All files | *") {
		t.Fatalf("unrestricted filter = %q, want *", got)
	}
}

func TestKdialogArgs(t *testing.T) {
	d := FileDialog{
		Title:     "Pick",
		Directory: "/tmp",
		Filters:   []FileFilter{{Name: "Images", Extensions: []string{"png"}}},
	}
	got := kdialogArgs(kindOpenMultiple, d)
	want := []string{"--getopenfilename", "/tmp/", "Images (*.png)", "--multiple", "--separate-output", "--title", "Pick"}
	if !slices.Equal(got, want) {
		t.Fatalf("kdialogArgs = %q, want %q", got, want)
	}
	got = kdialogArgs(kindFolder, d)
	want = []string{"--getexistingdirectory", "/tmp/", "--title", "Pick"}
	if !slices.Equal(got, want) {
		t.Fatalf("kdialogArgs(folder) = %q, want %q", got, want)
	}
	got = kdialogArgs(kindSave, FileDialog{FileName: "x.txt", Filters: []FileFilter{{Name: "All files"}}})
	want = []string{"--getsavefilename", "x.txt", "All files (*)"}
	if !slices.Equal(got, want) {
		t.Fatalf("kdialogArgs(save, unrestricted) = %q, want %q", got, want)
	}
}
