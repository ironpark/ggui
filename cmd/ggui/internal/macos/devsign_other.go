//go:build !darwin

package macos

import (
	"context"
	"errors"

	"github.com/ironpark/ggui/cmd/ggui/internal/machosign"
)

func listKeychainIdentities(context.Context) ([]KeychainIdentity, error) {
	return nil, nil
}

func signDevPath(_ context.Context, ds DevSigning, path string, isBundle bool, identifier string) error {
	if !ds.AdHoc() {
		return errors.New("keychain code signing is only available on macOS")
	}
	if isBundle {
		return signAdHoc(path, ds.Entitlements)
	}
	if ds.Entitlements == "" && identifier == "" {
		return nil
	}
	ents, err := readEntitlements(ds.Entitlements)
	if err != nil {
		return err
	}
	return machosign.SignFile(path, machosign.Options{
		Identifier:   identifier,
		Entitlements: ents,
	})
}
