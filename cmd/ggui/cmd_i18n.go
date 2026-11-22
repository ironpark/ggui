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
message is a literal, whatever its type. A constant of the package, as
const msg = "Saved", counts as its literal. A message that is not a
literal cannot be extracted, and is listed.

The app's own functions that pass a message on, such as
func tr(key string, args ...any) string, are named with --func, and
those like i18n.N with --plural, or in ggui.toml's [i18n] funcs and
plurals; the flags add to the file's. tr matches a call of tr and of any
method called tr; ui.T, a call of T on an import of a package called ui,
or on anything called ui. --func tr:1 takes the message from the second
argument, --plural trn:0,1 the singular and plural forms from the first
two. A message the wrapper's own body passes on from its parameters is
not listed.

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
		f := newI18nFlags(fs)
		return f.run
	},
}

// i18nFlags are the flags of ggui i18n, both subcommands'.
type i18nFlags struct {
	globs        globList
	funcs        globList
	plurals      globList
	allowMissing *bool
	lang         *string
	out          *string
	isSet        func(string) bool
}

func newI18nFlags(fs *flag.FlagSet) *i18nFlags {
	f := &i18nFlags{isSet: func(name string) bool { return isSet(fs, name) }}
	fs.Var(&f.funcs, "func", "read the calls of the app's own `function`, which passes its message on as i18n.T does: NAME, a function or any method called so, or pkg.Name, then :POS, the message argument's index, when it is not 0; repeat it for more, added to ggui.toml's i18n.funcs")
	fs.Var(&f.plurals, "plural", "read the calls of the app's own `function` as i18n.N's: NAME or pkg.Name, then :ONE,OTHER, the indexes of the singular and the plural form, when they are not 1,2; repeat it for more, added to ggui.toml's i18n.plurals")
	fs.Var(&f.globs, "catalog", "read the JSON catalogs matching `glob`, relative to the project, ** standing for any directories; repeat it for more (default **/i18n/*.json and **/locales/*.json)")
	f.allowMissing = fs.Bool("allow-missing", false, "with check, count missing translations as warnings")
	f.lang = fs.String("lang", "", "with extract, make a new JSON catalog for the `language`, such as ko, holding every message")
	f.out = fs.String("out", "", "with extract --lang, the new catalog's `file`; locales/LANG.json in the project by default")
	return f
}

// run is the i18n command once its flags are parsed.
func (f *i18nFlags) run(ctx context.Context, pos, rest []string) error {
	sub, flagFuncs, err := f.check(pos, rest)
	if err != nil {
		return err
	}
	src, err := f.find(pos, flagFuncs)
	if err != nil {
		return err
	}
	if sub == "check" {
		if printCheck(os.Stdout, src, i18n.Options{AllowMissing: *f.allowMissing}) {
			return errFailed
		}
		return nil
	}
	return extract(os.Stdout, src, *f.lang, *f.out)
}

// check reads the arguments and flags, returning the subcommand and the
// message-passing functions they name.
func (f *i18nFlags) check(pos, rest []string) (string, []i18n.Func, error) {
	sub, err := f.checkArgs(pos, rest)
	if err != nil {
		return "", nil, err
	}
	if err := f.checkFlags(sub); err != nil {
		return "", nil, err
	}
	flagFuncs, err := i18n.ParseFuncs(f.funcs, f.plurals)
	if err != nil {
		return "", nil, fmt.Errorf("--func and --plural take NAME, pkg.Name, NAME:POS or for --plural NAME:ONE,OTHER:\n%w", err)
	}
	return sub, flagFuncs, nil
}

// checkArgs reads the arguments: the subcommand and the directory.
func (f *i18nFlags) checkArgs(pos, rest []string) (string, error) {
	if len(pos) == 0 || len(pos) > 2 || len(rest) > 0 {
		fmt.Fprintln(os.Stderr, "ggui i18n: takes check or extract, and a directory")
		return "", errUsage
	}
	sub := pos[0]
	if sub != "check" && sub != "extract" {
		fmt.Fprintf(os.Stderr, "ggui i18n: %q is not check or extract\n", sub)
		return "", errUsage
	}
	return sub, nil
}

