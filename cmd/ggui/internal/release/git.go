package release

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// ParseRepo reads owner/repo from a git remote URL in any of the forms
// git takes: https://github.com/owner/repo.git, git@github.com:owner/repo,
// ssh://git@github.com/owner/repo.git. It also returns the host, so that
// a remote on GitHub Enterprise can be told from one on github.com.
func ParseRepo(remote string) (host, repo string, err error) {
	s := strings.TrimSpace(remote)
	var path string
	switch {
	case strings.Contains(s, "://"):
		u, err := url.Parse(s)
		if err != nil {
			return "", "", fmt.Errorf("remote %q: %v", remote, err)
		}
		host, path = u.Hostname(), u.Path
	case strings.Contains(s, ":"):
		// scp-like: [user@]host:owner/repo
		h, p, _ := strings.Cut(s, ":")
		if i := strings.LastIndexByte(h, '@'); i >= 0 {
			h = h[i+1:]
		}
		host, path = h, p
	default:
		return "", "", fmt.Errorf("remote %q is not a URL of a GitHub repository", remote)
	}
	path = strings.TrimSuffix(strings.Trim(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if host == "" || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("remote %q is not a URL of a GitHub repository, owner/repo", remote)
	}
	return host, parts[0] + "/" + parts[1], nil
}

// Git runs git in a directory.
type Git struct {
	Dir string
}

// ErrNotRepo is returned when the directory is not in a git work tree, or
// git is not installed.
var ErrNotRepo = errors.New("not in a git work tree")

func (g Git) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.Dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrNotRepo
		}
		msg := strings.TrimSpace(errb.String())
		if strings.Contains(msg, "not a git repository") {
			return "", ErrNotRepo
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", args[0], msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// Head returns the commit checked out.
func (g Git) Head(ctx context.Context) (string, error) {
	return g.run(ctx, "rev-parse", "HEAD")
}

// Dirty returns the paths with changes not committed, untracked files
// among them, or none for a clean tree.
func (g Git) Dirty(ctx context.Context) ([]string, error) {
	out, err := g.run(ctx, "status", "--porcelain")
	if err != nil || out == "" {
		return nil, err
	}
	return strings.Split(out, "\n"), nil
}

// Origin returns the URL of the remote called origin.
func (g Git) Origin(ctx context.Context) (string, error) {
	return g.run(ctx, "remote", "get-url", "origin")
}

// LocalTag returns the commit the tag points to here, or "" when there
// is no such tag.
func (g Git) LocalTag(ctx context.Context, tag string) (string, error) {
	out, err := g.run(ctx, "tag", "--list", tag)
	if err != nil || out == "" {
		return "", err
	}
	return g.run(ctx, "rev-parse", "refs/tags/"+tag+"^{commit}")
}

// RemoteTag returns the commit the tag points to on origin, or "" when
// origin has no such tag.
func (g Git) RemoteTag(ctx context.Context, tag string) (string, error) {
	out, err := g.run(ctx, "ls-remote", "--tags", "origin", "refs/tags/"+tag, "refs/tags/"+tag+"^{}")
	if err != nil {
		return "", err
	}
	return PeeledTag(out, tag), nil
}

// PeeledTag reads the commit tag points to from git ls-remote's output:
// an annotated tag's peeled line, ending ^{}, names the commit, a
// lightweight tag's own line does.
func PeeledTag(lsRemote, tag string) string {
	var direct string
	for line := range strings.SplitSeq(lsRemote, "\n") {
		sha, ref, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok {
			continue
		}
		switch ref {
		case "refs/tags/" + tag + "^{}":
			return sha
		case "refs/tags/" + tag:
			direct = sha
		}
	}
	return direct
}
