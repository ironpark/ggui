package homebrew

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/release"
	"github.com/ironpark/ggui/cmd/ggui/internal/release/github"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const dl = "https://github.com/alice/hello/releases/download/v1.2.0/"

func casks() map[string]Cask {
	base := Cask{
		Token: "hello-world", Name: "Hello World", Version: "1.2.0",
		Desc: "A friendly greeter.", Homepage: "https://github.com/alice/hello",
		ID: "com.example.hello", MinMacOS: "13.0",
	}
	universal := base
	universal.Universal = &File{URL: dl + "hello-world-1.2.0-macos-universal.dmg", SHA256: strings.Repeat("a", 64)}
	perArch := base
	perArch.Homepage = "https://hello.example.com"
	perArch.MinMacOS = "14"
	perArch.Arm = &File{URL: dl + "hello-world-1.2.0-macos-arm64.zip", SHA256: strings.Repeat("b", 64)}
	perArch.Intel = &File{URL: dl + "hello-world-1.2.0-macos-amd64.zip", SHA256: strings.Repeat("c", 64)}
	armOnly := base
	armOnly.Desc = ""
	armOnly.ID = ""
	armOnly.Arm = perArch.Arm
	// Downloads that differ by more than the architecture get a block
	// each.
	blocks := perArch
	blocks.Arm = &File{URL: dl + "hello-world-1.2.0-macos-arm64.dmg", SHA256: strings.Repeat("d", 64)}
	return map[string]Cask{"universal": universal, "per-arch": perArch, "arm-only": armOnly, "on-blocks": blocks}
}

// The casks are compared with golden files, written by -update; check a
// change with brew style, as TestBrewStyle does when asked.
func TestCaskGolden(t *testing.T) {
	for name, c := range casks() {
		t.Run(name, func(t *testing.T) {
			got, err := c.Ruby()
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", name+".rb")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("cask differs from %s:\n%s", golden, got)
			}
		})
	}
	if _, err := (Cask{Token: "x"}).Ruby(); err == nil {
		t.Error("a cask without a download was written")
	}
}

func TestRubyEscaping(t *testing.T) {
	c := casks()["universal"]
	c.Name = `Say "Hi" #{system("x")}`
	got, err := c.Ruby()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `name "Say \"Hi\" \#{system(\"x\")}"`) {
		t.Errorf("name not escaped:\n%s", got)
	}
}

func TestToken(t *testing.T) {
	for in, want := range map[string]string{
		"Hello World":     "hello-world",
		"Hello World.app": "hello-world",
		"C++ Studio":      "c-plus-plus-studio",
		"Fish & Chips":    "fish-and-chips",
		"app@2":           "app-at-2",
		"  ggui  ":        "ggui",
	} {
		if got := Token(in); got != want {
			t.Errorf("Token(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMacOSName(t *testing.T) {
	for in, want := range map[string]string{"13.0": "ventura", "14": "sonoma", "15.2": "sequoia", "26.0": "tahoe", "10.15": "catalina", "11.0": "big_sur", "99": ""} {
		if got := MacOSName(in); got != want {
			t.Errorf("MacOSName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPublish(t *testing.T) {
	var log []string
	var put map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log = append(log, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer tap-token" {
			w.WriteHeader(401)
			return
		}
		switch r.Method {
		case "GET":
			w.Write([]byte(`{"sha":"oldsha","name":"hello-world.rb"}`))
		case "PUT":
			if err := json.NewDecoder(r.Body).Decode(&put); err != nil {
				t.Error(err)
			}
			w.WriteHeader(200)
			w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	c := github.New(srv.URL, release.NewToken("tap-token", "$HOMEBREW_TAP_TOKEN"))
	if err := Publish(t.Context(), c, "alice/homebrew-tap", "hello-world", "1.2.0", []byte("cask \"hello-world\" do\nend\n")); err != nil {
		t.Fatal(err)
	}
	want := []string{"GET /repos/alice/homebrew-tap/contents/Casks/hello-world.rb", "PUT /repos/alice/homebrew-tap/contents/Casks/hello-world.rb"}
	if strings.Join(log, "\n") != strings.Join(want, "\n") {
		t.Errorf("requests %v", log)
	}
	content, _ := base64.StdEncoding.DecodeString(put["content"])
	if put["message"] != "Update hello-world to 1.2.0" || put["sha"] != "oldsha" || !strings.HasPrefix(string(content), `cask "hello-world"`) {
		t.Errorf("PUT %v", put)
	}
}

// TestBrewStyle runs brew style on the golden casks when
// GGUI_BREW_STYLE=1, in a tap's layout. It is not run by default: brew
// style turns Homebrew's developer mode on, and installs its own gems.
func TestBrewStyle(t *testing.T) {
	if os.Getenv("GGUI_BREW_STYLE") != "1" {
		t.Skip("set GGUI_BREW_STYLE=1 to run brew style on the casks")
	}
	brew, err := exec.LookPath("brew")
	if err != nil {
		t.Skip("brew is not installed")
	}
	tap := filepath.Join(t.TempDir(), "homebrew-test", "Casks")
	if err := os.MkdirAll(tap, 0o755); err != nil {
		t.Fatal(err)
	}
	var files []string
	for name, c := range casks() {
		ruby, err := c.Ruby()
		if err != nil {
			t.Fatal(err)
		}
		c.Token = "hello-" + name
		f := filepath.Join(tap, c.Token+".rb")
		if err := os.WriteFile(f, []byte(strings.Replace(ruby, `cask "hello-world"`, `cask "`+c.Token+`"`, 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
	}
	cmd := exec.Command(brew, append([]string{"style", "--cask"}, files...)...)
	cmd.Env = append(os.Environ(), "HOMEBREW_NO_AUTO_UPDATE=1")
	out, err := cmd.CombinedOutput()
	t.Logf("brew style:\n%s", out)
	if err != nil {
		t.Errorf("brew style: %v", err)
	}
}
