package reactive

import (
	"fmt"
	"testing"
)

// The graph is measured here on its own, with no widget, window or frame
// around it: what a change to how a value tells its readers costs.

func BenchmarkStateGet(b *testing.B) {
	s := State(1)
	b.ReportAllocs()
	for b.Loop() {
		_ = s.Get()
	}
}

// A read inside a computation subscribes it, once: the reads after are the
// ones a component's render makes of a value it already watches.
func BenchmarkStateGetTracked(b *testing.B) {
	s := State(1)
	b.ReportAllocs()
	dispose := Observe(func() {
		for b.Loop() {
			_ = s.Get()
		}
	})
	dispose()
}

// A set reaches n computations that read the value, and the flush runs
// them again.
func BenchmarkSetFlush(b *testing.B) {
	for _, n := range []int{1, 100, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s := State(0)
			dispose := Root(func() {
				for range n {
					Observe(func() { s.Get() })
				}
			})
			defer dispose()
			Flush()
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				i++
				s.Set(i)
				Flush()
			}
		})
	}
}

// A set equal to the value it replaces wakes no one.
func BenchmarkSetUnchanged(b *testing.B) {
	s := State(0)
	dispose := Root(func() {
		for range 100 {
			Observe(func() { s.Get() })
		}
	})
	defer dispose()
	Flush()
	b.ReportAllocs()
	for b.Loop() {
		s.Set(0)
		Flush()
	}
}

// A chain of deriveds, each of the one before, read at its end after its
// head changes.
func BenchmarkDerivedChain(b *testing.B) {
	for _, depth := range []int{10, 100} {
		b.Run(fmt.Sprint(depth), func(b *testing.B) {
			head := State(0)
			var tail Readable[int]
			dispose := Root(func() {
				var r Readable[int] = head
				for range depth {
					prev := r
					r = Derived(func() int { return prev.Get() + 1 })
				}
				tail = r
				Observe(func() { tail.Get() })
			})
			defer dispose()
			Flush()
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				i++
				head.Set(i)
				Flush()
				if tail.Get() != i+depth {
					b.Fatal("the chain's end is stale")
				}
			}
		})
	}
}

// One value read by n deriveds whose results do not change: the set wakes
// them all and stops there, with nothing past them to run.
func BenchmarkDerivedFanOutCutoff(b *testing.B) {
	for _, n := range []int{100, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s := State(0)
			dispose := Root(func() {
				for range n {
					d := Derived(func() bool { return s.Get() >= 0 })
					Observe(func() { d.Get() })
				}
			})
			defer dispose()
			Flush()
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				i++
				s.Set(i)
				Flush()
			}
		})
	}
}

// A diamond: two deriveds of one value, and a third of both, which runs
// once for each change of the value however many paths reach it.
func BenchmarkDerivedDiamond(b *testing.B) {
	s := State(0)
	runs := 0
	dispose := Root(func() {
		l := Derived(func() int { return s.Get() + 1 })
		r := Derived(func() int { return s.Get() * 2 })
		j := Derived(func() int { runs++; return l.Get() + r.Get() })
		Observe(func() { j.Get() })
	})
	defer dispose()
	Flush()
	runs = 0
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		i++
		s.Set(i)
		Flush()
	}
	if runs != i {
		b.Fatalf("the join ran %d times for %d changes", runs, i)
	}
}

// What a frame with nothing changed costs when n computations are alive.
func BenchmarkIdleEffects(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			value := State(0)
			dispose := Root(func() {
				for range n {
					Observe(func() { value.Get() })
				}
			})
			defer dispose()
			Flush()
			b.ReportAllocs()
			for b.Loop() {
				Flush()
			}
		})
	}
}

// The same for user effects, which run after the frame's own.
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
			FlushUsers(nil)
			b.ReportAllocs()
			for b.Loop() {
				FlushUsers(nil)
			}
		})
	}
}

// A user effect that reads the value runs again, with its cleanup, after
// each change.
func BenchmarkUserEffectRerun(b *testing.B) {
	s := State(0)
	cleaned := 0
	dispose := Root(func() {
		for range 100 {
			Effect(func() Cleanup { s.Get(); return func() { cleaned++ } })
		}
	})
	defer dispose()
	FlushUsers(nil)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		i++
		s.Set(i)
		Flush()
		FlushUsers(nil)
	}
}

// Building a root of n computations and disposing of it: what a component
// coming and going costs the graph.
func BenchmarkRootBuildDispose(b *testing.B) {
	for _, n := range []int{10, 1000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s := State(0)
			b.ReportAllocs()
			for b.Loop() {
				dispose := Root(func() {
					for range n {
						Observe(func() { s.Get() })
					}
				})
				dispose()
			}
		})
	}
}
