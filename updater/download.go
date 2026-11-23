package updater

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const downloadBufferSize = 32 * 1024

// Download streams the selected release's enclosure into CacheDir, verifies its
// size limit, optional SHA-256 digest, and Ed25519 signature (unless
// Config.AllowUnsigned is set), and atomically moves the verified archive into
// its final path.
func (u *Updater) Download(ctx context.Context, rel *Release, onProgress func(DownloadProgress)) (*DownloadedUpdate, error) {
	if rel == nil {
		return nil, errors.New("updater: nil release")
	}
	parsedURL, err := validateURLString(rel.Enclosure.URL, u.cfg.AllowInsecureHTTP)
	if err != nil {
		return nil, err
	}

	sig := strings.TrimSpace(rel.Enclosure.EdSignature)
	if !u.cfg.AllowUnsigned {
		if sig == "" {
			return nil, ErrMissingSignature
		}
		if _, err := decodeSignature(sig); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("updater: creating download request: %w", redactURLError(err))
	}
	if u.cfg.UserAgent != "" {
		req.Header.Set("User-Agent", u.cfg.UserAgent)
	}

	client := u.httpClient(0)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("updater: downloading %s: %w", redactURL(parsedURL), redactURLError(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("updater: download %s returned HTTP %d", redactURL(parsedURL), resp.StatusCode)
	}
	if resp.ContentLength > u.cfg.MaxDownloadBytes {
		return nil, fmt.Errorf("%w: Content-Length %d exceeds limit %d",
			ErrDownloadTooLarge, resp.ContentLength, u.cfg.MaxDownloadBytes)
	}

	if err := os.MkdirAll(u.cfg.CacheDir, 0o750); err != nil {
		return nil, fmt.Errorf("updater: creating cache directory: %w", err)
	}

	safeName := safeDownloadFilename(parsedURL, rel)
	finalPath := filepath.Join(u.cfg.CacheDir, safeName)

	partFile, err := os.CreateTemp(u.cfg.CacheDir, ".update-*.part")
	if err != nil {
		return nil, fmt.Errorf("updater: creating temp download file: %w", err)
	}
	partPath := partFile.Name()
	removePart := true
	partClosed := false
	defer func() {
		if !partClosed {
			_ = partFile.Close()
		}
		if removePart {
			_ = os.Remove(partPath)
		}
	}()

	total := resp.ContentLength
	if total <= 0 && rel.Enclosure.Length > 0 {
		total = rel.Enclosure.Length
	}
	if total <= 0 {
		total = -1
	}

	hasher := sha256.New()
	limited := io.LimitReader(resp.Body, u.cfg.MaxDownloadBytes+1)
	buf := make([]byte, downloadBufferSize)
	var downloaded int64
	reportedDone := false

	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, readErr := limited.Read(buf)
		if n > 0 {
			downloaded += int64(n)
			if downloaded > u.cfg.MaxDownloadBytes {
				return nil, fmt.Errorf("%w: received more than %d bytes",
					ErrDownloadTooLarge, u.cfg.MaxDownloadBytes)
			}
			chunk := buf[:n]
			if _, err := partFile.Write(chunk); err != nil {
				return nil, fmt.Errorf("updater: writing download file: %w", err)
			}
			_, _ = hasher.Write(chunk)
			if onProgress != nil {
				onProgress(makeDownloadProgress(downloaded, total))
				if total > 0 && downloaded >= total {
					reportedDone = true
				}
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("updater: reading download stream: %w", readErr)
		}
	}

	if err := partFile.Close(); err != nil {
		partClosed = true
		return nil, fmt.Errorf("updater: closing download file: %w", err)
	}
	partClosed = true

	if onProgress != nil && !reportedDone {
		finalTotal := total
		if finalTotal <= 0 {
			finalTotal = downloaded
		}
		onProgress(makeDownloadProgress(downloaded, finalTotal))
	}

	gotSHA := hex.EncodeToString(hasher.Sum(nil))
	if wantSHA := normalizeSHA256(rel.Enclosure.SHA256); wantSHA != "" {
		if subtle.ConstantTimeCompare([]byte(gotSHA), []byte(wantSHA)) != 1 {
			return nil, fmt.Errorf("%w: got %s, want %s", ErrHashMismatch, gotSHA, wantSHA)
		}
	}

	verified := false
	if u.cfg.PublicKey != "" && (sig != "" || !u.cfg.AllowUnsigned) {
		if err := VerifyFile(u.cfg.PublicKey, partPath, sig); err != nil {
			return nil, err
		}
		verified = true
	} else if !u.cfg.AllowUnsigned {
		return nil, ErrMissingSignature
	}

	if err := os.Rename(partPath, finalPath); err != nil {
		return nil, fmt.Errorf("updater: moving verified update into cache: %w", err)
	}
	removePart = false

	return &DownloadedUpdate{
		Release:  *rel,
		FilePath: finalPath,
		Size:     downloaded,
		SHA256:   gotSHA,
		Verified: verified,
	}, nil
}

func makeDownloadProgress(downloaded, total int64) DownloadProgress {
	var frac float64
	if total > 0 {
		frac = float64(downloaded) / float64(total)
		if frac > 1 {
			frac = 1
		}
	}
	return DownloadProgress{
		Downloaded: downloaded,
		Total:      total,
		Fraction:   frac,
	}
}

func normalizeSHA256(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) >= 7 && strings.EqualFold(s[:7], "sha256:") {
		s = strings.TrimSpace(s[7:])
	}
	return strings.ToLower(s)
}

