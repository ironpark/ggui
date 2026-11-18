package linux

import (
	"strings"
)

// The desktop entry is written to the Desktop Entry Specification,
// version 1.5:
// https://specifications.freedesktop.org/desktop-entry-spec/latest/

// escapeString escapes a value of type string or localestring: a
// backslash, newline, tab and carriage return each as their escape, and a
// space at either end as \s, since a reader trims the spaces around a
// value.
func escapeString(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		case ' ':
			if i == 0 || i == len(s)-1 {
				b.WriteString(`\s`)
			} else {
				b.WriteByte(' ')
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// escapeList writes a list value: each element escaped as a string, with
// a semicolon inside it as \;, and each followed by the semicolon that
// separates them, as the specification asks of the last one too.
func escapeList(elems []string) string {
	var b strings.Builder
	for _, e := range elems {
		b.WriteString(strings.ReplaceAll(escapeString(e), ";", `\;`))
		b.WriteByte(';')
	}
	return b.String()
}

// execReserved are the characters an argument of Exec must be quoted for.
const execReserved = " \t\n\"'\\><~|&;$*?#()`"

// quoteExecArg writes one argument of the Exec key, before the value as a
// whole is escaped as a string: a percent sign doubled, as the field codes
// take it, and an argument with a reserved character in double quotes,
// with a double quote, backtick, dollar sign and backslash inside escaped
// by a backslash. An empty argument is "".
func quoteExecArg(arg string) string {
	arg = strings.ReplaceAll(arg, "%", "%%")
	if arg != "" && !strings.ContainsAny(arg, execReserved) {
		return arg
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range arg {
		if strings.ContainsRune("\"`$\\", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// execValue writes the Exec value: the program, by its name on PATH, and %U
// when it opens files or URLs, escaped as a string over the quoting.
func execValue(program string, opens bool) string {
	v := quoteExecArg(program)
	if opens {
		v += " %U"
	}
	return escapeString(v)
}

// DesktopEntry returns the application's .desktop file, which menus and
// launchers list it from. hasIcon says whether the icons are installed
// under the ID, for the Icon key.
func DesktopEntry(a App, hasIcon bool) []byte {
	var b strings.Builder
	line := func(k, v string) {
		b.WriteString(k + "=" + v + "\n")
	}
	b.WriteString("[Desktop Entry]\n")
	line("Type", "Application")
	line("Version", "1.5")
	line("Name", escapeString(a.Name))
	if a.GenericName != "" {
		line("GenericName", escapeString(a.GenericName))
	}
	if a.Summary != "" {
		line("Comment", escapeString(a.Summary))
	}
	line("Exec", execValue(a.Package, len(a.MIMETypes) > 0))
	if hasIcon {
		line("Icon", escapeString(a.ID))
	}
	line("Terminal", "false")
	cats := a.Categories
	if len(cats) == 0 {
		cats = []string{"Utility"}
	}
	line("Categories", escapeList(cats))
	if len(a.MIMETypes) > 0 {
		line("MimeType", escapeList(a.MIMETypes))
	}
	// The window class the program sets, which is its ID, so that a
	// launcher shows its windows under this entry and its icon.
	line("StartupWMClass", escapeString(a.ID))
	return []byte(b.String())
}
