package installer

import (
	"path/filepath"
	"strings"
)

// modifyPathList adds or removes dir in a semicolon-delimited Windows PATH
// string, preserving existing entries and avoiding duplicates.
func modifyPathList(current, dir string, add bool) (string, bool) {
	target := normalizePathEntry(dir)
	if target == "" {
		return current, false
	}
	var kept []string
	found := false
	for raw := range strings.SplitSeq(current, ";") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if strings.EqualFold(normalizePathEntry(trimmed), target) {
			found = true
			if !add {
				continue
			}
		}
		kept = append(kept, trimmed)
	}
	if add {
		if found {
			return current, false
		}
		kept = append(kept, strings.TrimSpace(dir))
		return strings.Join(kept, ";"), true
	}
	if !found {
		return current, false
	}
	return strings.Join(kept, ";"), true
}

func normalizePathEntry(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"`)
	if p == "" {
		return ""
	}
	p = filepath.Clean(strings.ReplaceAll(p, "/", `\`))
	// Keep drive roots like C:\ intact while stripping trailing slashes on subdirs.
	if len(p) > 3 {
		p = strings.TrimRight(p, `\/`)
	}
	return p
}
