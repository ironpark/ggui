package macos

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// KeychainIdentity is a valid code-signing identity from the macOS keychain.
type KeychainIdentity struct {
	Fingerprint string // 40-char uppercase SHA-1 hex
	Description string // e.g. "Apple Development: Alice (TEAMID)"
	Type        string // e.g. "Apple Development"
}

// DevIdentitySource explains how the dev signing identity was selected.
type DevIdentitySource string

const (
	SourceFlag         DevIdentitySource = "--sign"
	SourceDevConfig    DevIdentitySource = "dev.identity"
	SourceMacOSConfig  DevIdentitySource = "macos.signing_identity"
	SourceAutoDetected DevIdentitySource = "keychain"
	SourceAdHoc        DevIdentitySource = "ad-hoc"
)

// DevSigningRequest holds the signing identity inputs in priority order.
type DevSigningRequest struct {
	FlagIdentity  string // value of --sign
	FlagSet       bool   // whether --sign was explicitly given
	DevIdentity   string // dev.identity from ggui.toml
	MacOSIdentity string // macos.signing_identity from ggui.toml
	Entitlements  string // optional entitlements plist path
}

// DevSigning is the resolved signing configuration for ggui dev on macOS.
type DevSigning struct {
	// Target is what codesign --sign receives (SHA-1 fingerprint for a
	// keychain identity, or "-" for ad hoc).
	Target string
	// Display is a human-readable description for logs (such as the
	// keychain identity's name, or "ad hoc").
	Display      string
	Source       DevIdentitySource
	Entitlements string
	// Warnings holds non-fatal notices (for instance when
	// macos.signing_identity is absent from the local keychain and dev
	// falls back).
	Warnings []string
}

// AdHoc reports whether s signs ad hoc rather than with a keychain identity.
func (s DevSigning) AdHoc() bool { return s.Target == "" || s.Target == "-" }

// ResolveDevIdentity chooses the signing identity for ggui dev on macOS:
//  1. --sign flag / dev.identity
//  2. macos.signing_identity (when set and not "-")
//  3. First valid "Apple Development" identity in the keychain
//  4. Ad-hoc signing ("-")
func ResolveDevIdentity(ctx context.Context, req DevSigningRequest) (DevSigning, error) {
	return resolveDevIdentity(ctx, req, listKeychainIdentities)
}

func resolveDevIdentity(ctx context.Context, req DevSigningRequest, listFn func(context.Context) ([]KeychainIdentity, error)) (DevSigning, error) {
	var (
		ids      []KeychainIdentity
		listErr  error
		listed   bool
		warnings []string
	)
	load := func() ([]KeychainIdentity, error) {
		if !listed {
			listed = true
			if listFn != nil {
				ids, listErr = listFn(ctx)
			}
		}
		return ids, listErr
	}

	// 1a. Explicit --sign flag.
	if req.FlagSet {
		want := strings.TrimSpace(req.FlagIdentity)
		if want == "" {
			return DevSigning{}, errors.New(`--sign must name a signing identity, or "-" to sign ad hoc`)
		}
		if want == "-" {
			return DevSigning{Target: "-", Display: "ad hoc", Source: SourceFlag, Entitlements: req.Entitlements}, nil
		}
		all, err := load()
		if err != nil {
			return DevSigning{}, fmt.Errorf("looking up signing identity %q (from --sign): %w; pass --sign - to sign ad hoc", want, err)
		}
		id, ok := matchIdentity(all, want)
		if !ok {
			return DevSigning{}, fmt.Errorf("signing identity %q (from --sign) not found in keychain; run `security find-identity -v -p codesigning` to list identities, or pass --sign - to sign ad hoc", want)
		}
		return DevSigning{Target: identityTarget(id), Display: id.Description, Source: SourceFlag, Entitlements: req.Entitlements}, nil
	}

	// 1b. dev.identity in ggui.toml.
	if want := strings.TrimSpace(req.DevIdentity); want != "" {
		if want == "-" {
			return DevSigning{Target: "-", Display: "ad hoc", Source: SourceDevConfig, Entitlements: req.Entitlements}, nil
		}
		all, err := load()
		if err != nil {
			return DevSigning{}, fmt.Errorf("looking up signing identity %q (from dev.identity): %w; pass --sign - to sign ad hoc", want, err)
		}
		id, ok := matchIdentity(all, want)
		if !ok {
			return DevSigning{}, fmt.Errorf("signing identity %q (from dev.identity) not found in keychain; run `security find-identity -v -p codesigning` to list identities, or pass --sign - to sign ad hoc", want)
		}
		return DevSigning{Target: identityTarget(id), Display: id.Description, Source: SourceDevConfig, Entitlements: req.Entitlements}, nil
	}

	// 2. macos.signing_identity from ggui.toml (if set and not "-").
	// If missing on a developer's local Mac (e.g. a CI-only Developer ID),
	// warn and fall through to Apple Development or ad hoc.
	if want := strings.TrimSpace(req.MacOSIdentity); want != "" && want != "-" {
		all, err := load()
		if err == nil {
			if id, ok := matchIdentity(all, want); ok {
				return DevSigning{Target: identityTarget(id), Display: id.Description, Source: SourceMacOSConfig, Entitlements: req.Entitlements}, nil
			}
			warnings = append(warnings, fmt.Sprintf("macos.signing_identity %q is not in the keychain; trying Apple Development or ad hoc signing for dev", want))
		} else {
			warnings = append(warnings, fmt.Sprintf("could not check keychain for macos.signing_identity %q (%v); falling back for dev", want, err))
		}
	}

	// 3. Auto-detect a valid "Apple Development" identity in the keychain.
	if all, err := load(); err == nil {
		for _, id := range all {
			if isAppleDevelopment(id) {
				return DevSigning{
					Target:       identityTarget(id),
					Display:      id.Description,
					Source:       SourceAutoDetected,
					Entitlements: req.Entitlements,
					Warnings:     warnings,
				}, nil
			}
		}
	}

	// 4. Fallback to ad-hoc signing.
	return DevSigning{
		Target:       "-",
		Display:      "ad hoc",
		Source:       SourceAdHoc,
		Entitlements: req.Entitlements,
		Warnings:     warnings,
	}, nil
}

func isAppleDevelopment(id KeychainIdentity) bool {
	return id.Type == "Apple Development" || strings.HasPrefix(id.Description, "Apple Development")
}

func identityTarget(id KeychainIdentity) string {
	if id.Fingerprint != "" {
		return id.Fingerprint
	}
	return id.Description
}

// matchIdentity finds the best matching keychain identity for want:
// exact fingerprint or description first, then prefix, then substring.
func matchIdentity(ids []KeychainIdentity, want string) (KeychainIdentity, bool) {
	for _, id := range ids {
		if strings.EqualFold(id.Fingerprint, want) || id.Description == want {
			return id, true
		}
	}
	for _, id := range ids {
		if strings.HasPrefix(id.Description, want) {
			return id, true
		}
	}
	for _, id := range ids {
		if strings.Contains(id.Description, want) {
			return id, true
		}
	}
	return KeychainIdentity{}, false
}
