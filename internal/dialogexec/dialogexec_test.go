//go:build !windows

package dialogexec

import (
	"slices"
	"testing"
)

func TestZenityArgs(t *testing.T) {
	r := Request{
		Kind:      OpenMultiple,
		Title:     "Pick",
		Directory: "/tmp",
		Filters:   []Filter{{Name: "Images", Extensions: []string{"png", "jpg"}}},
	}
	got := ZenityArgs(r)
	want := []string{
		"--file-selection", "--title=Pick", "--multiple", "--separator=\n",
		"--filename=/tmp/", "--file-filter=Images | *.png *.jpg",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("ZenityArgs = %q, want %q", got, want)
	}
	got = ZenityArgs(Request{Kind: Save, Directory: "/tmp", FileName: "out.txt"})
	want = []string{"--file-selection", "--save", "--confirm-overwrite", "--filename=/tmp/out.txt"}
	if !slices.Equal(got, want) {
		t.Fatalf("ZenityArgs(save) = %q, want %q", got, want)
	}
	r.Kind = Folder
	got = ZenityArgs(r)
	if slices.ContainsFunc(got, func(s string) bool { return len(s) > 13 && s[:13] == "--file-filter" }) {
		t.Fatalf("folder dialog carries a file filter: %q", got)
	}
	got = ZenityArgs(Request{Kind: Open, Filters: []Filter{{Name: "All files"}}})
	if !slices.Contains(got, "--file-filter=All files | *") {
		t.Fatalf("unrestricted filter = %q, want *", got)
	}
}

func TestKdialogArgs(t *testing.T) {
	r := Request{
		Kind:      OpenMultiple,
		Title:     "Pick",
		Directory: "/tmp",
		Filters:   []Filter{{Name: "Images", Extensions: []string{"png"}}},
	}
	got := KdialogArgs(r)
	want := []string{"--getopenfilename", "/tmp/", "Images (*.png)", "--multiple", "--separate-output", "--title", "Pick"}
	if !slices.Equal(got, want) {
		t.Fatalf("KdialogArgs = %q, want %q", got, want)
	}
	r.Kind = Folder
	got = KdialogArgs(r)
	want = []string{"--getexistingdirectory", "/tmp/", "--title", "Pick"}
	if !slices.Equal(got, want) {
		t.Fatalf("KdialogArgs(folder) = %q, want %q", got, want)
	}
	got = KdialogArgs(Request{Kind: Save, FileName: "x.txt", Filters: []Filter{{Name: "All files"}}})
	want = []string{"--getsavefilename", "x.txt", "All files (*)"}
	if !slices.Equal(got, want) {
		t.Fatalf("KdialogArgs(save, unrestricted) = %q, want %q", got, want)
	}
}
