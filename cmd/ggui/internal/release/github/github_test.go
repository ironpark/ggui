package github

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/release"
)

const secret = "ghp_s3cret_value"

// fake is a GitHub of one repository, alice/hello, with its releases.
type fake struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	releases []*Release
	nextID   int64
	log      []string          // "METHOD path?query"
	uploads  map[string][]byte // by asset name
	headers  map[string]http.Header
	fail     map[string]int // by "METHOD path": statuses to answer with first
	files    map[string]string
	putBody  map[string]any
}

func newFake(t *testing.T) *fake {
	f := &fake{t: t, nextID: 100, uploads: map[string][]byte{}, headers: map[string]http.Header{}, fail: map[string]int{}, files: map[string]string{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) client() *Client {
	c := New(f.srv.URL, release.NewToken(secret, "$GITHUB_TOKEN"))
	c.Backoff = time.Millisecond
	return c
}

func (f *fake) reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			f.t.Error(err)
		}
	}
}

func (f *fake) find(id int64) *Release {
	for _, r := range f.releases {
		if r.ID == id {
			return r
		}
	}
	return nil
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := r.Method + " " + r.URL.Path
	f.log = append(f.log, key+queryOf(r))
	if r.Header.Get("Authorization") != "Bearer "+secret {
		f.reply(w, 401, map[string]string{"message": "Bad credentials: " + r.Header.Get("Authorization")})
		return
	}
	if r.Header.Get("X-GitHub-Api-Version") == "" || r.Header.Get("Accept") != "application/vnd.github+json" {
		f.t.Errorf("%s without the API headers", key)
	}
	if codes := f.fail[key]; codes > 0 {
		f.fail[key]--
		io.Copy(io.Discard, r.Body)
		f.reply(w, 502, map[string]string{"message": "Server Error"})
		return
	}
	var body map[string]any
	if r.Header.Get("Content-Type") == "application/json" {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			f.t.Errorf("%s: %v", key, err)
		}
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	switch {
	case key == "GET /repos/alice/hello/releases":
		f.reply(w, 200, f.releases)
	case key == "POST /repos/alice/hello/releases":
		f.nextID++
		rel := &Release{ID: f.nextID, TagName: body["tag_name"].(string), Name: body["name"].(string),
			Draft: body["draft"].(bool), Prerelease: body["prerelease"].(bool),
			HTMLURL:   "https://github.com/alice/hello/releases/tag/" + body["tag_name"].(string),
			UploadURL: f.srv.URL + "/uploads/repos/alice/hello/releases/" + strconv.FormatInt(f.nextID, 10) + "/assets{?name,label}"}
		if body["target_commitish"] != "abc123" {
			f.t.Errorf("target_commitish %v", body["target_commitish"])
		}
		f.releases = append(f.releases, rel)
		f.reply(w, 201, rel)
	case r.Method == "PATCH" && len(parts) == 5 && parts[3] == "releases":
		id, _ := strconv.ParseInt(parts[4], 10, 64)
		rel := f.find(id)
		if d, ok := body["draft"].(bool); ok {
			rel.Draft = d
		}
		if n, ok := body["name"].(string); ok {
			rel.Name = n
		}
		f.reply(w, 200, rel)
	case r.Method == "POST" && parts[0] == "uploads":
		id, _ := strconv.ParseInt(parts[5], 10, 64)
		rel := f.find(id)
		name := r.URL.Query().Get("name")
		for _, a := range rel.Assets {
			if a.Name == name {
				f.reply(w, 422, map[string]any{"message": "Validation Failed", "errors": []map[string]string{{"resource": "ReleaseAsset", "code": "already_exists", "field": "name"}}})
				return
			}
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			f.t.Error(err)
		}
		if r.ContentLength != int64(len(data)) {
			f.t.Errorf("upload %s: Content-Length %d, body %d", name, r.ContentLength, len(data))
		}
		f.uploads[name] = data
		f.headers[name] = r.Header.Clone()
		f.nextID++
		a := Asset{ID: f.nextID, Name: name, Size: int64(len(data)), State: "uploaded"}
		rel.Assets = append(rel.Assets, a)
		f.reply(w, 201, a)
	case r.Method == "GET" && len(parts) == 6 && parts[5] == "assets":
		id, _ := strconv.ParseInt(parts[4], 10, 64)
		f.reply(w, 200, f.find(id).Assets)
	case r.Method == "DELETE" && len(parts) == 6 && parts[4] == "assets":
		id, _ := strconv.ParseInt(parts[5], 10, 64)
		for _, rel := range f.releases {
			rel.Assets = slices.DeleteFunc(rel.Assets, func(a Asset) bool { return a.ID == id })
		}
		w.WriteHeader(204)
	case r.Method == "GET" && len(parts) == 5 && parts[3] == "commits":
		if parts[4] == "abc123" {
			f.reply(w, 200, map[string]string{"sha": "abc123"})
		} else {
			f.reply(w, 422, map[string]string{"message": "No commit found for SHA: " + parts[4]})
		}
	case len(parts) >= 4 && parts[3] == "contents":
		path := strings.Join(parts[4:], "/")
		switch r.Method {
		case "GET":
			if sha, ok := f.files[path]; ok {
				f.reply(w, 200, map[string]string{"sha": sha})
			} else {
				f.reply(w, 404, map[string]string{"message": "Not Found"})
			}
		case "PUT":
			f.putBody = body
			f.files[path] = "newsha"
			f.reply(w, 201, map[string]any{"content": map[string]string{"sha": "newsha"}})
		}
	default:
		f.t.Errorf("unexpected request %s", key)
		f.reply(w, 404, map[string]string{"message": "Not Found"})
	}
}

