package main

import (
	"fmt"
	"strings"
	"unicode"
)

// Accept one statement, respecting SQL comments and quoted identifiers/literals.
// The client owns transactions so explicit transaction commands are rejected.
func validateStatement(sql string) error {
	var first strings.Builder
	started, ended, wordDone := false, false, false
	for i := 0; i < len(sql); {
		c := sql[i]
		if unicode.IsSpace(rune(c)) {
			if first.Len() > 0 {
				wordDone = true
			}
			i++
			continue
		}
		if c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			i += 2
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			i += 2
			for i+1 < len(sql) && !(sql[i] == '*' && sql[i+1] == '/') {
				i++
			}
			if i+1 >= len(sql) {
				return fmt.Errorf("unterminated SQL comment")
			}
			i += 2
			continue
		}
		if c == ';' {
			ended = true
			i++
			continue
		}
		if ended {
			return fmt.Errorf("run one SQL statement at a time")
		}
		started = true
		if c == '\'' || c == '"' || c == '`' || c == '[' {
			wordDone = true
			end := c
			if c == '[' {
				end = ']'
			}
			i++
			closed := false
			for i < len(sql) {
				if sql[i] == end {
					if end != ']' && i+1 < len(sql) && sql[i+1] == end {
						i += 2
						continue
					}
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return fmt.Errorf("unterminated quoted SQL value")
			}
			continue
		}
		if !wordDone {
			if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' {
				first.WriteByte(c)
			} else {
				wordDone = true
			}
		}
		i++
	}
	if !started {
		return fmt.Errorf("enter a SQL statement")
	}
	switch strings.ToUpper(first.String()) {
	case "BEGIN", "COMMIT", "END", "ROLLBACK", "SAVEPOINT", "RELEASE":
		return fmt.Errorf("transactions are managed by the client; run a query or write statement")
	}
	return nil
}
