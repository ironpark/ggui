package main

import (
	"github.com/ironpark/ggui"
	"testing"
	"time"
)

func TestAllReferenceExamples(t *testing.T) {
	if len(chartExamples) != 70 {
		t.Fatalf("catalog: %d", len(chartExamples))
	}
	for _, e := range chartExamples {
		t.Run(e.Name, func(t *testing.T) {
			for _, width := range []float64{320, 640} {
				p := ggui.ProbeBuilder(func() ggui.Widget { return chartCard(e) }, ggui.Sz(width, 600))
				p.Frame()
				p.Advance(2100 * time.Millisecond)
				p.Frame()
				p.Close()
			}
		})
	}
}
