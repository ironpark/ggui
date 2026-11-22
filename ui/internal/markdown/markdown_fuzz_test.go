package markdown

import (
	"strings"
	"testing"
)

// fuzzSeeds are sources worth parsing: every construct, the tricky cases
// of each, and the ways a stream is cut.
var fuzzSeeds = []string{
	"", "\n", "\r\n\r", "# h\n\npara *em* **strong** ~~del~~ `code` [l](u \"t\") ![i](s) <http://a.b> www.x.y\n",
	"- a\n  - b\n    1. c\n\n  more\n- [ ] t\n- [x] u\n", "> q\nlazy\n> > deeper\n>\n> - item\n",
	"```go\ncode\n", "~~~\n```\n~~~~", "    indented\n\n\tcode", "a\n===\nb\n---\n***",
	"| a | b |\n|:-|-:|\n| `x\\|y` | z |\nrow\n\nafter", "| a |\n| - |\n| b", "intro\n| a |\n|---|",
	"**open *nested ~~deep", "[unclosed](http://x", "[a [b](c) d](e)", "*a **b** c*", "_a_b_ __c__",
	"&amp; &#x1F600; &#99999999; &nonsense; \\* \\", "a  \nb\\\nc", "1. a\n2. b\n10) c", "* * *\n- - -",
	"<div>\n*html* stays text\n</div>", "-\tone\n\n\ttwo", " - a\n   - b\n     - c", "> ```\n> x\ny",
	"[*a*](b)*c*", "![a![b](c)](d)", "`` ` `` ``` ``", "a*b*c a_b_c", "*a\n\n*b", "[x]: /url\n\n[x]",
	strings.Repeat("[", 50) + strings.Repeat("]", 50), strings.Repeat("*a ", 40), strings.Repeat("> ", 40) + "x",
	strings.Repeat("- ", 40) + "x", strings.Repeat("`", 20) + "x" + strings.Repeat("`", 19),
	"a\x00b\r\nc\rd", "\xff\xfe *x* \xc3",
}

// FuzzParse checks that parsing never panics and makes sound blocks: in
// order within the source, nested no deeper than maxDepth allows, and the
// same whether parsed whole or as a stream that grew to it.
func FuzzParse(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s, uint16(len(s)/2))
	}
	f.Fuzz(func(t *testing.T, src string, cut uint16) {
		if len(src) > 4096 {
			return
		}
		for _, stream := range []bool{false, true} {
			blocks := parse(src, stream)
			checkBlocks(t, src, blocks, 0, 0)
			// Reparsing after a cut gives what a parse of the whole does.
			at := int(cut) % (len(src) + 1)
			prev := parse(src[:at], stream)
			again := Reparse(prev, src[:at], src, stream)
			if len(again) != len(blocks) {
				t.Fatalf("stream %v, cut at %d of %q: %d blocks reparsed, %d parsed", stream, at, src, len(again), len(blocks))
			}
			for i := range blocks {
				if !Equal(again[i], blocks[i]) || again[i].Pos != blocks[i].Pos || again[i].End != blocks[i].End {
					t.Fatalf("stream %v, cut at %d of %q: block %d reparsed as %s, parsed as %s", stream, at, src, i, dump(again[i:i+1]), dump(blocks[i:i+1]))
				}
			}
		}
	})
}

// checkBlocks checks blocks parsed from src: each within the source and
// after the one before, at nesting depth d.
func checkBlocks(t *testing.T, src string, blocks []Block, from, d int) {
	t.Helper()
	if d > 2*maxDepth {
		t.Fatalf("%q: blocks nested %d deep", src, d)
	}
	for i, b := range blocks {
		if b.Pos < from || b.End < b.Pos || b.End > len(src) {
			t.Fatalf("%q: block %d (%s) spans %d to %d, after %d", src, i, dump(blocks[i:i+1]), b.Pos, b.End, from)
		}
		from = b.Pos
		switch b.Kind {
		case Heading:
			if b.Level < 1 || b.Level > 6 {
				t.Fatalf("%q: heading level %d", src, b.Level)
			}
		case List:
			for _, item := range b.Children {
				if item.Kind != Item {
					t.Fatalf("%q: a list holds a %v", src, item.Kind)
				}
			}
		case Table:
			if len(b.Header) != len(b.Align) {
				t.Fatalf("%q: %d heading cells for %d columns", src, len(b.Header), len(b.Align))
			}
			for _, row := range b.Rows {
				if len(row) != len(b.Align) {
					t.Fatalf("%q: a row of %d cells for %d columns", src, len(row), len(b.Align))
				}
			}
		case Paragraph, ThematicBreak, CodeBlock, Quote, Item:
		default:
			t.Fatalf("%q: block of kind %d", src, b.Kind)
		}
		checkBlocks(t, src, b.Children, b.Pos, d+1)
	}
}
