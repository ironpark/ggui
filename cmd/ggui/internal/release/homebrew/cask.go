// Package homebrew writes the Homebrew cask of a macOS release, and
// commits it to a tap through GitHub's contents API.
package homebrew

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ironpark/ggui/cmd/ggui/internal/release/github"
)

// Cask is what the cask says.
type Cask struct {
	Token    string // the cask's token, such as hello-world
	Name     string // the app's name, as people see it
	Version  string
	Desc     string // one line; empty leaves desc out
	Homepage string
	ID       string // the bundle identifier, for zap
	// MinMacOS is the oldest macOS the app runs on, such as 13.0; a
	// version Homebrew has no name for leaves depends_on out.
	MinMacOS string
	// Universal is the one download for both architectures; otherwise
	// Arm and Intel are each architecture's, either of them nil for a
	// release without it.
	Universal  *File
	Arm, Intel *File
}

// File is a download: its URL, with the version written as it is, and
// its SHA-256.
type File struct {
	URL    string
	SHA256 string
}

// Token makes a cask token from an app's name by Homebrew's rules:
// lowercase, "+" as "-plus-", "@" as "-at-", "&" as "-and-", and every
// other run of what is not a letter or digit as one hyphen.
func Token(name string) string {
	name = strings.TrimSuffix(name, ".app")
	r := strings.NewReplacer("+", "-plus-", "@", "-at-", "&", "-and-")
	name = r.Replace(strings.ToLower(name))
	var b strings.Builder
	dash := false
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z' || c >= '0' && c <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(c)
			dash = false
		default:
			dash = true
		}
	}
	return b.String()
}

// macOSNames are the symbols Homebrew names macOS versions by.
var macOSNames = map[string]string{
	"10.13": "high_sierra", "10.14": "mojave", "10.15": "catalina",
	"11": "big_sur", "12": "monterey", "13": "ventura", "14": "sonoma",
	"15": "sequoia", "26": "tahoe",
}

// MacOSName returns Homebrew's symbol for a minimum macOS version, such as
// ventura for 13.0, or "" for one it has none for.
func MacOSName(v string) string {
	parts := strings.Split(v, ".")
	if parts[0] == "10" && len(parts) > 1 {
		return macOSNames["10."+parts[1]]
	}
	return macOSNames[parts[0]]
}

var rubyEscaper = strings.NewReplacer(
	`\`, `\\`,
	`"`, `\"`,
	"\r", `\r`,
	"\n", `\n`,
	"#{", `\#{`,
	"#@", `\#@`,
	"#$", `\#$`,
)

// rubyString quotes s as a Ruby double-quoted string with interpolation
// sequences escaped.
func rubyString(s string) string {
	return `"` + rubyEscaper.Replace(s) + `"`
}

// versioned writes u as a Ruby double-quoted string with safe occurrences of
// version in the download path substituted by #{version}. Pre-existing Ruby
// interpolation tokens in u are always escaped, and scheme/host/owner/repo
// prefixes are never substituted.
func versioned(u, version string) string {
	return `"` + versionedBody(u, version, -1, 0) + `"`
}

// versionedArch is versioned, also substituting the single slice u[archAt:archAt+archLen]
// with #{arch}.
func versionedArch(u, version string, archAt, archLen int) string {
	return `"` + versionedBody(u, version, archAt, archLen) + `"`
}

func versionedBody(u, version string, archAt, archLen int) string {
	prefixEnd := versionPrefixEnd(u)
	var b strings.Builder
	emitLiteral := func(seg string, allowVersion bool) {
		if !allowVersion || version == "" {
			b.WriteString(rubyEscaper.Replace(seg))
			return
		}
		b.WriteString(escapeAndSubVersion(seg, version))
	}
	if archAt < 0 {
		emitLiteral(u[:prefixEnd], false)
		emitLiteral(u[prefixEnd:], true)
		return b.String()
	}
	archEnd := archAt + archLen
	if archEnd <= prefixEnd {
		emitLiteral(u[:archAt], false)
		b.WriteString("#{arch}")
		emitLiteral(u[archEnd:prefixEnd], false)
		emitLiteral(u[prefixEnd:], true)
	} else if archAt >= prefixEnd {
		emitLiteral(u[:prefixEnd], false)
		emitLiteral(u[prefixEnd:archAt], true)
		b.WriteString("#{arch}")
		emitLiteral(u[archEnd:], true)
	} else {
		emitLiteral(u[:archAt], false)
		b.WriteString("#{arch}")
		emitLiteral(u[archEnd:], true)
	}
	return b.String()
}

