package lucide

import "testing"

func TestSetIsComparable(t *testing.T) {
	t.Parallel()
	if Set() != Set() {
		t.Fatal("Set() != Set()")
	}
}
