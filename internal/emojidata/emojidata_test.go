package emojidata

import "testing"

func TestIsCluster(t *testing.T) {
	for _, c := range []struct {
		s    string
		want bool
	}{
		{"a", false},
		{"😀", true},
		{"©", false},  // text presentation by default
		{"©️", true},  // asked to present as emoji
		{"😀︎", false}, // asked to present as text
		{"1️⃣", true}, // keycap
	} {
		if got := IsCluster(c.s); got != c.want {
			t.Errorf("IsCluster(%q) = %v, want %v", c.s, got, c.want)
		}
	}
	if MayHold("plain ascii") || !MayHold("😀") {
		t.Error("MayHold")
	}
}
