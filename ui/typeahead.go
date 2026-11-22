package ui

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Base UI's typeahead resets: a Select waits useTypeahead's default, a menu
// the shorter TYPEAHEAD_RESET_MS its root passes.
const (
	selectTypeaheadReset = 750 * time.Millisecond
	menuTypeaheadReset   = 500 * time.Millisecond
)

// typeahead is a search of a list's labels by typing, as Base UI's
// useTypeahead runs it: letters typed within reset of one another spell a
// prefix, and the search for it starts just after the row the session
// began on, wrapping round. One letter typed again steps through the rows
// under it instead, unless a row starts with a doubled letter, which the
// pair may be spelling. A letter that finds nothing ends the session.
type typeahead struct {
	typed string    // what the session has spelled, lower-cased
	at    time.Time // when its last character came
	prev  int       // the row the search starts after
	match int       // the row last found
}

// active reports whether a session is under way at now: a Space typed then
// is part of the search and activates nothing.
func (t *typeahead) active(now time.Time, reset time.Duration) bool {
	return t.typed != "" && now.Sub(t.at) <= reset
}

// clear ends the session.
func (t *typeahead) clear() { t.typed = "" }

// typeaheadRows are the rows a typeahead searches: n of them, label
// giving a row's text and available saying whether it can be found.
type typeaheadRows struct {
	n         int
	label     func(int) string
	available func(int) bool
}

// search adds text to the session and returns the row it finds among
// rows, or -1. start is the row a new session begins after, -1 for none.
func (t *typeahead) search(text string, now time.Time, reset time.Duration, start int, rows typeaheadRows) int {
	n := rows.n
	text = strings.ToLower(text)
	if text == "" || n == 0 {
		return -1
	}
	if !t.active(now, reset) {
		t.typed = ""
	}
	if t.typed == "" {
		if strings.TrimSpace(text) == "" {
			return -1 // a space starts no search
		}
		t.prev = start
	} else if t.typed == text && utf8.RuneCountInString(text) == 1 && !rows.doubledInitial() {
		// The same letter again: on to the next row under it.
		t.typed, t.prev = "", t.match
	}
	t.typed, t.at = t.typed+text, now
	from := ((t.prev+1)%n + n) % n
	for k := range n {
		i := (from + k) % n
		if rows.available(i) && strings.HasPrefix(strings.ToLower(rows.label(i)), t.typed) {
			t.match = i
			return i
		}
	}
	if strings.TrimSpace(text) != "" {
		t.typed = ""
	}
	return -1
}

// doubledInitial reports whether an available row's label starts with the
// same letter twice, as "llama" does.
func (rows typeaheadRows) doubledInitial() bool {
	for i := range rows.n {
		if !rows.available(i) {
			continue
		}
		s := strings.ToLower(rows.label(i))
		a, size := utf8.DecodeRuneInString(s)
		if size == 0 || size == len(s) {
			continue
		}
		if b, _ := utf8.DecodeRuneInString(s[size:]); a == b {
			return true
		}
	}
	return false
}
