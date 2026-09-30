package goid

import (
	"sync"
	"testing"
)

func TestIDMatchesTheStack(t *testing.T) {
	if !Fast() {
		t.Log("fast path unavailable; ID parses the stack")
	}
	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() {
			if got, want := ID(), slowID(); got != want {
				t.Errorf("ID = %d, want %d", got, want)
			}
		})
	}
	wg.Wait()
	if got, want := ID(), slowID(); got != want {
		t.Fatalf("ID = %d, want %d", got, want)
	}
}

func BenchmarkID(b *testing.B) {
	for b.Loop() {
		ID()
	}
}

func BenchmarkSlowID(b *testing.B) {
	for b.Loop() {
		slowID()
	}
}
