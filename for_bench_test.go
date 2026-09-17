package ggui

import (
	"fmt"
	"testing"
)

func BenchmarkForFrame(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		for _, changed := range []bool{false, true} {
			b.Run(fmt.Sprintf("rows=%d/changed=%t", n, changed), func(b *testing.B) {
				values := make([]*Signal[int], n)
				ids := make([]int, n)
				for i := range ids {
					ids[i], values[i] = i, State(0)
				}
				items := State(ids)
				p := ProbeBuilder(func() Widget {
					return For(items, func(id int) int { return id }, func(item Reader[int]) Widget {
						return Reactive(func() Widget { return Box().Size(float64(values[item.Get()].Get()%7+1), 1) })
					})
				}, Sz(100, 100))
				defer p.Close()
				p.Frame()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if changed {
						values[i%n].Set(i + 1)
					}
					p.Frame()
				}
				b.StopTimer()
			})
		}
	}
}
