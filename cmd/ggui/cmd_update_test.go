package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/release/appcast"
)

func runUpdateForTest(t *testing.T, args []string, env map[string]string) (string, error) {
	t.Helper()
	fs := flag.NewFlagSet("ggui update", flag.ContinueOnError)
	f := newUpdateFlags(fs)
	pos, rest, err := parseInterspersed(fs, args)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	getenv := func(k string) string { return env[k] }
	err = f.run(t.Context(), pos, rest, &out, getenv)
	return out.String(), err
}

func TestUpdateKeygenSignVerify(t *testing.T) {
	quiet(t)
	dir := t.TempDir()

	// 1. keygen to stdout
	stdout, err := runUpdateForTest(t, []string{"keygen"}, nil)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	var pub1, priv1 string
	for line := range strings.SplitSeq(strings.TrimSpace(stdout), "\n") {
		if v, ok := strings.CutPrefix(line, "PublicKey (SUPublicEDKey): "); ok {
			pub1 = strings.TrimSpace(v)
		}
		if v, ok := strings.CutPrefix(line, "PrivateKey (GGUI_UPDATE_PRIVATE_KEY): "); ok {
			priv1 = strings.TrimSpace(v)
		}
	}
	if pub1 == "" || priv1 == "" {
		t.Fatalf("unexpected keygen output:\n%s", stdout)
	}
	derived, err := appcast.PublicKeyFromPrivate(priv1)
	if err != nil || derived != pub1 {
		t.Fatalf("derived pub = %q (%v), want %q", derived, err, pub1)
	}

	// 2. keygen --out-private writes 0600 and refuses overwrite.
	keyFile := filepath.Join(dir, "keys", "ed25519.key")
	stdout, err = runUpdateForTest(t, []string{"keygen", "--out-private", keyFile}, nil)
	if err != nil {
		t.Fatalf("keygen --out-private: %v", err)
	}
	if strings.Contains(stdout, "PrivateKey") {
		t.Errorf("keygen --out-private printed private key to stdout:\n%s", stdout)
	}
	pub2 := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stdout), "PublicKey (SUPublicEDKey): "))
	info, err := os.Stat(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("key file perm = %o, want 0600", info.Mode().Perm())
	}
	if _, err := runUpdateForTest(t, []string{"keygen", "--out-private", keyFile}, nil); err == nil {
		t.Error("keygen --out-private overwrote an existing key file")
	}

	// 3. sign with --key-file and verify
	sample := filepath.Join(dir, "sample.zip")
	if err := os.WriteFile(sample, []byte("release archive data"), 0o644); err != nil {
		t.Fatal(err)
	}
	signOut, err := runUpdateForTest(t, []string{"sign", sample, "--key-file", keyFile}, nil)
	if err != nil {
		t.Fatalf("sign --key-file: %v", err)
	}
	re := regexp.MustCompile(`sparkle:edSignature="([^"]+)" length="(\d+)" sparkle:sha256="([0-9a-f]{64})"`)
	m := re.FindStringSubmatch(signOut)
	if m == nil {
		t.Fatalf("unexpected sign output: %q", signOut)
	}
	sig, length := m[1], m[2]
	if length != "20" {
		t.Errorf("length = %q, want 20", length)
	}

	if _, err := runUpdateForTest(t, []string{"verify", sample, "--signature", sig, "--public-key", pub2}, nil); err != nil {
		t.Fatalf("verify valid: %v", err)
	}
	// Tampered file must fail verify.
	if err := os.WriteFile(sample, []byte("release archive data!"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runUpdateForTest(t, []string{"verify", sample, "--signature", sig, "--public-key", pub2}, nil); err == nil {
		t.Error("verify succeeded on tampered file")
	}

	// 4. sign with custom --key-env
	if _, err := runUpdateForTest(t, []string{"sign", sample, "--key-env", "MY_SIGN_KEY"}, map[string]string{"MY_SIGN_KEY": priv1}); err != nil {
		t.Fatalf("sign --key-env: %v", err)
	}
	if _, err := runUpdateForTest(t, []string{"sign", sample}, nil); err == nil {
		t.Error("sign without key succeeded")
	}
}

func TestUpdateAppcast(t *testing.T) {
	quiet(t)
	pub, priv, err := appcast.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	keyFile := filepath.Join(root, "private.key")
	if err := os.WriteFile(keyFile, []byte(priv+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	notesFile := filepath.Join(root, "notes.md")
	if err := os.WriteFile(notesFile, []byte("## 1.2.0\n- Faster updates\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build v1.1.0 first so we can test --merge.
	dir11 := filepath.Join(root, "v1.1.0")
	if err := os.MkdirAll(dir11, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir11, "myapp-1.1.0-macos-universal.zip"), []byte("mac-1.1.0"), 0o644); err != nil {
		t.Fatal(err)
	}
	feedPath := filepath.Join(root, "out", "appcast.xml")
	if _, err := runUpdateForTest(t, []string{
		"appcast",
		"--dir", dir11,
		"--base-url", "https://example.com/releases/v1.1.0",
		"--version", "1.1.0",
		"--title", "MyApp",
		"--out", feedPath,
		"--key-file", keyFile,
	}, nil); err != nil {
		t.Fatalf("appcast 1.1.0: %v", err)
	}

	// Build v1.2.0 and merge into v1.1.0 feed.
	dir12 := filepath.Join(root, "v1.2.0")
	if err := os.MkdirAll(dir12, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"myapp-1.2.0-macos-universal.zip":     "mac-zip-1.2.0",
		"myapp-1.2.0-macos-universal.pkg":     "mac-pkg-skipped",
		"myapp-1.2.0-windows-amd64-setup.exe": "win-setup-1.2.0",
		"myapp-1.2.0-windows-amd64.zip":       "win-zip-1.2.0",
		"myapp-1.2.0-linux-arm64.tar.gz":      "linux-tar-1.2.0",
		"myapp_1.2.0_arm64.deb":               "linux-deb-skipped",
		"SHA256SUMS":                          "sums-skipped",
	} {
		if err := os.WriteFile(filepath.Join(dir12, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := runUpdateForTest(t, []string{
		"appcast",
		"--dir", dir12,
		"--base-url", "https://example.com/releases/v1.2.0",
		"--version", "v1.2.0",
		"--out", feedPath,
		"--merge", feedPath,
		"--title", "MyApp",
		"--channel", "beta",
		"--min-os", "13.0",
		"--notes", notesFile,
		"--notes-url", "https://example.com/releases/v1.2.0/notes",
		"--rollout", "24h",
		"--critical",
		"--max-items", "5",
		"--key-env", "GGUI_UPDATE_PRIVATE_KEY",
	}, map[string]string{"GGUI_UPDATE_PRIVATE_KEY": priv}); err != nil {
		t.Fatalf("appcast 1.2.0: %v", err)
	}

	data, err := os.ReadFile(feedPath)
	if err != nil {
		t.Fatal(err)
	}
	feed, err := appcast.Parse(data)
	if err != nil {
		t.Fatalf("Parse generated appcast: %v\n%s", err, data)
	}
	// 4 eligible from 1.2.0 + 1 from 1.1.0 = 5 items.
	if len(feed.Items) != 5 {
		t.Fatalf("got %d items, want 5:\n%s", len(feed.Items), data)
	}
	if feed.Title != "MyApp" {
		t.Errorf("feed.Title = %q, want MyApp", feed.Title)
	}
	for _, it := range feed.Items {
		if it.Version == "1.2.0" {
			if it.Channel != "beta" || it.PhasedRolloutInterval != 86400 || !it.CriticalUpdate || it.ReleaseNotesLink != "https://example.com/releases/v1.2.0/notes" || !strings.Contains(it.ReleaseNotes, "Faster updates") {
				t.Errorf("1.2.0 metadata wrong: %+v", it)
			}
			base := filepath.Base(it.Enclosure.URL)
			if err := appcast.VerifyFile(pub, filepath.Join(dir12, base), it.Enclosure.EdSignature); err != nil {
				t.Errorf("signature verify failed for %s: %v", base, err)
			}
			if it.Enclosure.OS == "macos" && it.MinimumSystemVersion != "13.0" {
				t.Errorf("macOS min-os = %q, want 13.0", it.MinimumSystemVersion)
			}
			if it.Enclosure.OS != "macos" && it.MinimumSystemVersion != "" {
				t.Errorf("%s min-os = %q, want empty", it.Enclosure.OS, it.MinimumSystemVersion)
			}
			if strings.HasSuffix(base, "-setup.exe") && it.Enclosure.InstallerArguments != "--silent" {
				t.Errorf("setup.exe installerArguments = %q, want --silent", it.Enclosure.InstallerArguments)
			}
		}
	}

	// Unsigned mode works without a key.
	unsignedPath := filepath.Join(root, "unsigned.xml")
	if _, err := runUpdateForTest(t, []string{
		"appcast",
		"--dir", dir11,
		"--base-url", "https://example.com/releases/v1.1.0",
		"--version", "1.1.0",
		"--out", unsignedPath,
		"--allow-unsigned",
	}, nil); err != nil {
		t.Fatalf("appcast --allow-unsigned: %v", err)
	}
	// Without key and without --allow-unsigned fails.
	if _, err := runUpdateForTest(t, []string{
		"appcast",
		"--dir", dir11,
		"--base-url", "https://example.com/releases/v1.1.0",
		"--version", "1.1.0",
		"--out", unsignedPath,
	}, nil); err == nil {
		t.Error("appcast without key or --allow-unsigned succeeded")
	}
	// Uppercase V before digit is stripped and --rollout 1s is accepted.
	vUpperPath := filepath.Join(root, "vupper.xml")
	if _, err := runUpdateForTest(t, []string{
		"appcast",
		"--dir", dir11,
		"--base-url", "https://example.com/releases/v1.1.0",
		"--version", "V1.1.0",
		"--rollout", "1s",
		"--out", vUpperPath,
		"--allow-unsigned",
	}, nil); err != nil {
		t.Fatalf("appcast V1.1.0 --rollout 1s: %v", err)
	}
	vUpperData, err := os.ReadFile(vUpperPath)
	if err != nil {
		t.Fatal(err)
	}
	vUpperFeed, err := appcast.Parse(vUpperData)
	if err != nil {
		t.Fatal(err)
	}
	if len(vUpperFeed.Items) != 1 || vUpperFeed.Items[0].Version != "1.1.0" || vUpperFeed.Items[0].PhasedRolloutInterval != 1 {
		t.Errorf("V1.1.0 feed item = %+v, want version=1.1.0 rollout=1", vUpperFeed.Items)
	}
}

func TestUpdateUsageAndErrors(t *testing.T) {
	quiet(t)
	var out, errOut bytes.Buffer
	for _, args := range [][]string{
		{"update"},
		{"update", "unknown"},
		{"update", "keygen", "extra"},
		{"update", "keygen", "--dir", "."},
		{"update", "sign"},
		{"update", "sign", "a", "b"},
		{"update", "verify", "a"},
		{"update", "appcast"},
		{"update", "appcast", "--dir", t.TempDir(), "--base-url", "ftp://bad", "--version", "1.0.0", "--allow-unsigned"},
		{"update", "appcast", "--dir", t.TempDir(), "--base-url", "https://", "--version", "1.0.0", "--allow-unsigned"},
		{"update", "appcast", "--dir", t.TempDir(), "--base-url", "http:///path", "--version", "1.0.0", "--allow-unsigned"},
		{"update", "appcast", "--dir", t.TempDir(), "--base-url", "https://example.com\nfoo", "--version", "1.0.0", "--allow-unsigned"},
		{"update", "appcast", "--dir", t.TempDir(), "--base-url", "https://example.com\x00", "--version", "1.0.0", "--allow-unsigned"},
		{"update", "appcast", "--dir", t.TempDir(), "--base-url", "https://example.com", "--notes-url", "https://", "--version", "1.0.0", "--allow-unsigned"},
		{"update", "appcast", "--dir", t.TempDir(), "--base-url", "https://example.com", "--merge", "ftp://example.com/appcast.xml", "--version", "1.0.0", "--allow-unsigned"},
		{"update", "appcast", "--dir", t.TempDir(), "--base-url", "https://example.com", "--merge", "https://", "--version", "1.0.0", "--allow-unsigned"},
		{"update", "appcast", "--dir", t.TempDir(), "--base-url", "https://example.com", "--version", "1.0.0", "--rollout", "-5m", "--allow-unsigned"},
		{"update", "appcast", "--dir", t.TempDir(), "--base-url", "https://example.com", "--version", "1.0.0", "--rollout", "500ms", "--allow-unsigned"},
	} {
		if code := run(args, &out, &errOut); code == 0 {
			t.Errorf("%q unexpectedly succeeded", args)
		}
	}
}
