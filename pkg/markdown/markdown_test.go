package markdown

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// dump writes blocks in a short notation the expectations below are
// written in: a paragraph is p[...], a heading h2[...], a code block
// code(lang)"..." (with ~open when its fence is unclosed), a quote
// quote{...}, a list ul{...} or ol3{...} (its first number, and ~loose
// when it is not tight), an item li{...} or li[x]{...}, a table
// table(alignments){row|row}; inline text is quoted, and em(), strong(),
// del(), code"", link<url>(), img<url>(), soft and hard are the rest.
func dump(blocks []Block) string {
	var b strings.Builder
	for i, bl := range blocks {
		if i > 0 {
			b.WriteByte(' ')
		}
		dumpBlock(&b, bl)
	}
	return b.String()
}

func dumpBlock(b *strings.Builder, bl Block) {
	switch bl.Kind {
	case Paragraph:
		fmt.Fprintf(b, "p[%s]", dumpInlines(bl.Inlines))
	case Heading:
		fmt.Fprintf(b, "h%d[%s]", bl.Level, dumpInlines(bl.Inlines))
	case ThematicBreak:
		b.WriteString("hr")
	case CodeBlock:
		fmt.Fprintf(b, "code(%s)%q", bl.Info, bl.Code)
		if bl.Open {
			b.WriteString("~open")
		}
	case Quote:
		fmt.Fprintf(b, "quote{%s}", dump(bl.Children))
	case List:
		if bl.Ordered {
			fmt.Fprintf(b, "ol%d", bl.Number)
		} else {
			b.WriteString("ul")
		}
		if !bl.Tight {
			b.WriteString("~loose")
		}
		fmt.Fprintf(b, "{%s}", dump(bl.Children))
	case Item:
		b.WriteString("li")
		switch bl.Task {
		case TaskOpen:
			b.WriteString("[ ]")
		case TaskDone:
			b.WriteString("[x]")
		}
		fmt.Fprintf(b, "{%s}", dump(bl.Children))
	case Table:
		b.WriteString("table(")
		for _, a := range bl.Align {
			b.WriteByte("-lcr"[a])
		}
		b.WriteString("){")
		rows := append([][]Cell{bl.Header}, bl.Rows...)
		for i, row := range rows {
			if i > 0 {
				b.WriteByte('|')
			}
			for j, cell := range row {
				if j > 0 {
					b.WriteByte(',')
				}
				b.WriteString(dumpInlines(cell))
			}
		}
		b.WriteByte('}')
	}
}

func dumpInlines(inlines []Inline) string {
	var parts []string
	for _, in := range inlines {
		switch in.Kind {
		case Text:
			parts = append(parts, fmt.Sprintf("%q", in.Text))
		case CodeSpan:
			parts = append(parts, fmt.Sprintf("code%q", in.Text))
		case Emphasis:
			parts = append(parts, "em("+dumpInlines(in.Children)+")")
		case Strong:
			parts = append(parts, "strong("+dumpInlines(in.Children)+")")
		case Strike:
			parts = append(parts, "del("+dumpInlines(in.Children)+")")
		case Link:
			parts = append(parts, "link<"+in.URL+">("+dumpInlines(in.Children)+")")
		case Image:
			parts = append(parts, "img<"+in.URL+">("+dumpInlines(in.Children)+")")
		case SoftBreak:
			parts = append(parts, "soft")
		case HardBreak:
			parts = append(parts, "hard")
		}
	}
	return strings.Join(parts, " ")
}

