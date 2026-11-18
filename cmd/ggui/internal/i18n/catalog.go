package i18n

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/shared"
)

// FormNames are a plural message's forms, as a JSON catalog spells them
// and in CLDR's order; i18n.Forms has a field for each, capitalized.
var FormNames = []string{"zero", "one", "two", "few", "many", "other"}

// Forms are a plural message's translations by form name, lowercase.
type Forms map[string]string

// Catalog is the translations of one language, from a composite literal
// of i18n.Catalog or from a JSON file.
type Catalog struct {
	Lang string
	// Path is the file, and Name the same relative to the project, for
	// messages; Line is where a Go catalog's literal starts.
	Path, Name string
	Line       int
	Go         bool
	Messages   map[string]string
	Plurals    map[string]Forms
	// Err is why the catalog cannot be used, such as a Go literal whose
	// Lang is not a literal.
	Err error
	// Partial is a Go catalog with entries whose keys are not literals:
	// its unused keys are not reported, as they may be the keys it was
	// thought to lack.
	Partial bool
	// dynamic are the keys of a Go catalog whose translations are not
	// literals, which count as translated and are not checked.
	dynamic map[string]bool
	// rest are a JSON catalog's other members, kept as they were.
	rest map[string]json.RawMessage
}

// String names the catalog for messages: its language and where it is.
func (c *Catalog) String() string {
	where := c.Name
	if c.Go {
		where = fmt.Sprintf("%s:%d", c.Name, c.Line)
	}
	if c.Lang == "" {
		return where
	}
	return c.Lang + " (" + where + ")"
}

// DefaultGlobs are where JSON catalogs are looked for, relative to the
// project: any directory called i18n or locales.
var DefaultGlobs = []string{"**/i18n/*.json", "**/locales/*.json"}

// Find scans root's Go sources, as Scan does, and reads the JSON catalogs
// below root that match one of globs, slash-separated patterns relative
// to root in which ** stands for any number of directories. The catalogs
// are the Go ones first, then the JSON ones, each in the order of their
// files. A Go file that does not parse makes the error, after the rest
// are read; a JSON file that cannot be read as a catalog is among the
// catalogs, with its Err set.
func Find(root string, globs []string) (*Sources, error) {
	s, err := Scan(root)
	if s == nil {
		return nil, err
	}
	var paths []string
	werr := walk(s.Root, func(p string) error {
		r := rel(s.Root, p)
		for _, g := range globs {
			if matchGlob(g, r) {
				paths = append(paths, p)
				break
			}
		}
		return nil
	})
	if werr != nil {
		return nil, werr
	}
	for _, p := range paths {
		c, rerr := ReadJSON(p)
		if c == nil {
			c = &Catalog{Path: p, Err: rerr}
		}
		c.Name = rel(s.Root, p)
		s.Catalogs = append(s.Catalogs, c)
	}
	return s, err
}

// matchGlob reports whether the slash-separated name matches pattern, as
// path.Match does each element, ** matching any number of them.
func matchGlob(pattern, name string) bool {
	return matchParts(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchParts(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(name); i++ {
				if matchParts(pat[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], name[0]); err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// ValidGlob reports a pattern Find cannot match with.
func ValidGlob(pattern string) error {
	for p := range strings.SplitSeq(pattern, "/") {
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Errorf("%q: %w", pattern, err)
		}
	}
	return nil
}

// jsonCatalog is a JSON catalog's members that are read.
type jsonCatalog struct {
	Lang     string            `json:"lang"`
	Messages map[string]string `json:"messages"`
	Plurals  map[string]Forms  `json:"plurals"`
}

// ReadJSON reads a JSON catalog. One that parses but is not a catalog, as
// one with no lang, is returned with its Err set and the error.
func ReadJSON(path string) (*Catalog, error) {
	data, err := shared.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Catalog{Path: path, Messages: map[string]string{}, Plurals: map[string]Forms{}}
	var rest map[string]json.RawMessage
	if err := json.Unmarshal(data, &rest); err != nil {
		c.Err = fmt.Errorf("not a JSON catalog: %w", err)
		return c, c.Err
	}
	var jc jsonCatalog
	if err := json.Unmarshal(data, &jc); err != nil {
		c.Err = fmt.Errorf("not a catalog: %w", err)
		return c, c.Err
	}
	for _, k := range []string{"lang", "messages", "plurals"} {
		delete(rest, k)
	}
	c.rest = rest
	c.Lang = jc.Lang
	if jc.Messages != nil {
		c.Messages = jc.Messages
	}
	if jc.Plurals != nil {
		c.Plurals = jc.Plurals
	}
	if c.Lang == "" {
		c.Err = errors.New(`not a catalog: it has no "lang"`)
		return c, c.Err
	}
	return c, nil
}

// EncodeJSON writes the catalog as a JSON catalog, every object's keys
// sorted, so that a file written again from the same entries is the same
// file, and a change to it is as small as the change to the entries.
func (c *Catalog) EncodeJSON() ([]byte, error) {
	out := map[string]any{"lang": c.Lang, "messages": c.Messages, "plurals": c.Plurals}
	for k, v := range c.rest {
		out[k] = v
	}
	if c.Messages == nil {
		out["messages"] = map[string]string{}
	}
	if c.Plurals == nil {
		out["plurals"] = map[string]Forms{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteJSON writes the catalog to its Path, replacing the file whole: a
// reader never sees half of it.
func (c *Catalog) WriteJSON() error {
	data, err := c.EncodeJSON()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(c.Path), "."+filepath.Base(c.Path)+".*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	if err := errors.Join(werr, tmp.Close()); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(c.Path); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := shared.Rename(tmp.Name(), c.Path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Add gives the catalog an empty translation of each message of s it
// lacks, and returns the keys added, the messages' and the plural
// messages', sorted.
func (c *Catalog) Add(s *Sources) (messages, plurals []string) {
	if c.Messages == nil {
		c.Messages = map[string]string{}
	}
	if c.Plurals == nil {
		c.Plurals = map[string]Forms{}
	}
	keys, pkeys := s.Keys()
	for _, k := range keys {
		if _, ok := c.Messages[k]; !ok {
			c.Messages[k] = ""
			messages = append(messages, k)
		}
	}
	for _, k := range pkeys {
		if _, ok := c.Plurals[k]; !ok {
			c.Plurals[k] = Forms{"other": ""}
			plurals = append(plurals, k)
		}
	}
	return messages, plurals
}

// Langs returns the languages of the usable catalogs, each once, in the
// order first found.
func Langs(cs []*Catalog) []string {
	var out []string
	for _, c := range cs {
		if c.Err == nil && c.Lang != "" && !slices.Contains(out, c.Lang) {
			out = append(out, c.Lang)
		}
	}
	return out
}
