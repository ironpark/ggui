package main

import (
	"github.com/ironpark/ggui"
	"testing"
)

func TestCatalog(t *testing.T) {
	for _, name := range controlNames {
		for _, width := range []float64{200, 320, 640} {
			t.Run(name, func(t *testing.T) {
				p := ggui.ProbeBuilder(func() ggui.Widget { return buildControl(name).Widget }, ggui.Sz(width, 600))
				defer p.Close()
				p.Frame()
			})
		}
	}
}
