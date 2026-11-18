//go:build !darwin && cgo && (amd64 || arm64) && (linux || (windows && ggui_rcodesign))

package macos

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/zapp/pkg/signing/rcodesign"
)

// A build with rcodesign signs a Mach-O with a certificate, here a
// throwaway self-signed one in PEM, given as macos.p12_file takes it. It
// reaches Apple's timestamp service, so it is skipped with -short.
func TestRcodesignSigns(t *testing.T) {
	if testing.Short() {
		t.Skip("signing asks Apple's timestamp service for a timestamp")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "hello")
	src := filepath.Join(dir, "main.go")
	if err := os.WriteFile(src, []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", bin, src)
	build.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0", "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building a Mach-O: %v\n%s", err, out)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Developer ID Application: ggui test (GGUITEST01)", OrganizationalUnit: []string{"GGUITEST01"}},
		NotBefore:    time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cert := filepath.Join(dir, "cert.pem")
	data := append(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	if err := os.WriteFile(cert, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// signing_identity names a keychain identity, as ggui.toml may for a
	// Mac; with a certificate it is not one here, and the certificate signs.
	sg, err := NewSigner(Signing{Identity: "Developer ID Application", P12File: cert, P12Source: cert}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := sg.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := sg.Check(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sg.Identity, cert) {
		t.Errorf("identity = %q, want the certificate %s", sg.Identity, cert)
	}
	if signed, err := sg.Sign(t.Context(), bin); !signed || err != nil {
		t.Fatalf("signed = %v, %v", signed, err)
	}
	if err := rcodesign.Verify(t.Context(), bin); err != nil {
		t.Errorf("the signature does not verify: %v", err)
	}
	if err := sg.CheckInstaller(t.Context()); err == nil || !strings.Contains(err.Error(), "on a Mac only") {
		t.Errorf("an installer away from a Mac: %v", err)
	}
}