func queryOf(r *http.Request) string {
	if r.URL.RawQuery == "" {
		return ""
	}
	return "?" + r.URL.RawQuery
}

// assets writes files to upload.
func assets(t *testing.T, names ...string) []release.Asset {
	dir := t.TempDir()
	var out []release.Asset
	for _, n := range names {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("contents of "+n), 0o644); err != nil {
			t.Fatal(err)
		}
		a := release.Asset{Name: n, Path: p}
		if err := a.Hash(); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func publish(as []release.Asset) Publish {
	return Publish{Repo: "alice/hello", Tag: "v1.0.0", Commitish: "abc123", Name: "Hello 1.0.0", Body: "notes", Assets: as}
}

func TestCreateUploadPublish(t *testing.T) {
	f := newFake(t)
	as := assets(t, "hello-1.0.0-windows-amd64.zip", "hello-1.0.0-linux-amd64.tar.gz", "SHA256SUMS")
	rel, err := f.client().Run(t.Context(), publish(as))
	if err != nil {
		t.Fatal(err)
	}
	if rel.Draft {
		t.Error("the release was left a draft")
	}
	want := []string{
		"GET /repos/alice/hello/releases?per_page=100&page=1",
		"POST /repos/alice/hello/releases",
		"POST /uploads/repos/alice/hello/releases/101/assets?name=hello-1.0.0-windows-amd64.zip",
		"POST /uploads/repos/alice/hello/releases/101/assets?name=hello-1.0.0-linux-amd64.tar.gz",
		"POST /uploads/repos/alice/hello/releases/101/assets?name=SHA256SUMS",
		"PATCH /repos/alice/hello/releases/101",
	}
	if !slices.Equal(f.log, want) {
		t.Errorf("requests:\n%s\nwant:\n%s", strings.Join(f.log, "\n"), strings.Join(want, "\n"))
	}
	for _, a := range as {
		if got := string(f.uploads[a.Name]); got != "contents of "+a.Name {
			t.Errorf("%s uploaded as %q", a.Name, got)
		}
	}
	for name, ct := range map[string]string{
		"hello-1.0.0-windows-amd64.zip":  "application/zip",
		"hello-1.0.0-linux-amd64.tar.gz": "application/gzip",
		"SHA256SUMS":                     "text/plain; charset=utf-8",
	} {
		if got := f.headers[name].Get("Content-Type"); got != ct {
			t.Errorf("%s Content-Type %q, want %q", name, got, ct)
		}
	}
	// Created as a draft: published only by the PATCH at the end.
	if !strings.Contains(f.log[1], "POST") {
		t.Fatal(f.log)
	}
}

func TestDraftStaysDraft(t *testing.T) {
	f := newFake(t)
	p := publish(assets(t, "a.zip"))
	p.Draft = true
	rel, err := f.client().Run(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	if !rel.Draft || slices.ContainsFunc(f.log, func(s string) bool { return strings.HasPrefix(s, "PATCH") }) {
		t.Errorf("--draft published: %v", f.log)
	}
}

func TestReuseDraft(t *testing.T) {
	f := newFake(t)
	f.releases = []*Release{
		{ID: 7, TagName: "v0.9.0", HTMLURL: "x"},
		{ID: 8, TagName: "v1.0.0", Draft: true, UploadURL: f.srv.URL + "/uploads/repos/alice/hello/releases/8/assets{?name,label}",
			Assets: []Asset{{ID: 50, Name: "a.zip"}, {ID: 51, Name: "old.zip"}}},
	}
	if _, err := f.client().Run(t.Context(), publish(assets(t, "a.zip"))); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /repos/alice/hello/releases?per_page=100&page=1",
		"PATCH /repos/alice/hello/releases/8",
		"DELETE /repos/alice/hello/releases/assets/50",
		"POST /uploads/repos/alice/hello/releases/8/assets?name=a.zip",
		"PATCH /repos/alice/hello/releases/8",
	}
	if !slices.Equal(f.log, want) {
		t.Errorf("requests:\n%s", strings.Join(f.log, "\n"))
	}
	if len(f.releases) != 2 || f.releases[1].Draft {
		t.Errorf("releases %+v", f.releases)
	}
}

func TestRefusePublished(t *testing.T) {
	f := newFake(t)
	f.releases = []*Release{{ID: 9, TagName: "v1.0.0", HTMLURL: "https://github.com/alice/hello/releases/tag/v1.0.0",
		UploadURL: f.srv.URL + "/uploads/repos/alice/hello/releases/9/assets{?name,label}",
		Assets:    []Asset{{ID: 60, Name: "a.zip"}}}}
	_, err := f.client().Run(t.Context(), publish(assets(t, "a.zip")))
	if err == nil || !strings.Contains(err.Error(), "published already") || !strings.Contains(err.Error(), "--replace-assets") {
		t.Fatalf("err = %v", err)
	}
	if len(f.log) != 1 {
		t.Errorf("refusing still sent %v", f.log)
	}

	// With ReplaceAssets the files are replaced, and the release stays
	// as it is.
	f.log = nil
	p := publish(assets(t, "a.zip"))
	p.ReplaceAssets = true
	if _, err := f.client().Run(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"GET /repos/alice/hello/releases?per_page=100&page=1",
		"DELETE /repos/alice/hello/releases/assets/60",
		"POST /uploads/repos/alice/hello/releases/9/assets?name=a.zip",
	}
	if !slices.Equal(f.log, want) {
		t.Errorf("requests:\n%s", strings.Join(f.log, "\n"))
	}
}

func TestRetryOn502(t *testing.T) {
	f := newFake(t)
	f.fail["POST /uploads/repos/alice/hello/releases/101/assets"] = 2
	f.fail["GET /repos/alice/hello/releases"] = 1
	var retries []string
	c := f.client()
	c.Logf = func(format string, args ...any) { retries = append(retries, fmt.Sprintf(format, args...)) }
	if _, err := c.Run(t.Context(), publish(assets(t, "a.zip"))); err != nil {
		t.Fatal(err)
	}
	if len(retries) != 3 || !strings.Contains(retries[0], "502") {
		t.Errorf("retries: %v", retries)
	}
	if string(f.uploads["a.zip"]) != "contents of a.zip" {
		t.Errorf("the retried upload sent %q", f.uploads["a.zip"])
	}

	// One more failure than retries is an error.
	f.fail["POST /repos/alice/hello/releases"] = 4
	f.releases = nil
	if _, err := c.Run(t.Context(), publish(assets(t, "a.zip"))); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("err = %v", err)
	}
}

