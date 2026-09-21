package ggui

import (
	"fmt"
	"github.com/ironpark/ggui/internal/reactive"
	"testing"
)

func BenchmarkIdleEffects(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			value := State(0)
			dispose := Root(func() {
				for range n {
					reactive.Observe(func() { value.Get() })
				}
			})
			defer dispose()
			reactive.Flush()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reactive.Flush()
			}
			b.StopTimer()
		})
	}
}

// Exercise real Interactive registrations: each control registers pointer,
// keyboard and cursor regions, and rebuilt controls adopt their prior state.
func BenchmarkInteractiveRegistration(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			c := Canvas{}
			old, next := make([]twice, n), make([]twice, n)
			for i := range old {
				old[i].SetKey(i)
				next[i].SetKey(i)
				old[i].Hit(&c, Rct(Pt(0, float64(i)*20), Sz(100, 20)), &old[i], CursorShapePointer)
			}
			c.prev, c.hits = c.hits, make([]hitRegion, 0, n)
			b.ReportAllocs()
			b.ResetTimer()
			for k := 0; k < b.N; k++ {
				c.hits = c.hits[:0]
				// A real frame starts here, so whatever adoption builds
				// once per frame is built again and shows up in the number.
				c.nextFrame()
				c.resetSemantics()
				for i := range next {
					next[i].Hit(&c, Rct(Pt(0, float64(i)*20), Sz(100, 20)), &next[i], CursorShapePointer)
				}
			}
		})
	}
}

func BenchmarkFocusedNode(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			c := Canvas{}
			for i := range n - 1 {
				c.Leaf(Rct(Pt(0, float64(i)), Sz(100, 1)), Node{Role: RoleText})
			}
			w := &twice{}
			w.Role = RoleTextField
			r := Rct(Pt(0, float64(n)), Sz(100, 20))
			c.Describe(r, w)
			focused := hitRegion{key: w, rect: r}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if focusedNode(&c, &focused) != n-1 {
					b.Fatal("focused node was lost")
				}
			}
		})
	}
}

func BenchmarkIdleUserEffects(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			value := State(0)
			dispose := Root(func() {
				for range n {
					Effect(func() Cleanup { value.Get(); return nil })
				}
			})
			defer dispose()
			reactive.FlushUsers(nil)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reactive.FlushUsers(nil)
			}
		})
	}
}