func TestParse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"empty", "", ""},
		{"paragraphs", "one\ntwo\n\nthree", `p["one" soft "two"] p["three"]`},
		{"atx headings", "# One\n### Three ###\n####### seven", `h1["One"] h3["Three"] p["####### seven"]`},
		{"setext headings", "Title\n=====\nSub\n---", `h1["Title"] h2["Sub"]`},
		{"thematic breaks", "a\n\n***\n- - -\n___", `p["a"] hr hr hr`},
		{"emphasis", "*a* _b_ **c** __d__ ***e*** ~~f~~ ~g~", `p[em("a") " " em("b") " " strong("c") " " strong("d") " " em(strong("e")) " " del("f") " " del("g")]`},
		{"intraword underscore", "snake_case_name and 2*3*4", `p["snake_case_name and 2" em("3") "4"]`},
		{"rule of three", "*foo**bar**baz*", `p[em("foo" strong("bar") "baz")]`},
		{"unmatched emphasis is text", "**bold and *more", `p["**bold and *more"]`},
		{"code spans", "`a` `` b`c `` ``` x", "p[code\"a\" \" \" code\"b`c\" \" ``` x\"]"},
		{"escapes and entities", `\*not\* &amp; &copy; &#65; &bogus; \q`, `p["*not* & © A &bogus; \\q"]`},
		{"hard breaks", "a  \nb\\\nc", `p["a" hard "b" hard "c"]`},
		{"links", `[text](http://x.test "T") [<a b>](<a b>) [empty]() [no]`, `p[link<http://x.test>("text") " " link<a b>("<a b>") " " link<>("empty") " [no]"]`},
		{"emphasis in links", "[*a*](u) *[b](v)*", `p[link<u>(em("a")) " " em(link<v>("b"))]`},
		{"no links in links", "[a [b](c) d](e)", `p["[a " link<c>("b") " d](e)"]`},
		{"image", "![alt *text*](pic.png)", `p[img<pic.png>("alt " em("text"))]`},
		{"autolinks", "<https://x.test/a?b> <me@x.test> <not a link>", `p[link<https://x.test/a?b>("https://x.test/a?b") " " link<mailto:me@x.test>("me@x.test") " <not a link>"]`},
		{"extended autolinks", "see www.x.test/a, https://y.test/(b)). and http://localhost", `p["see " link<http://www.x.test/a>("www.x.test/a") ", " link<https://y.test/(b)>("https://y.test/(b)") "). and http://localhost"]`},
		{"raw html is text", "<b>bold</b> <script>x()</script>", `p["<b>bold</b> <script>x()</script>"]`},
		{"html block is text", "<div>\nhi\n</div>", `p["<div>" soft "hi" soft "</div>"]`},
		{"fenced code", "```go\nfunc main() {\n\tprintln()\n}\n```\nafter", `code(go)"func main() {\n\tprintln()\n}" p["after"]`},
		{"tilde fence and longer close", "~~~\na\n```\n~~~~~", `code()"a\n` + "```" + `"`},
		{"indented fence", "  ```\n  a\n    b\n c\n  ```", `code()"a\n  b\nc"`},
		{"unclosed fence", "```py\nprint(1)\n", `code(py)"print(1)"~open`},
		{"indented code", "    a\n\n    b\n\nc", `code()"a\n\nb" p["c"]`},
		{"block quote", "> a\nlazy\n> > b\n\n> c", `quote{p["a" soft "lazy"] quote{p["b"]}} quote{p["c"]}`},
		{"bullet list", "- a\n- b\n* c", `ul{li{p["a"]} li{p["b"]}} ul{li{p["c"]}}`},
		{"ordered list", "3. a\n4. b\n1) c", `ol3{li{p["a"]} li{p["b"]}} ol1{li{p["c"]}}`},
		{"loose list", "- a\n\n- b", `ul~loose{li{p["a"]} li{p["b"]}}`},
		{"nested lists", "- a\n  - b\n    1. c\n- d", `ul{li{p["a"] ul{li{p["b"] ol1{li{p["c"]}}}}} li{p["d"]}}`},
		{"list item lazy continuation", "- a\nlazy\n- b", `ul{li{p["a" soft "lazy"]} li{p["b"]}}`},
		{"list item with code", "1.  item\n\n        code", `ol1~loose{li{p["item"] code()"code"}}`},
		{"only 1 interrupts a paragraph", "a\n2. b\n1. c", `p["a" soft "2. b"] ol1{li{p["c"]}}`},
		{"task list", "- [ ] todo\n- [x] done\n- [X] *also*\n- [y] no\n- [ ]", `ul{li[ ]{p["todo"]} li[x]{p["done"]} li[x]{p[em("also")]} li{p["[y] no"]} li{p["[ ]"]}}`},
		{"tables", "| a | b | c |\n|:--|:-:|--:|\n| 1 | **2** |\n| x | y | z | w |", `table(lcr){"a","b","c"|"1",strong("2"),|"x","y","z"}`},
		{"table without outer pipes", "a | b\n--- | ---\nc | d\n\nafter", `table(--){"a","b"|"c","d"} p["after"]`},
		{"table pipes in code and escapes", "| a | b |\n|---|---|\n| `x\\|y` | p\\|q |", "table(--){\"a\",\"b\"|code\"x|y\",\"p|q\"}"},
		{"table after paragraph lines", "intro\n| a |\n| - |\n| b |", `p["intro"] table(-){"a"|"b"}`},
		{"table needs matching cells", "| a | b |\n| - |", `p["| a | b |" soft "| - |"]`},
		{"table ends at a block start", "| a |\n|---|\n| b |\n> q", `table(-){"a"|"b"} quote{p["q"]}`},
		{"crlf", "# T\r\n\r\n- a\r\n- b\r\n\r\n```\r\nx\r\n```\r\n", `h1["T"] ul{li{p["a"]} li{p["b"]}} code()"x"`},
		{"lone cr", "a\rb\r\rc", `p["a" soft "b"] p["c"]`},
		{"tabs", "-\tone\n\n\ttwo\n\nx\n\n\tcode\tkept\n\t\tdeeper", `ul~loose{li{p["one"] p["two"]}} p["x"] code()"code\tkept\n\tdeeper"`},
		{"tabs in a fence", "```\n\tfunc() {\n\t\treturn\n\t}\n```", `code()"\tfunc() {\n\t\treturn\n\t}"`},
		{"nul", "a\x00b", "p[\"a�b\"]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := dump(Parse(tc.src)); got != tc.want {
				t.Errorf("Parse(%q)\n got %s\nwant %s", tc.src, got, tc.want)
			}
		})
	}
}