// versionPrefixEnd returns the byte offset in u after which version
// substitution is allowed (after /releases/download/, or after scheme://host/).
func versionPrefixEnd(u string) int {
	const ghMarker = "/releases/download/"
	if i := strings.Index(u, ghMarker); i >= 0 {
		return i + len(ghMarker)
	}
	if i := strings.Index(u, "://"); i >= 0 {
		if j := strings.IndexByte(u[i+3:], '/'); j >= 0 {
			return i + 3 + j + 1
		}
		return len(u)
	}
	return 0
}

func isASCIIAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// escapeAndSubVersion escapes s for a Ruby double-quoted string while replacing
// token-bounded occurrences of version with #{version}.
func escapeAndSubVersion(s, version string) string {
	var b strings.Builder
	pos := 0
	for pos < len(s) {
		i := strings.Index(s[pos:], version)
		if i < 0 {
			b.WriteString(rubyEscaper.Replace(s[pos:]))
			break
		}
		start := pos + i
		end := start + len(version)
		leftOK := start == 0 || !isASCIIAlnum(s[start-1]) ||
			((s[start-1] == 'v' || s[start-1] == 'V') && (start-1 == 0 || !isASCIIAlnum(s[start-2])))
		rightOK := end == len(s) || !isASCIIAlnum(s[end])
		if leftOK && rightOK {
			b.WriteString(rubyEscaper.Replace(s[pos:start]))
			b.WriteString("#{version}")
			pos = end
		} else {
			b.WriteString(rubyEscaper.Replace(s[pos : start+1]))
			pos = start + 1
		}
	}
	return b.String()
}

// Desc tidies a description as Homebrew's style wants one: no article to
// begin it, no full stop to end it, the first letter a capital.
func Desc(s string) string {
	s = strings.TrimSpace(s)
	for _, a := range []string{"A ", "An ", "The "} {
		if len(s) > len(a) && strings.EqualFold(s[:len(a)], a) {
			s = s[len(a):]
			break
		}
	}
	s = strings.TrimRight(s, ". ")
	if s != "" {
		r, size := utf8.DecodeRuneInString(s)
		s = string(unicode.ToUpper(r)) + s[size:]
	}
	return s
}

