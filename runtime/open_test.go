//go:build !js

package runtime

import "testing"

func TestRevealFileURIEscapesComma(t *testing.T) {
	got := revealFileURI("/tmp/a,b c.txt")
	want := "file:///tmp/a%2Cb%20c.txt"
	if got != want {
		t.Fatalf("revealFileURI = %q, want %q", got, want)
	}
}
