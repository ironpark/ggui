package main

import (
	"bytes"
	"flag"
	"slices"
	"strings"
	"testing"
)

func TestParseInterspersed(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	web := fs.Bool("web", false, "")
	tags := fs.String("tags", "", "")
	pos, rest, err := parseInterspersed(fs, []string{"./app", "--web", "--tags", "a,b", "--", "--app-flag", "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !*web || *tags != "a,b" {
		t.Errorf("flags after the package were not parsed: web=%v tags=%q", *web, *tags)
	}
	if !slices.Equal(pos, []string{"./app"}) || !slices.Equal(rest, []string{"--app-flag", "x"}) {
		t.Errorf("pos = %v, rest = %v", pos, rest)
	}
	if !isSet(fs, "tags") || isSet(fs, "nothing") {
		t.Error("isSet is wrong")
	}
}

func TestSplitTags(t *testing.T) {
	if got := splitTags("a,b c"); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("splitTags = %v", got)
	}
	// An empty --tags clears the configured tags rather than keeping them.
	if got := splitTags(""); got == nil || len(got) != 0 {
		t.Errorf("splitTags(\"\") = %#v, want empty", got)
	}
}

func TestHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"help"}, &out, &errOut); code != 0 {
		t.Fatalf("help exited %d", code)
	}
	root := out.String()
	for _, c := range commands {
		if !strings.Contains(root, c.name) {
			t.Errorf("root help lacks %s", c.name)
		}
		out.Reset()
		if code := run([]string{"help", c.name}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "Usage: ggui "+c.name) {
			t.Errorf("help %s: exit %d\n%s", c.name, code, out.String())
		}
	}
	if code := run([]string{"frobnicate"}, &out, &errOut); code != 2 {
		t.Errorf("an unknown command exited %d, want 2", code)
	}
	if code := run([]string{"build", "--no-such-flag"}, &out, &errOut); code != 2 {
		t.Errorf("an unknown flag exited %d, want 2", code)
	}
}
