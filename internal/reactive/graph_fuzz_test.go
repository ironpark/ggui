package reactive

import "testing"

// graphNode is one value of a randomly built graph, beside the model of it:
// a state, or a memo of two earlier nodes with an observer reading it.
type graphNode struct {
	sig      *StateValue[int] // for a state
	memo     *DerivedValue[int]
	op       byte
	a, b     int // input node indexes, for a memo
	disposed bool
	frozen   int // the model's value once disposed

	computed int // memo runs in the current flush
	observed int // observer runs in the current flush
	seen     int // what the observer read last
}

// graphOp is what a memo computes from its two inputs. Parity and clamp
// often give the same result for a different input, which exercises the
// cut-off.
func graphOp(op byte, a, b int) int {
	switch op % 4 {
	case 0:
		return a + b
	case 1:
		return a - b
	case 2:
		return a & 1
	default:
		return max(min(a, 3), -3) * b
	}
}

// graphModel holds the graph and recomputes every value naively.
type graphModel struct {
	t      *testing.T
	nodes  []*graphNode
	vals   []int // model value of each state
	pulled bool  // a memo was read outside a flush since the last one
}

// values recomputes every node from scratch. A memo's inputs come before
// it, so one pass in order suffices.
func (m *graphModel) values() []int {
	out := make([]int, len(m.nodes))
	for i, n := range m.nodes {
		switch {
		case n.sig != nil:
			out[i] = m.vals[i]
		case n.disposed:
			out[i] = n.frozen
		default:
			out[i] = graphOp(n.op, out[n.a], out[n.b])
		}
	}
	return out
}

// flush settles rt and checks every memo and observer against the model:
// values agree, nothing ran twice in one flush, an observer whose value is
// unchanged did not run, and reading afterwards recomputes nothing.
func (m *graphModel) flush(rt *Runtime) {
	m.t.Helper()
	before := make([]int, len(m.nodes))
	for i, n := range m.nodes {
		n.computed, n.observed = 0, 0
		before[i] = n.seen
	}
	if !rt.Flush() {
		m.t.Fatal("an acyclic graph did not settle")
	}
	model := m.values()
	for i, n := range m.nodes {
		if n.memo == nil {
			continue
		}
		want := model[i]
		if n.computed > 1 || n.observed > 1 {
			m.t.Fatalf("node %d: one flush ran its memo %d and its observer %d times, want at most once each", i, n.computed, n.observed)
		}
		if n.seen != want {
			m.t.Fatalf("node %d: the observer saw %d, want %d", i, n.seen, want)
		}
		if !m.pulled && n.observed == 1 && before[i] == want {
			m.t.Fatalf("node %d: the observer re-ran on an unchanged value %d", i, want)
		}
		computed := n.computed
		if got := Untrack(n.memo.Get); got != want {
			m.t.Fatalf("node %d: Get = %d, want %d", i, got, want)
		}
		if n.computed != computed {
			m.t.Fatalf("node %d: a read after the flush recomputed the memo", i)
		}
	}
	m.pulled = false
}

// FuzzGraphAgreesWithRecomputation applies a sequence of writes, new memos,
// disposals and flushes decoded from the input, and checks after every
// flush that the graph agrees with a naive recomputation of it: see flush.
// Disposing the graph's root must leave nothing registered.
func FuzzGraphAgreesWithRecomputation(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{1, 0, 0, 1, 1, 0, 1, 2, 3, 0, 0, 5, 3})
	f.Add([]byte{1, 2, 0, 1, 1, 3, 3, 1, 0, 4, 0, 1, 3, 2, 0, 0, 0, 3, 3})
	f.Add([]byte{1, 0, 0, 0, 1, 1, 3, 3, 1, 3, 4, 4, 2, 1, 0, 1, 7, 3, 0, 1, 250, 3, 2, 0, 3})
	f.Add([]byte{1, 3, 0, 1, 1, 2, 3, 3, 0, 0, 9, 0, 0, 1, 2, 4, 5, 2, 0, 0, 1, 1, 0, 1, 3})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 400 {
			data = data[:400]
		}
		next := func() byte {
			if len(data) == 0 {
				return 0
			}
			b := data[0]
			data = data[1:]
			return b
		}
		const states, maxNodes = 3, 40
		m := &graphModel{t: t}
		for i := range states {
			m.nodes = append(m.nodes, &graphNode{sig: State(i)})
			m.vals = append(m.vals, i)
		}
		rt := NewRuntime()
		var owner *Computation
		dispose := rt.Root(func() { owner = CurrentOwner() })
		var memos []int
		for len(data) > 0 {
			switch next() % 4 {
			case 0: // write a state
				i, v := int(next())%states, int(int8(next()))
				m.nodes[i].sig.Set(v)
				m.vals[i] = v
			case 1: // derive a memo from two nodes
				op, a, b := next(), int(next())%len(m.nodes), int(next())%len(m.nodes)
				if len(m.nodes) == maxNodes {
					continue
				}
				n := &graphNode{op: op, a: a, b: b}
				in := m.nodes
				WithOwner(owner, func() {
					n.memo = Derived(func() int {
						n.computed++
						return graphOp(op, readNode(in[a]), readNode(in[b]))
					})
					Observe(func() { n.seen = n.memo.Get(); n.observed++ })
				})
				m.nodes = append(m.nodes, n)
				memos = append(memos, len(m.nodes)-1)
				// Building the memo read its inputs outside a flush.
				m.pulled = true
				if want := m.values()[len(m.nodes)-1]; n.seen != want {
					t.Fatalf("a new memo's observer saw %d, want %d", n.seen, want)
				}
			case 2: // dispose a memo, after reading its current value
				if len(memos) == 0 {
					continue
				}
				i := memos[int(next())%len(memos)]
				n := m.nodes[i]
				if n.disposed {
					continue
				}
				want := m.values()[i]
				if got := Untrack(n.memo.Get); got != want {
					t.Fatalf("node %d read %d before disposal, want %d", i, got, want)
				}
				n.memo.Dispose()
				n.disposed, n.frozen, m.pulled = true, want, true
			case 3:
				m.flush(rt)
			}
		}
		m.flush(rt)
		dispose()
		if rt.Count() != 0 {
			t.Fatalf("disposing the graph's root left %d computations", rt.Count())
		}
	})
}

// readNode reads n's value, subscribing the running memo.
func readNode(n *graphNode) int {
	if n.sig != nil {
		return n.sig.Get()
	}
	return n.memo.Get()
}
