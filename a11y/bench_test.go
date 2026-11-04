package a11y

import (
	"fmt"
	"strings"
	"testing"
)

// A window's tree as a long list publishes it: a root, rows under it, and
// a few leaves in each, in paint order.
func benchNodes(rows int) []SemNode {
	nodes := []SemNode{{Node: Node{Role: RoleWindow, Name: "Main"}, Parent: -1}}
	for range rows {
		row := len(nodes)
		nodes = append(nodes, SemNode{Node: Node{Role: RoleGroup}, Parent: 0})
		for _, role := range []Role{RoleText, RoleButton, RoleCheckbox} {
			nodes = append(nodes, SemNode{Node: Node{Role: role, Name: "item"}, Parent: row})
		}
	}
	return nodes
}

func BenchmarkBuild(b *testing.B) {
	for _, rows := range []int{100, 2500} {
		b.Run(fmt.Sprint(rows*4+1), func(b *testing.B) {
			nodes := benchNodes(rows)
			b.ReportAllocs()
			for b.Loop() {
				Build(nodes, len(nodes)-1)
			}
		})
	}
}

// A text area of mixed scripts, 64 KiB over 1000 lines, as a screen
// reader reading it line by line asks of it: the line of an offset, and
// the range of a line.
func benchTextNode() Node {
	line := "The quick brown fox — 빠른 갈색 여우 — 素早い茶色の狐 😀\n"
	var b strings.Builder
	var runs []TextRun
	for b.Len() < 64<<10 {
		start := b.Len()
		b.WriteString(line)
		runs = append(runs, TextRun{Start: start, End: b.Len()})
	}
	return Node{Role: RoleTextField, Value: b.String(), Runs: runs}
}

func BenchmarkLineForIndex(b *testing.B) {
	n := benchTextNode()
	end := CharCount(n)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		LineForIndex(n, i*7919%end)
		i++
	}
}

func BenchmarkRangeForLine(b *testing.B) {
	n := benchTextNode()
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		RangeForLine(n, i%len(n.Runs))
		i++
	}
}
