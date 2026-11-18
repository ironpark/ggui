// Package homebrew writes the Homebrew cask of a macOS release, and
// commits it to a tap through GitHub's contents API.
package homebrew

import (
	"context"
	"fmt"
	"net/url"
	"strings"

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

// rubyString quotes s as a Ruby double-quoted string, with #{ escaped
// unless interpolate.
func rubyString(s string, interpolate bool) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	s = r.Replace(s)
	if !interpolate {
		s = strings.ReplaceAll(s, "#{", `\#{`)
	}
	return `"` + s + `"`
}

// versioned writes u with the version as #{version}, so that the cask's
// url need not change but for the version stanza.
func versioned(u, version string) string {
	return rubyString(strings.ReplaceAll(u, version, "#{version}"), true)
}

// replaceLast replaces the last old in s with new.
func replaceLast(s, old, new string) string {
	i := strings.LastIndex(s, old)
	if i < 0 {
		return s
	}
	return s[:i] + new + s[i+len(old):]
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
		s = strings.ToUpper(s[:1]) + s[1:]
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
	p("cask %s do\n", rubyString(c.Token, false))

	var dl *File
	switch {
	case c.Universal != nil:
		dl = c.Universal
		p("  version %s\n", rubyString(c.Version, false))
		p("  sha256 %s\n\n", rubyString(dl.SHA256, false))
		p("  url %s%s\n", versioned(dl.URL, c.Version), verified(dl.URL, c.Homepage, "      "))
	case c.Arm != nil && c.Intel != nil:
		arm := replaceLast(c.Arm.URL, "arm64", "#{arch}")
		intel := replaceLast(c.Intel.URL, "amd64", "#{arch}")
		if arm == intel {
			p("  arch arm: \"arm64\", intel: \"amd64\"\n\n")
			p("  version %s\n", rubyString(c.Version, false))
			p("  sha256 arm:   %s,\n", rubyString(c.Arm.SHA256, false))
			p("         intel: %s\n\n", rubyString(c.Intel.SHA256, false))
			p("  url %s%s\n", versioned(arm, c.Version), verified(c.Arm.URL, c.Homepage, "      "))
		} else {
			p("  version %s\n\n", rubyString(c.Version, false))
			for _, a := range []struct {
				block string
				f     *File
			}{{"on_arm", c.Arm}, {"on_intel", c.Intel}} {
				p("  %s do\n", a.block)
				p("    sha256 %s\n\n", rubyString(a.f.SHA256, false))
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
		p("  version %s\n", rubyString(c.Version, false))
		p("  sha256 %s\n\n", rubyString(dl.SHA256, false))
		p("  url %s%s\n", versioned(dl.URL, c.Version), verified(dl.URL, c.Homepage, "      "))
	}
	p("  name %s\n", rubyString(c.Name, false))
	if d := Desc(c.Desc); d != "" {
		p("  desc %s\n", rubyString(d, false))
	}
	p("  homepage %s\n\n", rubyString(c.Homepage, false))

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

	p("  app %s\n", rubyString(c.Name+".app", false))
	if c.ID != "" {
		p("\n  zap trash: [\n")
		for _, path := range []string{
			"~/Library/Application Support/" + c.ID,
			"~/Library/Preferences/" + c.ID + ".plist",
			"~/Library/Saved Application State/" + c.ID + ".savedState",
		} {
			p("    %s,\n", rubyString(path, false))
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
	return ",\n" + indent + "verified: " + rubyString(v, false)
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
