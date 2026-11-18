// Package github is the little of GitHub's REST API that ggui release
// uses: releases, their assets, a commit's existence and a file's
// contents, with net/http and encoding/json.
//
// No error it returns holds the token: the token goes in a header only,
// and an error names the method, the URL and what GitHub said.
package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/release"
)

// DefaultAPI is github.com's API.
const DefaultAPI = "https://api.github.com"

// Client calls the API as the holder of Token.
type Client struct {
	API   string // the API root, DefaultAPI or a GitHub Enterprise one
	Token release.Token
	HTTP  *http.Client // nil is a client with a long timeout
	// Retries is how many times a request that failed for a reason that
	// may pass, a network error or a 5xx status, is tried again.
	Retries int
	// Backoff is the wait before the first retry, doubled for each one
	// after; zero is a second.
	Backoff time.Duration
	// Logf, when set, is told of each retry.
	Logf func(format string, args ...any)
}

// New returns a client of api with token, retrying three times.
func New(api string, token release.Token) *Client {
	return &Client{API: strings.TrimRight(api, "/"), Token: token, Retries: 3}
}

// Error is a response GitHub refused a request with.
type Error struct {
	Method, URL string
	Status      int
	Message     string // GitHub's message and the errors it listed
}

func (e *Error) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	return fmt.Sprintf("%s %s: %d %s", e.Method, e.URL, e.Status, msg)
}

// IsNotFound reports whether err is GitHub's 404.
func IsNotFound(err error) bool {
	e, ok := errors.AsType[*Error](err)
	return ok && e.Status == http.StatusNotFound
}

// body makes a request body anew for each attempt.
type body func() (io.ReadCloser, int64, error)

func jsonBody(v any) (body, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return func() (io.ReadCloser, int64, error) {
		return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
	}, nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Minute}
}

// do sends a request to u, an absolute URL or a path below the API,
// retrying what may pass, and decodes a JSON reply into out when given.
func (c *Client) do(ctx context.Context, method, u string, b body, contentType string, out any) error {
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = c.API + u
	}
	wait := c.Backoff
	if wait <= 0 {
		wait = time.Second
	}
	for attempt := 0; ; attempt++ {
		err := c.once(ctx, method, u, b, contentType, out)
		if err == nil || attempt >= c.Retries || !transient(err) || ctx.Err() != nil {
			return err
		}
		if c.Logf != nil {
			c.Logf("%v; trying again in %v", err, wait)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		wait *= 2
	}
}

// transient reports whether a request that failed with err may succeed
// when sent again.
func transient(err error) bool {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Status >= 500
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	_, isNet := errors.AsType[net.Error](err)
	_, isURL := errors.AsType[*url.Error](err)
	return isNet || isURL || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
}

func (c *Client) once(ctx context.Context, method, u string, b body, contentType string, out any) error {
	var (
		rd   io.ReadCloser
		size int64
	)
	if b != nil {
		var err error
		if rd, size, err = b(); err != nil {
			return err
		}
		defer rd.Close()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	if rd != nil {
		req.ContentLength = size
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "ggui-release")
	if t := c.Token.Value(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		// The URL holds no secret; the error is the transport's.
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg := message(data)
		if t := c.Token.Value(); t != "" {
			// A proxy may echo the request back.
			msg = strings.ReplaceAll(msg, t, "(the token)")
		}
		return &Error{Method: method, URL: redact(u), Status: resp.StatusCode, Message: msg}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("%s %s: reading the reply: %v", method, redact(u), err)
		}
	}
	return nil
}

// redact drops a URL's query and user information, which ggui never puts
// a secret in but a configured URL might.
func redact(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return "(a URL)"
	}
	p.User = nil
	return p.String()
}