// A stream cut mid-token shows what it has so far, without the syntax that
// waits for its other half.
func TestParseStream(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"open strong", "say **bol", `p["say " strong("bol")]`},
		{"open emphasis inside strong", "**a *b", `p[strong("a " em("b"))]`},
		{"triple", "***x", `p[em(strong("x"))]`},
		{"delimiters at the cut", "say **", `p["say "]`},
		{"closed strong at the cut", "say **this**", `p["say " strong("this")]`},
		{"open strike", "~~gone", `p[del("gone")]`},
		{"open code span", "run `go te", `p["run " code"go te"]`},
		{"backticks at the cut", "run ``", `p["run "]`},
		{"link without its destination", "[docs](https://exa", `p[link<>("docs")]`},
		{"link text at the cut", "see [do", `p["see do"]`},
		{"brackets at the cut", "see [1]", `p["see [1]"]`},
		{"open fence", "```go\nfunc", `code(go)"func"~open`},
		{"only the last block", "**a\n\n**b", `p["**a"] p[strong("b")]`},
		{"a closed paragraph is whole", "**a\n\n", `p["**a"]`},
		{"an open paragraph at a line ending", "**a\n", `p[strong("a")]`},
		{"in a list", "- one\n- **tw", `ul{li{p["one"]} li{p[strong("tw")]}}`},
		{"table delimiter row arriving", "| a | b |\n|---|-", `table(--){"a","b"}`},
		{"table cell", "| a |\n|---|\n| *x", `table(-){"a"|em("x")}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := dump(ParseStream(tc.src)); got != tc.want {
				t.Errorf("ParseStream(%q)\n got %s\nwant %s", tc.src, got, tc.want)
			}
		})
	}
	// Parse reads the same cut as a finished document.
	if got, want := dump(Parse("say **bol")), `p["say **bol"]`; got != want {
		t.Errorf("Parse got %s, want %s", got, want)
	}
}

func TestBlockPositions(t *testing.T) {
	t.Parallel()
	src := "# A\n\npara\nmore\n\n- x\n- y\n"
	blocks := Parse(src)
	var got []string
	for _, b := range blocks {
		got = append(got, src[b.Pos:b.End])
	}
	if want := []string{"# A", "para\nmore", "- x\n- y"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("blocks span %q, want %q", got, want)
	}
}

func TestEqualIgnoresPositions(t *testing.T) {
	t.Parallel()
	a, b := Parse("x\n\n- *a*\n- b"), Parse("y\n\n\n\n- *a*\n- b")
	if !Equal(a[1], b[1]) {
		t.Fatal("the same list at another offset is not Equal")
	}
	if Equal(a[0], b[0]) {
		t.Fatal("different paragraphs are Equal")
	}
	if Equal(Parse("- a\n- b")[0], Parse("- a\n\n- b")[0]) {
		t.Fatal("a tight and a loose list are Equal")
	}
}

func TestLangAndPlainText(t *testing.T) {
	t.Parallel()
	b := Parse("```go linenums\nx\n```")[0]
	if b.Lang() != "go" || b.Info != "go linenums" {
		t.Fatalf("Lang = %q of info %q", b.Lang(), b.Info)
	}
	p := Parse("a *b* `c`  \n[d](e)")[0]
	if got := PlainText(p.Inlines); got != "a b c\nd" {
		t.Fatalf("PlainText = %q", got)
	}
}

// Deep nesting stops at maxDepth rather than building a block per marker.
func TestDeepNestingIsBounded(t *testing.T) {
	t.Parallel()
	blocks := Parse(strings.Repeat(">", 10000) + " x")
	d := 0
	for len(blocks) > 0 && blocks[0].Kind == Quote {
		blocks = blocks[0].Children
		d++
	}
	if d == 0 || d > maxDepth {
		t.Fatalf("%d nested quotes", d)
	}
	blocks = Parse(strings.Repeat("- ", 5000) + "x")
	if len(blocks) != 1 {
		t.Fatalf("%d blocks", len(blocks))
	}
}

// A document reparsed at every byte it grows by, as a stream arrives, is
// the document parsed whole at each.
func TestReparseFollowsAGrowingSource(t *testing.T) {
	t.Parallel()
	doc := benchDoc()[:3000] + "\r\n> - quote\n> lazy\nSetext\n---\n| x |\n|:-:|\n"
	for _, stream := range []bool{false, true} {
		var prev []Block
		prevSrc := ""
		for i := 0; i <= len(doc); i++ {
			src := doc[:i]
			got := Reparse(prev, prevSrc, src, stream)
			if want := parse(src, stream); dump(got) != dump(want) {
				t.Fatalf("stream %v at %d: reparsed\n%s\nparsed\n%s", stream, i, dump(got), dump(want))
			}
			prev, prevSrc = got, src
		}
	}
}

// Inputs that make a careless parser quadratic parse in about the time of
// ordinary text of their length.
func TestPathologicalInputs(t *testing.T) {
	t.Parallel()
	for _, src := range []string{
		strings.Repeat("[", 20000) + strings.Repeat("]", 20000),
		strings.Repeat("*a ", 20000), strings.Repeat("a*", 20000), strings.Repeat("_a _", 10000),
		strings.Repeat("`", 5000) + strings.Repeat("a`` ", 5000), strings.Repeat("[a](", 10000),
		strings.Repeat("&amp;a", 10000), strings.Repeat("<a", 20000), strings.Repeat("> ", 20000),
		strings.Repeat("- ", 20000), strings.Repeat("a\n", 20000), strings.Repeat("| a ", 5000) + "\n|" + strings.Repeat("-|", 5000),
	} {
		start := time.Now()
		ParseStream(src)
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("parsing %q… (%d bytes) took %v", src[:min(len(src), 20)], len(src), d)
		}
	}
}

