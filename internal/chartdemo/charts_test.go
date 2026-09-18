package chartdemo

import (
	"github.com/ironpark/ggui"
	"testing"
	"time"
)

func TestAllReferenceExamples(t *testing.T) {
	if len(Examples) != 70 {
		t.Fatalf("catalog: %d", len(Examples))
	}
	for _, e := range Examples {
		t.Run(e.Name, func(t *testing.T) {
			for _, width := range []float64{320, 640} {
				p := ggui.ProbeBuilder(func() ggui.Widget { return Card(e) }, ggui.Sz(width, 600))
				p.Frame()
				p.Advance(2100 * time.Millisecond)
				p.Frame()
				p.Close()
			}
		})
	}
}
