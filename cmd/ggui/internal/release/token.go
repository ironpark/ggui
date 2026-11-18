package release

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
)

// Token is a credential, whose text is never printed: formatting one with
// %v or %s shows where it came from.
type Token struct {
	value  string
	Source string // $GITHUB_TOKEN, gh auth token, ...
}

// Value returns the secret, for an Authorization header.
func (t Token) Value() string { return t.value }

// String says where the token came from, never what it is.
func (t Token) String() string { return "token from " + t.Source }

// GoString is String, for %#v.
func (t Token) GoString() string { return t.String() }

// NewToken makes a token of value, from source.
func NewToken(value, source string) Token { return Token{value: value, Source: source} }

// ErrNoToken says how to give a GitHub token.
var ErrNoToken = errors.New("no GitHub token: set $GITHUB_TOKEN or $GH_TOKEN, or sign in with gh auth login")

// GitHubToken finds a token for host, github.com or a GitHub Enterprise
// host: $GITHUB_TOKEN, then $GH_TOKEN, then what gh auth token prints
// when gh is installed. getenv is os.Getenv outside tests.
func GitHubToken(ctx context.Context, getenv func(string) string, host string) (Token, error) {
	for _, v := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if s := strings.TrimSpace(getenv(v)); s != "" {
			return NewToken(s, "$"+v), nil
		}
	}
	gh, err := exec.LookPath("gh")
	if err != nil {
		return Token{}, ErrNoToken
	}
	args := []string{"auth", "token"}
	if host != "" && host != "github.com" {
		args = append(args, "--hostname", host)
	}
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, gh, args...)
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return Token{}, ErrNoToken
	}
	if s := strings.TrimSpace(out.String()); s != "" {
		return NewToken(s, "gh auth token"), nil
	}
	return Token{}, ErrNoToken
}
