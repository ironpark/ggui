package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/i18n"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

var i18nCmd = &command{
	name:    "i18n",
	args:    "check|extract [dir]",
	summary: "check the translation catalogs against the messages, or add what they lack",
	help: `I18n compares the messages the Go sources pass to the ggui i18n
package with the catalogs that translate them. The project is dir, or
the directory of the ggui.toml found from the current directory, or the
current directory; its Go files are read but for tests, vendor and
testdata directories, and other modules nested inside.

A message is the string literal given to i18n.T, i18n.N (its singular
form, which keys a plural message) or a Localizer's S and N, followed by
" [context]" when the call is followed by .Context("context"). A method
called S or N counts in a file that imports the i18n package when its
message is a literal, whatever its type. A message that is not a literal
cannot be extracted, and is listed.

Catalogs are the i18n.Catalog composite literals in the Go files, and the
JSON files --catalog matches, by default any *.json in a directory called
i18n or locales: {"lang": "ko", "messages": {...}, "plurals": {...}}.

  check    prints a line for each catalog and each problem, as doctor
           does: missing translations, an empty translation among them,
           and translations whose format verbs differ from the source's,
           or whose plural forms lack other, are errors; unused keys and
           a difference in leading or trailing space are warnings. It
           exits with status 1 when there is an error. --allow-missing
           makes missing translations warnings.
  extract  adds each missing message to each JSON catalog, with an empty
           translation, rewriting the file with its keys sorted, and
           prints the entries each Go catalog lacks, ready to paste into
           its Messages and Plurals. With --lang it makes a new JSON
           catalog of every message instead, at --out.`,
	flags: func(fs *flag.FlagSet) func(context.Context, []string, []string) error {
		var globs globList
		fs.Var(&globs, "catalog", "read the JSON catalogs matching `glob`, relative to the project, ** standing for any directories; repeat it for more (default **/i18n/*.json and **/locales/*.json)")
		allowMissing := fs.Bool("allow-missing", false, "with check, count missing translations as warnings")
		lang := fs.String("lang", "", "with extract, make a new JSON catalog for the `language`, such as ko, holding every message")
		out := fs.String("out", "", "with extract --lang, the new catalog's `file`; locales/LANG.json in the project by default")
		return func(ctx context.Context, pos, rest []string) error {
			if len(pos) == 0 || len(pos) > 2 || len(rest) > 0 {
				fmt.Fprintln(os.Stderr, "ggui i18n: takes check or extract, and a directory")
				return errUsage
			}
			sub := pos[0]
			if sub != "check" && sub != "extract" {
				fmt.Fprintf(os.Stderr, "ggui i18n: %q is not check or extract\n", sub)
				return errUsage
			}
			if sub == "check" && (isSet(fs, "lang") || isSet(fs, "out")) {
				return errors.New("--lang and --out make a catalog: use them with ggui i18n extract")
			}
			if sub == "extract" && isSet(fs, "allow-missing") {
				return errors.New("--allow-missing is for ggui i18n check")
			}
			if *out != "" && *lang == "" {
				return errors.New("--out names the catalog --lang makes: give --lang too")
			}
			for _, g := range globs {
				if err := i18n.ValidGlob(g); err != nil {
					return fmt.Errorf("--catalog %w", err)
				}
			}
			if len(globs) == 0 {
				globs = i18n.DefaultGlobs
			}
			root := ""
			if len(pos) == 2 {
				root = pos[1]
			} else {
				proj, err := project.Open(".", "", "")
				if err != nil {
					return err
				}
				root = proj.Root
			}
			s := ui.Stderr.Step("Reading the sources", ui.QuietStart())
			src, err := i18n.Find(root, globs)
			s.Stop()
			if src == nil {
				return err
			}
			if err != nil {
				return fmt.Errorf("the sources must parse to be read:\n%w", err)
			}
			if sub == "check" {
				if printCheck(os.Stdout, src, i18n.Options{AllowMissing: *allowMissing}) {
					return errFailed
				}
				return nil
			}
			return extract(os.Stdout, src, *lang, *out)
		}
	},
}

// globList is a flag that may be given more than once.
type globList []string

func (g *globList) String() string     { return strings.Join(*g, ", ") }
func (g *globList) Set(v string) error { *g = append(*g, v); return nil }

// checkLine is a line of the report.
type checkLine struct {
	status, name, detail, fix string
}

// fixes say what to do about each kind of problem, once per catalog.
func fixFor(c *i18n.Catalog, k i18n.Kind) string {
	switch k {
	case i18n.Missing:
		if c.Go {
			return "ggui i18n extract prints the entries to add, to translate"
		}
		return "ggui i18n extract adds them, empty, to translate"
	case i18n.Verbs:
		return "a translation formats the arguments its source does, each with the same verb; %[2]s and %[1]d reorder them"
	case i18n.NoOther:
		return "add the other form, which every language's plural rules fall back to"
	case i18n.Unused:
		return "remove them, unless a message that is not a literal passes them"
	case i18n.Invalid:
		if c.Go {
			return "write the catalog's Lang as a string literal and its fields by name"
		}
		return `a JSON catalog is {"lang": "ko", "messages": {...}, "plurals": {...}}`
	}
	return ""
}