// message reads GitHub's error reply: its message and the errors it
// lists.
func message(data []byte) string {
	var r struct {
		Message string `json:"message"`
		Errors  []struct {
			Code     string `json:"code"`
			Field    string `json:"field"`
			Message  string `json:"message"`
			Resource string `json:"resource"`
		} `json:"errors"`
	}
	if json.Unmarshal(data, &r) != nil {
		s := strings.TrimSpace(string(data))
		if len(s) > 200 {
			s = s[:200] + "..."
		}
		return s
	}
	var msg strings.Builder
	msg.WriteString(r.Message)
	for _, e := range r.Errors {
		switch {
		case e.Message != "":
			msg.WriteString("; " + e.Message)
		case e.Code != "":
			msg.WriteString("; " + strings.TrimSpace(e.Resource+" "+e.Field+" "+e.Code))
		}
	}
	return msg.String()
}

// Release is a GitHub release.
type Release struct {
	ID         int64   `json:"id"`
	TagName    string  `json:"tag_name"`
	Name       string  `json:"name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	HTMLURL    string  `json:"html_url"`
	UploadURL  string  `json:"upload_url"`
	Assets     []Asset `json:"assets"`
}

// Asset is a file of a release.
type Asset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	State              string `json:"state"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func repoPath(repo string) string {
	owner, name, _ := strings.Cut(repo, "/")
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(name)
}

// FindRelease returns the release of repo made at tag, draft or not, or
// nil when there is none. A draft has no tag yet, so the releases are
// listed rather than looked up by tag.
func (c *Client) FindRelease(ctx context.Context, repo, tag string) (*Release, error) {
	for page := 1; page <= 10; page++ {
		var rs []Release
		u := repoPath(repo) + "/releases?per_page=100&page=" + strconv.Itoa(page)
		if err := c.do(ctx, http.MethodGet, u, nil, "", &rs); err != nil {
			return nil, err
		}
		for i := range rs {
			if rs[i].TagName == tag {
				return &rs[i], nil
			}
		}
		if len(rs) < 100 {
			break
		}
	}
	return nil, nil
}

// NewRelease is what a release is created with.
type NewRelease struct {
	TagName         string `json:"tag_name"`
	TargetCommitish string `json:"target_commitish,omitempty"`
	Name            string `json:"name"`
	Body            string `json:"body"`
	Draft           bool   `json:"draft"`
	Prerelease      bool   `json:"prerelease"`
}

// CreateRelease creates a release in repo.
func (c *Client) CreateRelease(ctx context.Context, repo string, r NewRelease) (*Release, error) {
	b, err := jsonBody(r)
	if err != nil {
		return nil, err
	}
	var out Release
	return &out, c.do(ctx, http.MethodPost, repoPath(repo)+"/releases", b, "application/json", &out)
}

// UpdateRelease changes the fields of the release id that patch sets.
func (c *Client) UpdateRelease(ctx context.Context, repo string, id int64, patch map[string]any) (*Release, error) {
	b, err := jsonBody(patch)
	if err != nil {
		return nil, err
	}
	var out Release
	return &out, c.do(ctx, http.MethodPatch, repoPath(repo)+"/releases/"+strconv.FormatInt(id, 10), b, "application/json", &out)
}

// ListAssets lists the assets of the release id.
func (c *Client) ListAssets(ctx context.Context, repo string, id int64) ([]Asset, error) {
	var all []Asset
	for page := 1; page <= 10; page++ {
		var as []Asset
		u := repoPath(repo) + "/releases/" + strconv.FormatInt(id, 10) + "/assets?per_page=100&page=" + strconv.Itoa(page)
		if err := c.do(ctx, http.MethodGet, u, nil, "", &as); err != nil {
			return nil, err
		}
		all = append(all, as...)
		if len(as) < 100 {
			break
		}
	}
	return all, nil
}

// DeleteAsset deletes the asset id.
func (c *Client) DeleteAsset(ctx context.Context, repo string, id int64) error {
	return c.do(ctx, http.MethodDelete, repoPath(repo)+"/releases/assets/"+strconv.FormatInt(id, 10), nil, "", nil)
}

// UploadAsset uploads the file at path to the release as name.
func (c *Client) UploadAsset(ctx context.Context, r *Release, path, name string) (*Asset, error) {
	base, _, _ := strings.Cut(r.UploadURL, "{")
	if base == "" {
		return nil, fmt.Errorf("release %d has no upload URL", r.ID)
	}
	u := base + "?name=" + url.QueryEscape(name)
	b := func() (io.ReadCloser, int64, error) {
		f, err := os.Open(path)
		if err != nil {
			return nil, 0, err
		}
		fi, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, 0, err
		}
		return f, fi.Size(), nil
	}
	var out Asset
	return &out, c.do(ctx, http.MethodPost, u, b, ContentType(name), &out)
}

