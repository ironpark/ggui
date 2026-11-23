package updater

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Sentinel errors returned by the updater package.
var (
	// ErrInvalidSignature indicates an Ed25519 signature failed verification or was malformed.
	ErrInvalidSignature = errors.New("updater: invalid ed25519 signature")

	// ErrMissingSignature indicates an update enclosure had no Ed25519 signature
	// while signature verification is required.
	ErrMissingSignature = errors.New("updater: missing ed25519 signature")

	// ErrInsecureURL indicates a feed or download URL (or redirect target) used
	// an insecure scheme such as http:// without AllowInsecureHTTP enabled.
	ErrInsecureURL = errors.New("updater: insecure URL (HTTPS required)")

	// ErrHashMismatch indicates a downloaded archive's SHA-256 digest did not
	// match the expected value from the appcast enclosure.
	ErrHashMismatch = errors.New("updater: SHA-256 hash mismatch")

	// ErrDownloadTooLarge indicates a download exceeded Config.MaxDownloadBytes.
	ErrDownloadTooLarge = errors.New("updater: download exceeds maximum allowed size")

	// ErrUnsupportedPlatform indicates applying updates is not supported on this
	// platform or no platform Applier has been registered.
	ErrUnsupportedPlatform = fmt.Errorf("updater: unsupported platform: %w", errors.ErrUnsupported)
)

// GenerateKeyPair generates a new Sparkle 2-compatible Ed25519 key pair.
//
// publicKeyBase64 is the standard base64 encoding of the 32-byte Ed25519 public
// key (Sparkle's SUPublicEDKey). privateKeyBase64 is the standard base64
// encoding of the 32-byte private key seed (matching Sparkle 2's generate_keys).
func GenerateKeyPair() (publicKeyBase64, privateKeyBase64 string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("updater: generating ed25519 key pair: %w", err)
	}
	return base64.StdEncoding.EncodeToString(pub),
		base64.StdEncoding.EncodeToString(priv.Seed()),
		nil
}

// ParsePublicKey decodes a base64-encoded 32-byte Ed25519 public key.
func ParsePublicKey(b64 string) (ed25519.PublicKey, error) {
	b64 = strings.TrimSpace(b64)
	if b64 == "" {
		return nil, errors.New("updater: empty ed25519 public key")
	}
	raw, err := decodeBase64Std(b64)
	if err != nil {
		return nil, fmt.Errorf("updater: decoding ed25519 public key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("updater: invalid ed25519 public key length %d (want %d)", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

// ParsePrivateKey decodes a base64-encoded Ed25519 private key.
// Both 32-byte seeds (Sparkle 2 generate_keys format) and 64-byte full private
// keys are accepted.
func ParsePrivateKey(b64 string) (ed25519.PrivateKey, error) {
	b64 = strings.TrimSpace(b64)
	if b64 == "" {
		return nil, errors.New("updater: empty ed25519 private key")
	}
	raw, err := decodeBase64Std(b64)
	if err != nil {
		return nil, fmt.Errorf("updater: decoding ed25519 private key: %w", err)
	}
	switch len(raw) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(raw), nil
	case ed25519.PrivateKeySize:
		derived := ed25519.NewKeyFromSeed(raw[:ed25519.SeedSize])
		if !bytes.Equal(derived, raw) {
			return nil, errors.New("updater: inconsistent 64-byte ed25519 private key")
		}
		return ed25519.PrivateKey(raw), nil
	default:
		return nil, fmt.Errorf("updater: invalid ed25519 private key length %d (want %d or %d)",
			len(raw), ed25519.SeedSize, ed25519.PrivateKeySize)
	}
}

// SignBytes signs raw bytes with the base64-encoded Ed25519 private key and
// returns the base64-encoded 64-byte signature, matching Sparkle 2's
// sparkle:edSignature format.
func SignBytes(privateKeyBase64 string, data []byte) (string, error) {
	priv, err := ParsePrivateKey(privateKeyBase64)
	if err != nil {
		return "", err
	}
	sig := ed25519.Sign(priv, data)
	return base64.StdEncoding.EncodeToString(sig), nil
}

// SignFile reads the file at path and signs its raw contents with the
// base64-encoded Ed25519 private key, returning the base64 signature and byte size.
func SignFile(privateKeyBase64, path string) (sigBase64 string, size int64, err error) {
	priv, err := ParsePrivateKey(privateKeyBase64)
	if err != nil {
		return "", 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, fmt.Errorf("updater: reading file to sign: %w", err)
	}
	sig := ed25519.Sign(priv, data)
	return base64.StdEncoding.EncodeToString(sig), int64(len(data)), nil
}

// VerifyBytes verifies that sigBase64 is a valid Sparkle 2 Ed25519 signature
// over data using publicKeyBase64.
func VerifyBytes(publicKeyBase64 string, data []byte, sigBase64 string) error {
	pub, err := ParsePublicKey(publicKeyBase64)
	if err != nil {
		return err
	}
	sig, err := decodeSignature(sigBase64)
	if err != nil {
		return err
	}
	if !ed25519.Verify(pub, data, sig) {
		return fmt.Errorf("%w: verification failed", ErrInvalidSignature)
	}
	return nil
}

// VerifyFile verifies that sigBase64 is a valid Sparkle 2 Ed25519 signature
// over the raw contents of the file at path using publicKeyBase64.
func VerifyFile(publicKeyBase64, path, sigBase64 string) error {
	pub, err := ParsePublicKey(publicKeyBase64)
	if err != nil {
		return err
	}
	sig, err := decodeSignature(sigBase64)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("updater: reading file to verify: %w", err)
	}
	if !ed25519.Verify(pub, data, sig) {
		return fmt.Errorf("%w: verification failed", ErrInvalidSignature)
	}
	return nil
}

func decodeSignature(sigBase64 string) ([]byte, error) {
	sigBase64 = strings.TrimSpace(sigBase64)
	if sigBase64 == "" {
		return nil, ErrMissingSignature
	}
	sig, err := decodeBase64Std(sigBase64)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid base64: %v", ErrInvalidSignature, err)
	}
	if len(sig) != ed25519.SignatureSize {
		return nil, fmt.Errorf("%w: unexpected length %d (want %d)", ErrInvalidSignature, len(sig), ed25519.SignatureSize)
	}
	return sig, nil
}

func decodeBase64Std(s string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(s)
	if err != nil && !strings.ContainsRune(s, '=') {
		if rawNoPad, rawErr := base64.RawStdEncoding.DecodeString(s); rawErr == nil {
			return rawNoPad, nil
		}
	}
	return raw, err
}
