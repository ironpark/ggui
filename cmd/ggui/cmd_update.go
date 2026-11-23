package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/release/appcast"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

const defaultUpdateKeyEnv = "GGUI_UPDATE_PRIVATE_KEY"

var updateCmd = &command{
	name:    "update",
	args:    "keygen|sign|verify|appcast [args]",
	summary: "generate Ed25519 keys, sign or verify files, and build Sparkle 2 appcast.xml feeds",
	help: `Update manages Sparkle 2-compatible Ed25519 update keys, file signatures,
and appcast.xml feeds.

  keygen   generate an Ed25519 key pair (32-byte public key for SUPublicEDKey
           and 32-byte private seed). With --out-private FILE the private key
           is written with mode 0600 instead of printed.
  sign     sign a release file with an Ed25519 private key from --key-file or
           $GGUI_UPDATE_PRIVATE_KEY (or --key-env) and print its Sparkle 2
           enclosure attributes: sparkle:edSignature, length and sparkle:sha256.
  verify   verify a release file against --signature and --public-key.
  appcast  scan --dir for updatable release artifacts (.zip, .dmg, -setup.exe,
           .tar.gz), sign each with Ed25519 (unless --allow-unsigned), merge
           with --merge when given, and write a Sparkle 2 appcast.xml feed.`,
	flags: func(fs *flag.FlagSet) func(context.Context, []string, []string) error {
		f := newUpdateFlags(fs)
		return func(ctx context.Context, pos, rest []string) error {
			return f.run(ctx, pos, rest, os.Stdout, os.Getenv)
		}
	},
}

type updateFlags struct {
	outPrivate    *string
	keyFile       *string
	keyEnv        *string
	signature     *string
	publicKey     *string
	dir           *string
	baseURL       *string
	version       *string
	out           *string
	merge         *string
	title         *string
	channel       *string
	minOS         *string
	notes         *string
	notesURL      *string
	rollout       *string
	critical      *bool
	allowUnsigned *bool
	maxItems      *int
	isSet         func(string) bool
}

func newUpdateFlags(fs *flag.FlagSet) *updateFlags {
	f := &updateFlags{isSet: func(name string) bool { return isSet(fs, name) }}
	f.outPrivate = fs.String("out-private", "", "with keygen, write the private key to `file` with mode 0600")
	f.keyFile = fs.String("key-file", "", "with sign and appcast, read the base64 Ed25519 private key from `file`")
	f.keyEnv = fs.String("key-env", defaultUpdateKeyEnv, "with sign and appcast, environment variable holding the base64 Ed25519 private key")
	f.signature = fs.String("signature", "", "with verify, the base64 Ed25519 `signature` (sparkle:edSignature)")
	f.publicKey = fs.String("public-key", "", "with verify, the base64 Ed25519 public `key` (SUPublicEDKey)")
	f.dir = fs.String("dir", "", "with appcast, `directory` containing the release artifacts")
	f.baseURL = fs.String("base-url", "", "with appcast, base `URL` where the release artifacts are hosted")
	f.version = fs.String("version", "", "with appcast, release `version` (for example 1.2.0)")
	f.out = fs.String("out", appcast.DefaultFileName, "with appcast, output appcast XML `file`")
	f.merge = fs.String("merge", "", "with appcast, existing appcast `path` or http(s) URL to merge with")
	f.title = fs.String("title", "", "with appcast, feed `title`")
	f.channel = fs.String("channel", "", "with appcast, release `channel` such as beta or nightly (empty is stable)")
	f.minOS = fs.String("min-os", "", "with appcast, minimum OS `version` (sparkle:minimumSystemVersion)")
	f.notes = fs.String("notes", "", "with appcast, Markdown or text `file` with release notes")
	f.notesURL = fs.String("notes-url", "", "with appcast, http(s) `URL` to release notes (sparkle:releaseNotesLink)")
	f.rollout = fs.String("rollout", "", "with appcast, phased rollout `duration` such as 24h")
	f.critical = fs.Bool("critical", false, "with appcast, mark the release as a critical update")
	f.allowUnsigned = fs.Bool("allow-unsigned", false, "with appcast, allow generating an unsigned feed when no private key is set")
	f.maxItems = fs.Int("max-items", 20, "with appcast, maximum number of distinct versions per channel to retain when merging")
	return f
}