// printCheck writes the report of ggui i18n check, in doctor's form, and
// reports whether there was an error.
func printCheck(w io.Writer, src *i18n.Sources, o i18n.Options) bool {
	var lines []checkLine
	for _, sk := range src.Skipped {
		lines = append(lines, checkLine{status: "warn", name: "sources",
			detail: fmt.Sprintf("%s at %s: the message is not a string literal, so it cannot be checked", sk.Call, sk.Pos)})
	}
	keys, pkeys := src.Keys()
	if len(src.Catalogs) == 0 {
		lines = append(lines, checkLine{status: "warn", name: "catalogs",
			detail: fmt.Sprintf("none found: no i18n.Catalog literal in the Go files and no JSON file matching the globs (%d messages, %d plural)", len(keys), len(pkeys)),
			fix:    "ggui i18n extract --lang ko makes one"})
	}
	failed := false
	for _, c := range src.Catalogs {
		ps := i18n.Check(c, src, o)
		if len(ps) == 0 {
			lines = append(lines, checkLine{status: "ok", name: c.String(),
				detail: fmt.Sprintf("%d messages and %d plural messages translated", len(keys), len(pkeys))})
			continue
		}
		for i, p := range ps {
			l := checkLine{status: "warn", name: c.String(), detail: p.Detail}
			if p.Error {
				l.status = "fail"
				failed = true
			}
			// The fix comes once, under the last of its kind.
			if i == len(ps)-1 || ps[i+1].Kind != p.Kind {
				l.fix = fixFor(c, p.Kind)
			}
			lines = append(lines, l)
		}
	}
	printLines(w, lines)
	return failed
}

// printLines writes lines as doctor writes its checks, with a count at
// the end.
func printLines(w io.Writer, lines []checkLine) {
	o := ui.For(w)
	statusWidth := 4
	status := func(s string) string { return fmt.Sprintf("%-4s", s) }
	if o.Terminal() {
		statusWidth = 6
		status = func(s string) string {
			text := fmt.Sprintf("%-4s", s)
			switch s {
			case "ok":
				return o.Green("✓ " + text)
			case "warn":
				return o.Yellow("! " + text)
			default:
				return o.Red("✗ " + text)
			}
		}
	}
	width := 0
	for _, l := range lines {
		width = max(width, len(l.name))
	}
	var fails, warns int
	for _, l := range lines {
		fmt.Fprintf(w, "%s  %-*s  %s\n", status(l.status), width, l.name, l.detail)
		if l.fix != "" {
			fmt.Fprintf(w, "%-*s  %-*s  %s %s\n", statusWidth, "", width, "", o.Cyan("→"), l.fix)
		}
		switch l.status {
		case "fail":
			fails++
		case "warn":
			warns++
		}
	}
	fmt.Fprintln(w)
	switch {
	case fails > 0:
		fmt.Fprintln(w, o.Red(fmt.Sprintf("%d errors, %d warnings.", fails, warns)))
	case warns > 0:
		fmt.Fprintln(w, o.Yellow(fmt.Sprintf("No errors; %d warnings.", warns)))
	default:
		fmt.Fprintln(w, o.Green("Every catalog translates every message."))
	}
}

// extract adds what each catalog lacks, or with lang makes a new one at
// out. JSON catalogs are written; the entries a Go catalog lacks are
// printed on w, as Go, for its author to paste.
func extract(w io.Writer, src *i18n.Sources, lang, out string) error {
	log := ui.Stderr
	for _, sk := range src.Skipped {
		log.Warn("cannot extract the message of %s at %s: it is not a string literal", sk.Call, sk.Pos)
	}
	keys, pkeys := src.Keys()
	if lang != "" {
		if out == "" {
			out = filepath.Join(src.Root, "locales", lang+".json")
		} else if abs, err := filepath.Abs(out); err == nil {
			out = abs
		}
		if _, err := os.Stat(out); err == nil {
			return fmt.Errorf("%s exists; ggui i18n extract without --lang adds the messages it lacks", out)
		}
		c := &i18n.Catalog{Lang: lang, Path: out}
		c.Add(src)
		if err := c.WriteJSON(); err != nil {
			return err
		}
		log.Success("Wrote %s: %d messages and %d plural messages to translate into %s", log.Cyan(shortPath(out)), len(keys), len(pkeys), lang)
		return nil
	}
	if len(src.Catalogs) == 0 {
		return errors.New("no catalogs found: no i18n.Catalog literal in the Go files and no JSON file matching the globs; ggui i18n extract --lang ko --out locales/ko.json makes one")
	}
	o := ui.For(w)
	for _, c := range src.Catalogs {
		if c.Err != nil {
			log.Warn("%s: %v; left as it is", c, c.Err)
			continue
		}
		added, padded := c.Add(src)
		if len(added)+len(padded) == 0 {
			log.Info("%s has every message", c)
			continue
		}
		if !c.Go {
			if err := c.WriteJSON(); err != nil {
				return err
			}
			log.Success("Added %s to %s, to translate", count(len(added), len(padded)), log.Cyan(c.Name))
			continue
		}
		log.Info("%s lacks %s; its entries follow, to paste", c, count(len(added), len(padded)))
		fmt.Fprintln(w, o.Dim(fmt.Sprintf("// %s", c)))
		if len(added) > 0 {
			fmt.Fprintln(w, o.Dim("// Messages:"))
			for _, k := range added {
				fmt.Fprintf(w, "\t%s: \"\",\n", strconv.Quote(k))
			}
		}
		if len(padded) > 0 {
			fmt.Fprintln(w, o.Dim("// Plurals:"))
			for _, k := range padded {
				fmt.Fprintf(w, "\t%s: {Other: \"\"},\n", strconv.Quote(k))
			}
		}
	}
	return nil
}

// count says how many messages and plural messages there are.
func count(messages, plurals int) string {
	var parts []string
	if messages > 0 {
		parts = append(parts, plural(messages, "message"))
	}
	if plurals > 0 {
		parts = append(parts, plural(plurals, "plural message"))
	}
	return strings.Join(parts, " and ")
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return fmt.Sprintf("%d %ss", n, what)
}
