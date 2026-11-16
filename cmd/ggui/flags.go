package main

import (
	"flag"
	"slices"
	"strings"
)

// parseInterspersed parses args into fs, taking flags after positional
// arguments too, since "ggui dev ./app --web" reads as naturally as the
// other order. It returns the positional arguments and, separately,
// everything after a "--", which belongs to the app rather than to ggui.
func parseInterspersed(fs *flag.FlagSet, args []string) (pos, rest []string, err error) {
	if i := slices.Index(args, "--"); i >= 0 {
		args, rest = args[:i], args[i+1:]
	}
	for {
		if err := fs.Parse(args); err != nil {
			return nil, nil, err
		}
		left := fs.Args()
		if len(left) == 0 {
			return pos, rest, nil
		}
		pos = append(pos, left[0])
		args = left[1:]
	}
}

// isSet reports whether the flag called name was given on the command
// line, which is when it overrides ggui.toml.
func isSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// splitTags splits a -tags value as the go command does: by commas, or by
// spaces in the older form.
func splitTags(s string) []string {
	f := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	if f == nil {
		return []string{}
	}
	return f
}