func sha256File(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	cp := *u
	cp.User = nil
	cp.RawQuery = ""
	cp.ForceQuery = false
	cp.Fragment = ""
	cp.RawFragment = ""
	return cp.String()
}

func redactURLString(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		s := raw
		if idx := strings.IndexAny(s, "?#"); idx >= 0 {
			s = s[:idx]
		}
		if schemeEnd := strings.Index(s, "://"); schemeEnd >= 0 {
			rest := s[schemeEnd+3:]
			authority := rest
			if slashIdx := strings.IndexByte(rest, '/'); slashIdx >= 0 {
				authority = rest[:slashIdx]
			}
			if atIdx := strings.LastIndexByte(authority, '@'); atIdx >= 0 {
				s = s[:schemeEnd+3] + rest[atIdx+1:]
			}
		}
		return s
	}
	return redactURL(u)
}

func redactURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return &url.Error{
			Op:  ue.Op,
			URL: redactURLString(ue.URL),
			Err: ue.Err,
		}
	}
	return err
}

func validateURLString(raw string, allowInsecure bool) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("updater: empty URL")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("updater: invalid URL %q: %w", redactURLString(raw), redactURLError(err))
	}
	if err := validateURL(u, allowInsecure); err != nil {
		return nil, err
	}
	return u, nil
}

func validateURL(u *url.URL, allowInsecure bool) error {
	if u == nil {
		return errors.New("updater: nil URL")
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https":
		// Always allowed.
	case "http":
		if !allowInsecure {
			return fmt.Errorf("%w: %s", ErrInsecureURL, redactURL(u))
		}
	default:
		return fmt.Errorf("%w: unsupported scheme %q", ErrInsecureURL, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("updater: URL %q missing host", redactURL(u))
	}
	return nil
}

func (u *Updater) httpClient(defaultTimeout time.Duration) *http.Client {
	var client http.Client
	var baseCheckRedirect func(*http.Request, []*http.Request) error
	if u.cfg.HTTPClient != nil {
		client = *u.cfg.HTTPClient
		baseCheckRedirect = u.cfg.HTTPClient.CheckRedirect
	} else {
		client = http.Client{
			Timeout: defaultTimeout,
		}
	}

	allowInsecure := u.cfg.AllowInsecureHTTP
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := validateURL(req.URL, allowInsecure); err != nil {
			return err
		}
		if baseCheckRedirect != nil {
			return baseCheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return &client
}

func safeDownloadFilename(parsed *url.URL, rel *Release) string {
	base := ""
	if parsed != nil {
		base = path.Base(parsed.Path)
	}
	base = sanitizePathComponent(base)
	if base == "" || base == "." || base == ".." {
		ver := sanitizePathComponent(rel.CompareVersion())
		if ver == "" {
			ver = "latest"
		}
		return "update-" + ver + ".bin"
	}
	return base
}

func sanitizePathComponent(name string) string {
	name = strings.Map(func(r rune) rune {
		if r < ' ' || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, ". ")
	if isWindowsReservedDeviceName(name) {
		name = "_" + name
	}
	return name
}

func isWindowsReservedDeviceName(name string) bool {
	if name == "" {
		return false
	}
	base := name
	if dot := strings.IndexByte(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	base = strings.ToUpper(strings.TrimRight(base, " "))
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(base) == 4 {
		prefix := base[:3]
		digit := base[3]
		if (prefix == "COM" || prefix == "LPT") && digit >= '1' && digit <= '9' {
			return true
		}
	}
	return false
}