var updateAllowedFlags = map[string][]string{
	"keygen": {"out-private"},
	"sign":   {"key-file", "key-env"},
	"verify": {"signature", "public-key"},
	"appcast": {
		"dir", "base-url", "version", "out", "merge", "title", "channel",
		"min-os", "notes", "notes-url", "rollout", "critical",
		"key-file", "key-env", "allow-unsigned", "max-items",
	},
}

var allUpdateFlagNames = []string{
	"out-private", "key-file", "key-env", "signature", "public-key",
	"dir", "base-url", "version", "out", "merge", "title", "channel",
	"min-os", "notes", "notes-url", "rollout", "critical",
	"allow-unsigned", "max-items",
}

func (f *updateFlags) run(ctx context.Context, pos, rest []string, stdout io.Writer, getenv func(string) string) error {
	if len(pos) == 0 || len(rest) > 0 {
		fmt.Fprintln(os.Stderr, "ggui update: takes keygen, sign, verify, or appcast")
		return errUsage
	}
	sub := pos[0]
	allowed, ok := updateAllowedFlags[sub]
	if !ok {
		fmt.Fprintf(os.Stderr, "ggui update: %q is not keygen, sign, verify, or appcast\n", sub)
		return errUsage
	}
	for _, name := range allUpdateFlagNames {
		if f.isSet(name) && !slices.Contains(allowed, name) {
			return fmt.Errorf("--%s cannot be used with ggui update %s", name, sub)
		}
	}
	switch sub {
	case "keygen":
		if len(pos) != 1 {
			fmt.Fprintln(os.Stderr, "ggui update keygen: takes no positional arguments")
			return errUsage
		}
		return f.runKeygen(stdout)
	case "sign":
		if len(pos) != 2 {
			fmt.Fprintln(os.Stderr, "ggui update sign: takes one file path")
			return errUsage
		}
		return f.runSign(pos[1], stdout, getenv)
	case "verify":
		if len(pos) != 2 {
			fmt.Fprintln(os.Stderr, "ggui update verify: takes one file path")
			return errUsage
		}
		return f.runVerify(pos[1])
	case "appcast":
		if len(pos) != 1 {
			fmt.Fprintln(os.Stderr, "ggui update appcast: takes no positional arguments")
			return errUsage
		}
		return f.runAppcast(ctx, getenv)
	}
	return errUsage
}

func (f *updateFlags) runKeygen(stdout io.Writer) error {
	pub, priv, err := appcast.GenerateKeyPair()
	if err != nil {
		return err
	}
	if outPath := strings.TrimSpace(*f.outPrivate); outPath != "" {
		if dir := filepath.Dir(outPath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		// O_EXCL refuses to overwrite an existing signing key and sets 0600 atomically.
		file, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("writing private key to %s: %w", outPath, err)
		}
		_, writeErr := io.WriteString(file, priv+"\n")
		closeErr := file.Close()
		if writeErr != nil {
			os.Remove(outPath)
			return fmt.Errorf("writing private key to %s: %w", outPath, writeErr)
		}
		if closeErr != nil {
			os.Remove(outPath)
			return fmt.Errorf("writing private key to %s: %w", outPath, closeErr)
		}
		fmt.Fprintf(stdout, "PublicKey (SUPublicEDKey): %s\n", pub)
		ui.Stderr.Success("Wrote Ed25519 private key to %s (mode 0600)", ui.Stderr.Cyan(shortPath(outPath)))
		ui.Stderr.Warn("Keep the private key secret: store it in $%s and never commit it to version control.", defaultUpdateKeyEnv)
		return nil
	}
	fmt.Fprintf(stdout, "PublicKey (SUPublicEDKey): %s\n", pub)
	fmt.Fprintf(stdout, "PrivateKey (%s): %s\n", defaultUpdateKeyEnv, priv)
	ui.Stderr.Warn("Keep the private key secret: store it in $%s and never commit it to version control.", defaultUpdateKeyEnv)
	return nil
}