// checkFlags checks the flags against the subcommand and the catalog
// globs.
func (f *i18nFlags) checkFlags(sub string) error {
	if sub == "check" && (f.isSet("lang") || f.isSet("out")) {
		return errors.New("--lang and --out make a catalog: use them with ggui i18n extract")
	}
	if sub == "extract" && f.isSet("allow-missing") {
		return errors.New("--allow-missing is for ggui i18n check")
	}
	if *f.out != "" && *f.lang == "" {
		return errors.New("--out names the catalog --lang makes: give --lang too")
	}
	if *f.lang != "" && !validLang(*f.lang) {
		return fmt.Errorf("--lang %q must be a language tag using letters, digits, hyphens or underscores", *f.lang)
	}
	for _, g := range f.globs {
		if err := i18n.ValidGlob(g); err != nil {
			return fmt.Errorf("--catalog %w", err)
		}
	}
	if len(f.globs) == 0 {
		f.globs = i18n.DefaultGlobs
	}
	return nil
}

// find reads the sources of the project pos names, with the catalogs
// and the message-passing functions.
func (f *i18nFlags) find(pos []string, flagFuncs []i18n.Func) (*i18n.Sources, error) {
	dir := "."
	if len(pos) == 2 {
		dir = pos[1]
	}
	proj, err := project.Open(dir, "", "")
	if err != nil {
		return nil, err
	}
	// A directory given is the project, though its ggui.toml may
	// be above it.
	root := proj.Root
	if len(pos) == 2 {
		root = dir
	}
	// ggui.toml was checked as it was read.
	wrappers, _ := i18n.ParseFuncs(proj.Config.I18n.Funcs, proj.Config.I18n.Plurals)
	wrappers = append(wrappers, flagFuncs...)
	s := ui.Stderr.Step("Reading the sources", ui.QuietStart())
	src, err := i18n.Find(root, f.globs, wrappers...)
	s.Stop()
	if src == nil {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("the sources must parse to be read:\n%w", err)
	}
	return src, nil
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
		return extractLang(src, lang, out, len(keys), len(pkeys))
	}
	if len(src.Catalogs) == 0 {
		return errors.New("no catalogs found: no i18n.Catalog literal in the Go files and no JSON file matching the globs; ggui i18n extract --lang ko --out locales/ko.json makes one")
	}
	for _, c := range src.Catalogs {
		if c.Err != nil {
			log.Warn("%s: %v; left as it is", c, c.Err)
			continue
		}
		if err := extractCatalog(w, src, c); err != nil {
			return err
		}
	}
	return nil
}

// extractLang makes a new JSON catalog for lang at out, holding every
// message, and says what it wrote.
func extractLang(src *i18n.Sources, lang, out string, messages, plurals int) error {
	if !validLang(lang) {
		return fmt.Errorf("--lang %q must be a language tag using letters, digits, hyphens or underscores", lang)
	}
	if out == "" {
		out = filepath.Join(src.Root, "locales", lang+".json")
	} else if abs, err := filepath.Abs(out); err == nil {
		out = abs
	}
	if _, err := os.Stat(out); err == nil { //nolint:gosec // G703: the path is the CLI user's own argument or build output
		return fmt.Errorf("%s exists; ggui i18n extract without --lang adds the messages it lacks", out)
	}
	c := &i18n.Catalog{Lang: lang, Path: out}
	c.Add(src)
	if err := c.WriteJSON(); err != nil {
		return err
	}
	ui.Stderr.Success("Wrote %s: %d messages and %d plural messages to translate into %s", ui.Stderr.Cyan(shortPath(out)), messages, plurals, lang)
	return nil
}

// extractCatalog adds to c what it lacks: a JSON catalog is written, a
// Go catalog's missing entries are printed on w to paste.
func extractCatalog(w io.Writer, src *i18n.Sources, c *i18n.Catalog) error {
	log := ui.Stderr
	added, padded := c.Add(src)
	if len(added)+len(padded) == 0 {
		log.Info("%s has every message", c)
		return nil
	}
	if !c.Go {
		if err := c.WriteJSON(); err != nil {
			return err
		}
		log.Success("Added %s to %s, to translate", count(len(added), len(padded)), log.Cyan(c.Name))
		return nil
	}
	log.Info("%s lacks %s; its entries follow, to paste", c, count(len(added), len(padded)))
	o := ui.For(w)
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

// validLang reports whether s is a safe language tag for filenames like
// locales/<lang>.json: starts with an ASCII letter and contains only ASCII
// letters, digits, hyphens, and underscores.
func validLang(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		digit := c >= '0' && c <= '9'
		if i == 0 && !letter {
			return false
		}
		if !letter && !digit && c != '-' && c != '_' {
			return false
		}
	}
	return true
}
