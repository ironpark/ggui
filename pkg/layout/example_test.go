package layout_test

import (
	"fmt"

	"github.com/ironpark/ggui/pkg/layout"
)

func ExampleScratch_Flow() {
	// This consumer imports no widgets, graphics backend or reactive runtime.
	var row layout.Scratch
	result := row.Flow(layout.Loose(layout.Sz(200, 80)), layout.FlowStyle{Axis: layout.Horizontal, Gap: 10}, []layout.Item{{}, {Weight: 1}}, func(i int, c layout.Constraints) layout.Measurement {
		if i == 0 {
			child := layout.Box(c, layout.BoxStyle{Width: 50, Height: 30}, nil)
			return layout.Measurement{Size: child.Size}
		}
		return layout.Measurement{Size: c.Constrain(layout.Sz(20, 30))}
	})
	fmt.Println(result.Size)
	for _, r := range result.Children {
		fmt.Println(r.Origin, r.Size)
	}
	// Output:
	// {200 30}
	// {0 0} {50 30}
	// {60 0} {140 30}
}