func BenchmarkParse(b *testing.B) {
	doc := benchDoc()
	b.SetBytes(int64(len(doc)))
	for b.Loop() {
		Parse(doc)
	}
}

// BenchmarkParseStream parses a reply a few bytes longer each time, as a
// widget following a stream does at every piece that arrives.
func BenchmarkParseStream(b *testing.B) {
	doc := benchDoc()
	b.SetBytes(int64(len(doc)))
	for b.Loop() {
		ParseStream(doc[:len(doc)*3/4])
	}
}

// BenchmarkReparseStream follows a reply as it streams in, a few bytes at
// a time, reparsing only what the new bytes may change.
func BenchmarkReparseStream(b *testing.B) {
	doc := benchDoc()
	b.SetBytes(int64(len(doc)))
	for b.Loop() {
		var prev []Block
		for i := 16; i <= len(doc); i += 16 {
			prev = Reparse(prev, doc[:i-16], doc[:i], true)
		}
	}
}

// benchDoc is a long chat reply: headings, prose with inline markup, lists,
// code and a table, about 12 KB.
func benchDoc() string {
	section := "## Section\n\nSome *emphasis*, **strong** text, `code`, and a [link](https://example.com) " +
		"with ~~strike~~ in a paragraph that goes on for a while, as an answer's paragraphs do.\n" +
		"It wraps onto a second line here, with www.example.com in it.\n\n" +
		"- first item\n- second item with **bold**\n  - nested item\n- [x] done\n\n" +
		"1. one\n2. two\n\n" +
		"```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n\n" +
		"| Name | Value |\n|:-----|------:|\n| a | 1 |\n| b | 2 |\n\n> A quote with *style*.\n\n"
	return strings.Repeat(section, 20)
}