func readUpdatePrivateKey(keyFile, keyEnv string, getenv func(string) string) (string, error) {
	if keyFile != "" {
		data, err := os.ReadFile(keyFile)
		if err != nil {
			return "", fmt.Errorf("reading --key-file %s: %w", keyFile, err)
		}
		key := strings.TrimSpace(string(data))
		if key == "" {
			return "", fmt.Errorf("--key-file %s is empty", keyFile)
		}
		if _, err := appcast.ParsePrivateKey(key); err != nil {
			return "", fmt.Errorf("--key-file %s: %w", keyFile, err)
		}
		return key, nil
	}
	env := strings.TrimSpace(keyEnv)
	if env == "" {
		env = defaultUpdateKeyEnv
	}
	key := strings.TrimSpace(getenv(env))
	if key == "" {
		return "", fmt.Errorf("no Ed25519 private key: pass --key-file or set $%s (run 'ggui update keygen' to create one)", env)
	}
	if _, err := appcast.ParsePrivateKey(key); err != nil {
		return "", fmt.Errorf("$%s: %w", env, err)
	}
	return key, nil
}

func (f *updateFlags) runSign(file string, stdout io.Writer, getenv func(string) string) error {
	priv, err := readUpdatePrivateKey(*f.keyFile, *f.keyEnv, getenv)
	if err != nil {
		return err
	}
	sig, size, sum, err := appcast.SignFile(priv, file)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "sparkle:edSignature=%q length=\"%d\" sparkle:sha256=%q\n", sig, size, sum)
	return nil
}

func (f *updateFlags) runVerify(file string) error {
	if strings.TrimSpace(*f.signature) == "" {
		return errors.New("--signature is required")
	}
	if strings.TrimSpace(*f.publicKey) == "" {
		return errors.New("--public-key is required")
	}
	if err := appcast.VerifyFile(*f.publicKey, file, *f.signature); err != nil {
		return fmt.Errorf("verifying %s: %w", file, err)
	}
	ui.Stderr.Success("Verified %s", ui.Stderr.Cyan(shortPath(file)))
	return nil
}