// Ruby writes the cask.
func (c Cask) Ruby() (string, error) {
	if c.Universal == nil && c.Arm == nil && c.Intel == nil {
		return "", fmt.Errorf("cask %s: no macOS download", c.Token)
	}
	var b strings.Builder
	p := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	p("cask %s do\n", rubyString(c.Token))

	var dl *File
	switch {
	case c.Universal != nil:
		dl = c.Universal
		p("  version %s\n", rubyString(c.Version))
		p("  sha256 %s\n\n", rubyString(dl.SHA256))
		p("  url %s%s\n", versioned(dl.URL, c.Version), verified(dl.URL, c.Homepage, "      "))
	case c.Arm != nil && c.Intel != nil:
		ai := strings.LastIndex(c.Arm.URL, "arm64")
		ii := strings.LastIndex(c.Intel.URL, "amd64")
		if ai >= 0 && ii >= 0 && c.Arm.URL[:ai] == c.Intel.URL[:ii] && c.Arm.URL[ai+len("arm64"):] == c.Intel.URL[ii+len("amd64"):] {
			p("  arch arm: \"arm64\", intel: \"amd64\"\n\n")
			p("  version %s\n", rubyString(c.Version))
			p("  sha256 arm:   %s,\n", rubyString(c.Arm.SHA256))
			p("         intel: %s\n\n", rubyString(c.Intel.SHA256))
			p("  url %s%s\n", versionedArch(c.Arm.URL, c.Version, ai, len("arm64")), verified(c.Arm.URL, c.Homepage, "      "))
		} else {
			p("  version %s\n\n", rubyString(c.Version))
			for _, a := range []struct {
				block string
				f     *File
			}{{"on_arm", c.Arm}, {"on_intel", c.Intel}} {
				p("  %s do\n", a.block)
				p("    sha256 %s\n\n", rubyString(a.f.SHA256))
				p("    url %s%s\n", versioned(a.f.URL, c.Version), verified(a.f.URL, c.Homepage, "        "))
				p("  end\n")
			}
			b.WriteString("\n")
		}
		dl = c.Arm
	default:
		dl = c.Arm
		if dl == nil {
			dl = c.Intel
		}
		p("  version %s\n", rubyString(c.Version))
		p("  sha256 %s\n\n", rubyString(dl.SHA256))
		p("  url %s%s\n", versioned(dl.URL, c.Version), verified(dl.URL, c.Homepage, "      "))
	}
	p("  name %s\n", rubyString(c.Name))
	if d := Desc(c.Desc); d != "" {
		p("  desc %s\n", rubyString(d))
	}
	p("  homepage %s\n\n", rubyString(c.Homepage))

	if strings.Contains(dl.URL, "/releases/download/") {
		p("  livecheck do\n    url :url\n    strategy :github_latest\n  end\n\n")
	}

	var deps []string
	if c.Universal == nil && (c.Arm == nil) != (c.Intel == nil) {
		if c.Arm != nil {
			deps = append(deps, "  depends_on arch: :arm64\n")
		} else {
			deps = append(deps, "  depends_on arch: :x86_64\n")
		}
	}
	if name := MacOSName(c.MinMacOS); name != "" {
		deps = append(deps, fmt.Sprintf("  depends_on macos: \">= :%s\"\n", name))
	}
	if len(deps) > 0 {
		b.WriteString(strings.Join(deps, ""))
		b.WriteString("\n")
	}

	p("  app %s\n", rubyString(c.Name+".app"))
	if c.ID != "" {
		p("\n  zap trash: [\n")
		for _, path := range []string{
			"~/Library/Application Support/" + c.ID,
			"~/Library/Preferences/" + c.ID + ".plist",
			"~/Library/Saved Application State/" + c.ID + ".savedState",
		} {
			p("    %s,\n", rubyString(path))
		}
		p("  ]\n")
	}
	p("end\n")
	return b.String(), nil
}

// verified returns the url stanza's verified parameter when the download
// is on another host than the homepage, as Homebrew's audit asks: the
// download's host and its owner's path.
func verified(download, homepage, indent string) string {
	d, err := url.Parse(download)
	if err != nil {
		return ""
	}
	h, err := url.Parse(homepage)
	if err == nil && strings.TrimPrefix(h.Host, "www.") == strings.TrimPrefix(d.Host, "www.") {
		return ""
	}
	parts := strings.Split(strings.Trim(d.Path, "/"), "/")
	v := d.Host + "/"
	if len(parts) >= 2 {
		v += parts[0] + "/" + parts[1] + "/"
	}
	return ",\n" + indent + "verified: " + rubyString(v)
}

// Path is where the cask goes in a tap.
func Path(token string) string { return "Casks/" + token + ".rb" }

// Publish commits the cask to the tap, replacing the file there.
func Publish(ctx context.Context, c *github.Client, tap, token, version string, ruby []byte) error {
	path := Path(token)
	sha, err := c.FileSHA(ctx, tap, path)
	if err != nil {
		return err
	}
	return c.PutFile(ctx, tap, path, CommitMessage(token, version), ruby, sha)
}

// CommitMessage is the tap commit's message.
func CommitMessage(token, version string) string {
	return "Update " + token + " to " + version
}

// Plan lists the requests Publish would make, for a dry run.
func Plan(api, tap, token, version string) []string {
	owner, name, _ := strings.Cut(tap, "/")
	u := strings.TrimRight(api, "/") + "/repos/" + owner + "/" + name + "/contents/" + Path(token)
	return []string{
		"GET    " + u + "  (the current file's sha, if any)",
		"PUT    " + u + "  (message " + fmt.Sprintf("%q", CommitMessage(token, version)) + ")",
	}
}