// ContentType is the media type an asset is uploaded as, by its name.
func ContentType(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.HasSuffix(n, ".tar.gz"), strings.HasSuffix(n, ".tgz"), strings.HasSuffix(n, ".gz"):
		return "application/gzip"
	case strings.HasSuffix(n, ".zip"):
		return "application/zip"
	case strings.HasSuffix(n, ".dmg"):
		return "application/x-apple-diskimage"
	case strings.HasSuffix(n, ".pkg"):
		return "application/vnd.apple.installer+xml"
	case strings.HasSuffix(n, ".deb"):
		return "application/vnd.debian.binary-package"
	case strings.HasSuffix(n, ".rpm"):
		return "application/x-rpm"
	case filepath.Base(n) == strings.ToLower(release.SumsName), strings.HasSuffix(n, ".txt"):
		return "text/plain; charset=utf-8"
	}
	return "application/octet-stream"
}

// CommitExists reports whether repo has the commit sha, as a release
// made at it needs.
func (c *Client) CommitExists(ctx context.Context, repo, sha string) (bool, error) {
	err := c.do(ctx, http.MethodGet, repoPath(repo)+"/commits/"+url.PathEscape(sha), nil, "", nil)
	if e, ok := errors.AsType[*Error](err); ok && (e.Status == http.StatusNotFound || e.Status == http.StatusUnprocessableEntity) {
		return false, nil
	}
	return err == nil, err
}

func contentsPath(repo, path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return repoPath(repo) + "/contents/" + strings.Join(parts, "/")
}

// FileSHA returns the blob SHA of path in repo's default branch, which
// replacing it needs, or "" when there is no such file.
func (c *Client) FileSHA(ctx context.Context, repo, path string) (string, error) {
	var out struct {
		SHA string `json:"sha"`
	}
	err := c.do(ctx, http.MethodGet, contentsPath(repo, path), nil, "", &out)
	if IsNotFound(err) {
		return "", nil
	}
	return out.SHA, err
}

// PutFile commits content to path in repo's default branch with message,
// replacing the blob sha, or creating the file when sha is "".
func (c *Client) PutFile(ctx context.Context, repo, path, message string, content []byte, sha string) error {
	req := map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString(content),
	}
	if sha != "" {
		req["sha"] = sha
	}
	b, err := jsonBody(req)
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPut, contentsPath(repo, path), b, "application/json", nil)
}

// UploadsURL is where the API's uploads go: uploads.github.com for
// github.com, /api/uploads on a GitHub Enterprise host.
func UploadsURL(api string) string {
	api = strings.TrimRight(api, "/")
	if api == DefaultAPI {
		return "https://uploads.github.com"
	}
	if base, ok := strings.CutSuffix(api, "/api/v3"); ok {
		return base + "/api/uploads"
	}
	return api
}

// WebURL is the site the API belongs to, where release downloads are:
// https://github.com for github.com.
func WebURL(api string) string {
	api = strings.TrimRight(api, "/")
	if api == DefaultAPI {
		return "https://github.com"
	}
	if base, ok := strings.CutSuffix(api, "/api/v3"); ok {
		return base
	}
	return api
}

// Host is the host of the site the API belongs to, as gh auth token
// --hostname takes it.
func Host(api string) string {
	u, err := url.Parse(WebURL(api))
	if err != nil {
		return ""
	}
	return u.Host
}

// DownloadURL is where an asset of the release at tag is downloaded from.
func DownloadURL(web, repo, tag, name string) string {
	return strings.TrimRight(web, "/") + "/" + repo + "/releases/download/" + url.PathEscape(tag) + "/" + url.PathEscape(name)
}
