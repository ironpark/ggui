package ui

import (
	"math"
	"testing"

	"github.com/ironpark/ggui"
)

// The scores are what cmdk's own commandScore (cmdk/src/command-score.ts)
// returns for the same text, query and keywords, computed by running it.
func TestCommandScoreMatchesCmdk(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		text, query string
		keywords    []string
		want        float64
	}{
		{"Toggle dark theme", "tdt", nil, 0.80181981},
		{"Toggle dark theme", "dark", nil, 0.891},
		{"Drag marker", "dark", nil, 0.16744343529970956},
		{"HTML", "HM", nil, 0.1681317},
		{"haml", "HM", nil, 0.16809807534131702},
		{"html", "html", nil, 1},
		{"html5", "html", nil, 0.99},
		{"ouch", "uc", nil, 0.1683},
		{"curtain", "uc", nil, 0.099},
		{"bad", "bd", nil, 0.16983},
		{"bard", "bd", nil, 0.16966017000000003},
		{"foo/bar", "b", nil, 0.792},
		{"foo bar", "b", nil, 0.891},
		{"foo-bar", "b", nil, 0.891},
		{"foo_bar.baz qux", "q", nil, 0.891},
		{"a/b/c/d", "ad", nil, 0.7984008},
		{"ab", "aab", nil, 0.1}, // a doubled letter at the very start of the text
		{"aab", "ab", nil, 0.17},
		{"llama", "lama", nil, 0.17},
		{"banana", "bnn", nil, 0.099},
		{"Profile", "rpofile", nil, 0.1},
		{"Settings", "stg", nil, 0.028493877878829454},
		{"Open Recent", "or", nil, 0.8908218089100001},
		{"Zoom In", "zoom in", nil, 0.9998000100000001},
		{"Zoom-In", "zoom in", nil, 0.9997000299990001},
		{"Copy", "copy", nil, 0.9999},
		{"Copy", "COPY", nil, 0.9997000299990001},
		{"New File", "nf", []string{"create", "document"}, 0.8908218089100001},
		{"New File", "doc", []string{"create", "document"}, 0.891},
		{"New File", "file create", []string{"create", "document"}, 0.8909109000000001},
		{"  New File ", "doc", []string{" create", "document "}, 0.891}, // cmdk trims the value and each keyword
		{"abc", "xyz", nil, 0},
		{"abc", "", nil, 0.99},
		{"", "doc", []string{"document"}, 0}, // no value, no match
		{"Calendar", "cal", nil, 0.989901},
		{"Calculator", "cal", nil, 0.989901},
		{"Search Emoji", "se", nil, 0.989901},
		{"Set theme", "se", nil, 0.989901},
	} {
		if got := commandScore(commandText(c.text, c.keywords), c.query); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("commandScore(%q + %q, %q) = %v, want %v", c.text, c.keywords, c.query, got, c.want)
		}
	}
}

// What a query leaves is ordered as cmdk's sort leaves it: the entries
// outside any group, best first; then the groups, the one holding the best
// match first; and inside a group the best first. Equal scores keep the
// order the entries were given in.
func TestCommandRankOrdersAsCmdkSorts(t *testing.T) {
	t.Parallel()
	entry := func(label, group string) CommandEntry { return CommandItem(label, nil).Group(group) }
	c := Command(ggui.State(""),
		entry("Drag marker", "Edit"),       // 0: dark = 0.167
		entry("Dark reader", "Edit"),       // 1: dark = 0.99
		entry("Toggle dark theme", "View"), // 2: dark = 0.891
		entry("Dark", "View"),              // 3: dark = 0.9999
		entry("Sidebar", "View"),           // 4: no match
		entry("Darken", ""),                // 5: dark = 0.99
		entry("Dark mode", ""),             // 6: dark = 0.99, after 5 by the order given
		entry("A dark horse", ""),          // 7: dark = 0.891
	)
	got := c.rank("dark")
	want := []int{5, 6, 7, 3, 2, 1, 0}
	if len(got) != len(want) {
		t.Fatalf("rank = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("rank = %v, want %v", got, want)
		}
	}
}
