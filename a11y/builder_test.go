package a11y

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
)

// nodeSource is a Source over finished nodes, as a test writes them.
type nodeSource []SemNode

func (s nodeSource) Len() int         { return len(s) }
func (s nodeSource) Parent(i int) int { return s[i].Parent }
func (s nodeSource) Same(i int, old *SemNode) bool {
	return s[i].Node.Role == old.Role && s[i].Name == old.Name && s[i].Value == old.Value && reflect.DeepEqual(s[i].Runs, old.Runs)
}
func (s nodeSource) Fill(i int, dst *SemNode) {
	dst.Node = s[i].Node
	dst.Node.Runs = slices.Clone(s[i].Runs)
}

func randomNodes(r *rand.Rand, n int) []SemNode {
	nodes := make([]SemNode, n)
	for i := range nodes {
		nodes[i] = SemNode{Node: Node{Role: RoleButton, Name: fmt.Sprint(r.IntN(3))}, Parent: -1}
		if i > 0 && r.IntN(8) != 0 {
			nodes[i].Parent = r.IntN(i)
		}
		if r.IntN(16) == 0 {
			nodes[i].Value = "ab"
			nodes[i].Runs = []TextRun{{Start: 0, End: 1}, {Start: 1, End: 2}}
		}
	}
	return nodes
}

func snapshot(t *SemTree) []SemNode {
	var out []SemNode
	for _, n := range t.All() {
		out = append(out, n)
	}
	return out
}

// Every tree the builder makes equals one Build makes from scratch, and no
// tree before it changes as the builder shares and replaces its pages.
func TestBuilderMatchesBuildAndNeverWritesAPublishedTree(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(1, 2))
	var b Builder
	var prev *SemTree
	type held struct {
		tree  *SemTree
		nodes []SemNode
		roots []int
	}
	var trees []held
	nodes := randomNodes(r, 300)
	for step := range 200 {
		switch r.IntN(4) {
		case 0: // one name changes
			nodes[r.IntN(len(nodes))].Name = fmt.Sprint(step)
		case 1: // a node moves to another parent
			if len(nodes) > 1 {
				i := 1 + r.IntN(len(nodes)-1)
				nodes[i].Parent = r.IntN(i)
			}
		case 2: // the list grows or shrinks
			nodes = randomNodes(r, 1+r.IntN(500))
		}
		focus := r.IntN(len(nodes)+1) - 1
		got := b.Build(prev, nodeSource(nodes), focus)
		want := Build(slices.Clone(nodes), focus)
		if got.String() != want.String() || !slices.Equal(got.Roots(), want.Roots()) || got.FocusIndex() != focus {
			t.Fatalf("step %d: builder's tree\n%s\nwant\n%s", step, got, want)
		}
		for i := range want.Len() {
			if g, w := got.At(i), want.At(i); !reflect.DeepEqual(g.Node, w.Node) || g.Parent != w.Parent || !slices.Equal(g.Children, w.Children) {
				t.Fatalf("step %d node %d = %+v, want %+v", step, i, g, w)
			}
		}
		trees = append(trees, held{got, snapshot(got), slices.Clone(got.Roots())})
		prev = got
	}
	for k, h := range trees {
		if !reflect.DeepEqual(snapshot(h.tree), h.nodes) || !slices.Equal(h.tree.Roots(), h.roots) {
			t.Fatalf("tree %d changed after it was published", k)
		}
	}
}

func TestBuilderSharesUnchangedPages(t *testing.T) {
	t.Parallel()
	nodes := benchNodes(1000)
	var b Builder
	first := b.Build(nil, nodeSource(nodes), -1)
	if again := b.Build(first, nodeSource(nodes), -1); again != first {
		t.Fatal("an unchanged tree was rebuilt")
	}
	focused := b.Build(first, nodeSource(nodes), 3)
	if focused == first || focused.Ref(5) != first.Ref(5) {
		t.Fatal("a change of focus copied nodes")
	}
	nodes[2*pageSize+1].Name = "changed"
	next := b.Build(focused, nodeSource(nodes), 3)
	for i := range next.Len() {
		if shared := next.Ref(i) == focused.Ref(i); shared == (i>>pageShift == 2) {
			t.Fatalf("node %d shared=%t", i, shared)
		}
	}
	if focused.At(2*pageSize+1).Name == "changed" {
		t.Fatal("the earlier tree saw the change")
	}
}

// One changed node in a long list, as a frame that changed one row asks.
func BenchmarkBuilderOneChange(b *testing.B) {
	for _, rows := range []int{250, 2500} {
		b.Run(fmt.Sprint(rows*4+1), func(b *testing.B) {
			nodes := benchNodes(rows)
			var bl Builder
			names := [2]string{"a", "b"}
			prev := bl.Build(nil, nodeSource(nodes), -1)
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				nodes[i%len(nodes)].Name = names[i&1]
				prev = bl.Build(prev, nodeSource(nodes), -1)
				i++
			}
		})
	}
}
