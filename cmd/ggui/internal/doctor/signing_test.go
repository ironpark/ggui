package doctor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
)

func TestCheckCertificate(t *testing.T) {
	m := config.Defaults().MacOS
	r := &Report{}
	checkCertificate(r, m, nil)
	if len(r.Checks) != 0 {
		t.Errorf("reported a certificate when there is none: %v", r.Checks)
	}
	t.Setenv(m.P12Base64Env, "AAAA")
	t.Setenv(m.P12PasswordEnv, "")
	checkCertificate(r, m, nil)
	if len(r.Checks) != 1 || r.Checks[0].Status != Warn || strings.Contains(r.Checks[0].Detail, "AAAA") {
		t.Errorf("certificate without a password: %v", r.Checks)
	}
	m.P12File = filepath.Join(t.TempDir(), "missing.p12")
	r = &Report{}
	checkCertificate(r, m, nil)
	if len(r.Checks) != 1 || r.Checks[0].Status != Fail {
		t.Errorf("missing certificate file: %v", r.Checks)
	}
}