func (f *updateFlags) runAppcast(ctx context.Context, getenv func(string) string) error {
	dir := strings.TrimSpace(*f.dir)
	if dir == "" {
		return errors.New("--dir is required")
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("--dir: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("--dir %q is not a directory", dir)
	}
	baseURL := strings.TrimSpace(*f.baseURL)
	if baseURL == "" {
		return errors.New("--base-url is required")
	}
	if !isHTTPURL(baseURL) {
		return fmt.Errorf("--base-url %q must be an http or https URL", baseURL)
	}
	ver := strings.TrimSpace(*f.version)
	if len(ver) > 1 && (ver[0] == 'v' || ver[0] == 'V') && ver[1] >= '0' && ver[1] <= '9' {
		ver = ver[1:]
	}
	if ver == "" {
		return errors.New("--version is required")
	}
	outPath := strings.TrimSpace(*f.out)
	if outPath == "" {
		return errors.New("--out must not be empty")
	}
	if *f.maxItems <= 0 {
		return fmt.Errorf("--max-items %d must be greater than zero", *f.maxItems)
	}
	ch := strings.TrimSpace(*f.channel)
	if strings.ContainsAny(ch, " \t\n\r") {
		return fmt.Errorf("--channel %q must not contain spaces or newlines", ch)
	}
	notesURL := strings.TrimSpace(*f.notesURL)
	if notesURL != "" && !isHTTPURL(notesURL) {
		return fmt.Errorf("--notes-url %q must be an http or https URL", notesURL)
	}
	mergeSrc := strings.TrimSpace(*f.merge)
	if mergeSrc != "" {
		if strings.ContainsAny(mergeSrc, "\r\n\x00") {
			return fmt.Errorf("--merge %q must be one path or http(s) URL", mergeSrc)
		}
		if strings.Contains(mergeSrc, "://") && !isHTTPURL(mergeSrc) {
			return fmt.Errorf("--merge %q must be a local path or an http or https URL", mergeSrc)
		}
	}
	var rolloutSec int64
	if r := strings.TrimSpace(*f.rollout); r != "" {
		d, err := time.ParseDuration(r)
		if err != nil || d < time.Second {
			return fmt.Errorf("--rollout %q must be a duration of at least 1s, such as 24h", r)
		}
		rolloutSec = int64(d / time.Second)
	}
	var notesText string
	if notesFile := strings.TrimSpace(*f.notes); notesFile != "" {
		data, err := os.ReadFile(notesFile)
		if err != nil {
			return fmt.Errorf("--notes: %w", err)
		}
		notesText = string(data)
	}

	var privKey string
	if *f.keyFile != "" {
		privKey, err = readUpdatePrivateKey(*f.keyFile, *f.keyEnv, getenv)
		if err != nil {
			return err
		}
	} else {
		env := strings.TrimSpace(*f.keyEnv)
		if env == "" {
			env = defaultUpdateKeyEnv
		}
		if raw := strings.TrimSpace(getenv(env)); raw != "" {
			if _, err := appcast.ParsePrivateKey(raw); err != nil {
				return fmt.Errorf("$%s: %w", env, err)
			}
			privKey = raw
		} else if *f.allowUnsigned {
			ui.Stderr.Warn("no signing key provided; generating an unsigned %s", filepath.Base(outPath))
		} else {
			return fmt.Errorf("no Ed25519 private key: pass --key-file, set $%s, or pass --allow-unsigned", env)
		}
	}

	assets, err := appcast.ScanDir(dir)
	if err != nil {
		return fmt.Errorf("scanning %s: %w", dir, err)
	}

	// If --min-os is given for a multi-OS release directory containing macOS
	// alongside Windows/Linux, apply it only to macOS so a macOS version like
	// "13.0" does not block Windows or Linux clients.
	opts := appcast.BuildOptions{
		Version:               ver,
		ShortVersion:          ver,
		Title:                 "Version " + ver,
		PubDate:               time.Now().UTC(),
		Channel:               ch,
		PhasedRolloutInterval: rolloutSec,
		CriticalUpdate:        *f.critical,
		ReleaseNotes:          notesText,
		ReleaseNotesLink:      notesURL,
		PrivateKey:            privKey,
		AllowUnsigned:         *f.allowUnsigned,
		BaseURL:               baseURL,
		AppcastFile:           filepath.Base(outPath),
	}
	if minOS := strings.TrimSpace(*f.minOS); minOS != "" {
		hasMac, hasOther := false, false
		for _, a := range assets {
			switch a.Target {
			case "macos":
				hasMac = true
			case "windows", "linux":
				hasOther = true
			}
		}
		if hasMac && hasOther {
			opts.MinMacOSVersion = minOS
		} else {
			opts.MinimumSystemVersion = minOS
		}
	}

	items, err := appcast.BuildFromAssets(assets, opts)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return fmt.Errorf("no updatable release artifacts found in %s (expected .zip, .dmg, -setup.exe, or .tar.gz)", dir)
	}

	var existing *appcast.Feed
	if mergeSrc != "" {
		var found bool
		existing, found, err = appcast.LoadSource(ctx, mergeSrc)
		if err != nil {
			return fmt.Errorf("--merge: %w", err)
		}
		if !found {
			ui.Stderr.Warn("merge source %s was not found; starting a new feed", mergeSrc)
		}
	}

	feed := appcast.Merge(existing, items, *f.maxItems)
	if t := strings.TrimSpace(*f.title); t != "" {
		feed.Title = t
	} else if feed.Title == "" {
		feed.Title = "Updates"
	}
	if feed.Language == "" {
		feed.Language = "en"
	}

	data, err := appcast.Marshal(feed)
	if err != nil {
		return err
	}
	if parent := filepath.Dir(outPath); parent != "" && parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return err
	}
	ui.Stderr.Success("Wrote %s (%d items)", ui.Stderr.Cyan(shortPath(outPath)), len(feed.Items))
	return nil
}

func isHTTPURL(s string) bool {
	if s == "" || strings.ContainsAny(s, "\r\n\x00") {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return (scheme == "http" || scheme == "https") && u.Host != ""
}