func TestFailedUploadLeavesDraft(t *testing.T) {
	f := newFake(t)
	f.fail["POST /uploads/repos/alice/hello/releases/101/assets"] = 10
	_, err := f.client().Run(t.Context(), publish(assets(t, "a.zip", "b.zip")))
	if err == nil {
		t.Fatal("no error")
	}
	if len(f.releases) != 1 || !f.releases[0].Draft {
		t.Errorf("a failed upload left %+v", f.releases)
	}
}

func TestTokenNotInErrors(t *testing.T) {
	f := newFake(t)
	// The server echoes the Authorization header it was sent; a wrong
	// one is still a secret.
	c := New(f.srv.URL, release.NewToken(secret+"x", "$GITHUB_TOKEN"))
	_, err := c.Run(t.Context(), publish(assets(t, "a.zip")))
	if err == nil {
		t.Fatal("no error")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(fmt.Sprintf("%+v %#v", err, c), secret) {
		t.Errorf("the token is in %q", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("err = %v", err)
	}

	// Nor in a transport error.
	c = New("http://127.0.0.1:1", release.NewToken(secret, "$GITHUB_TOKEN"))
	c.Retries = 0
	if _, err := c.FindRelease(t.Context(), "alice/hello", "v1"); err == nil || strings.Contains(err.Error(), secret) {
		t.Errorf("err = %v", err)
	}
}

func TestCommitExists(t *testing.T) {
	f := newFake(t)
	c := f.client()
	if ok, err := c.CommitExists(t.Context(), "alice/hello", "abc123"); !ok || err != nil {
		t.Errorf("pushed commit: %v %v", ok, err)
	}
	if ok, err := c.CommitExists(t.Context(), "alice/hello", "def456"); ok || err != nil {
		t.Errorf("unpushed commit: %v %v", ok, err)
	}
}

func TestContents(t *testing.T) {
	f := newFake(t)
	c := f.client()
	sha, err := c.FileSHA(t.Context(), "alice/homebrew-tap", "Casks/hello.rb")
	if err != nil || sha != "" {
		t.Fatalf("missing file: %q %v", sha, err)
	}
	if err := c.PutFile(t.Context(), "alice/homebrew-tap", "Casks/hello.rb", "msg", []byte("cask"), ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.putBody["sha"]; ok {
		t.Error("a new file was sent with a sha")
	}
	content, _ := base64.StdEncoding.DecodeString(f.putBody["content"].(string))
	if string(content) != "cask" || f.putBody["message"] != "msg" {
		t.Errorf("PUT %v", f.putBody)
	}
	if sha, err := c.FileSHA(t.Context(), "alice/homebrew-tap", "Casks/hello.rb"); sha != "newsha" || err != nil {
		t.Errorf("existing file: %q %v", sha, err)
	}
}

func TestURLs(t *testing.T) {
	for _, c := range []struct{ api, uploads, web, host string }{
		{"https://api.github.com", "https://uploads.github.com", "https://github.com", "github.com"},
		{"https://ghe.example.com/api/v3/", "https://ghe.example.com/api/uploads", "https://ghe.example.com", "ghe.example.com"},
	} {
		if got := UploadsURL(c.api); got != c.uploads {
			t.Errorf("UploadsURL(%s) = %s", c.api, got)
		}
		if got := WebURL(c.api); got != c.web {
			t.Errorf("WebURL(%s) = %s", c.api, got)
		}
		if got := Host(c.api); got != c.host {
			t.Errorf("Host(%s) = %s", c.api, got)
		}
	}
	if got := DownloadURL("https://github.com", "alice/hello", "v1.0.0", "hello world.zip"); got != "https://github.com/alice/hello/releases/download/v1.0.0/hello%20world.zip" {
		t.Errorf("DownloadURL = %s", got)
	}
	plan := Plan("https://api.github.com", publish(assets(t, "a.zip")))
	if len(plan) != 4 || !strings.Contains(plan[2], "https://uploads.github.com/repos/alice/hello/releases/{id}/assets?name=a.zip") {
		t.Errorf("plan:\n%s", strings.Join(plan, "\n"))
	}
}
