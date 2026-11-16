package web

import (
	_ "embed"
	"encoding/json"
	"html"
	"strings"
)

// indexHTML is the page: a loading screen that follows the build, then the
// binary, and gets out of the way once the app has painted. Errors from the
// toolchain are shown where they happened rather than only in the terminal.
//
//go:embed index.html
var indexHTML string

// PageOptions says how the page is written.
type PageOptions struct {
	Title string
	// Base is the path the app is served below, such as /app/, written as
	// the page's <base> so that its files load relative to it rather than
	// to a route the page was opened at. Empty writes none, and the page
	// then loads its files relative to wherever it is, which suits a
	// bundle that may be put anywhere and is opened at its own URL.
	Base string
	// Dev is set when ggui dev serves the page.
	Dev bool
	// Size is the binary's size, for the progress bar of a page with no
	// server to ask.
	Size int64
	// Target names what is being built, shown beside the title.
	Target string
}

// Page returns the page.
func Page(o PageOptions) []byte {
	page := indexHTML
	if o.Title != "" {
		page = strings.Replace(page, "<title>ggui</title>", "<title>"+html.EscapeString(o.Title)+"</title>", 1)
		page = strings.Replace(page, `<span class="brand">ggui</span>`,
			`<span class="brand">`+html.EscapeString(o.Title)+`</span>`, 1)
	}
	if o.Base != "" {
		const charset = `<meta charset="utf-8">`
		page = strings.Replace(page, charset, charset+"\n<base href=\""+html.EscapeString(CleanBase(o.Base))+"\">", 1)
	}
	// json.Marshal escapes <, > and &, so nothing in it can close the
	// script element.
	cfg, _ := json.Marshal(struct {
		Dev    bool   `json:"dev"`
		Size   int64  `json:"size,omitempty"`
		Target string `json:"target,omitempty"`
	}{o.Dev, o.Size, o.Target})
	page = strings.Replace(page, "<!--ggui:config-->", "<script>window.gguiConfig = "+string(cfg)+";</script>", 1)
	return []byte(page)
}

// CleanBase returns base as "/" or "/path/".
func CleanBase(base string) string {
	base = "/" + strings.Trim(base, "/") + "/"
	if base == "//" {
		return "/"
	}
	return base
}
