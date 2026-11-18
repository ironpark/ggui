package ggui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoginID is not parallel: it sets the app's id, which parallel tests
// read.
func TestLoginID(t *testing.T) {
	if bundleID() != "" {
		t.Skip("the test runs from a bundle, whose identifier names it")
	}
	for _, c := range []struct{ build, instance, exe, want string }{
		{"com.example.app", "com.example.other", "/x/App", "com.example.app"},
		{"", "com.example.editor", "/x/editor", "com.example.editor"},
		{"", "", "/usr/local/bin/hello", "hello"},
		{"", "", `C:\Apps\Hello World.exe`, "Hello World"},
		{"", "a/b:c", "/x/y", "a_b_c"},
	} {
		setAppID(t, c.build, c.instance)
		exe := c.exe
		if strings.Contains(exe, `\`) && filepath.Separator == '/' {
			exe = strings.ReplaceAll(exe, `\`, "/")
		}
		if got := loginID(exe); got != c.want {
			t.Errorf("loginID(%q) with build id %q, instance %q = %q, want %q", exe, c.build, c.instance, got, c.want)
		}
	}
}

func TestLaunchAgent(t *testing.T) {
	t.Parallel()
	got := string(launchAgent("com.example.a&b", "/Applications/A & B.app/Contents/MacOS/A <B>"))
	for _, want := range []string{
		"<key>Label</key>\n\t<string>com.example.a&amp;b</string>",
		"<array>\n\t\t<string>/Applications/A &amp; B.app/Contents/MacOS/A &lt;B&gt;</string>\n\t</array>",
		"<key>RunAtLoad</key>\n\t<true/>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist lacks %q:\n%s", want, got)
		}
	}
	if p := launchAgentPath("/Users/me/Library/LaunchAgents", "com.example.app"); p != filepath.Join("/Users/me/Library/LaunchAgents", "com.example.app.plist") {
		t.Errorf("path = %q", p)
	}
}

func TestAutostartEntry(t *testing.T) {
	t.Parallel()
	got := string(autostartEntry("My App", `/opt/my app/100%$run"x\y`))
	want := "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=My App\n" +
		`Exec="/opt/my app/100%%\\$run\\"x\\\\y"` + "\n" +
		"X-GNOME-Autostart-enabled=true\n"
	if got != want {
		t.Errorf("entry =\n%s\nwant\n%s", got, want)
	}
	if p := autostartPath("/home/me/.config/autostart", "hello"); p != filepath.Join("/home/me/.config/autostart", "hello.desktop") {
		t.Errorf("path = %q", p)
	}
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	if d := autostartDir(getenv, "/home/me"); d != filepath.Join("/home/me", ".config", "autostart") {
		t.Errorf("default dir = %q", d)
	}
	env["XDG_CONFIG_HOME"] = filepath.Join(t.TempDir(), "cfg")
	if d := autostartDir(getenv, "/home/me"); d != filepath.Join(env["XDG_CONFIG_HOME"], "autostart") {
		t.Errorf("XDG dir = %q", d)
	}
	env["XDG_CONFIG_HOME"] = "relative"
	if d := autostartDir(getenv, "/home/me"); d != filepath.Join("/home/me", ".config", "autostart") {
		t.Errorf("relative XDG_CONFIG_HOME was used: %q", d)
	}
}

func TestRunCommandQuotes(t *testing.T) {
	t.Parallel()
	if got := runCommand(`C:\Program Files\App\app.exe`); got != `"C:\Program Files\App\app.exe"` {
		t.Errorf("got %s", got)
	}
}

func TestLoginFileRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "LaunchAgents", "com.example.app.plist")
	if on, err := loginFileExists(path); err != nil || on {
		t.Fatalf("before = %v, %v", on, err)
	}
	data := launchAgent("com.example.app", "/x/app")
	if err := setLoginFile(path, data, true); err != nil {
		t.Fatal(err)
	}
	if err := setLoginFile(path, data, true); err != nil {
		t.Fatalf("again: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
		t.Fatalf("written = %q, %v", got, err)
	}
	if on, err := loginFileExists(path); err != nil || !on {
		t.Fatalf("after = %v, %v", on, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("directory holds %v, %v; want the one file", entries, err)
	}
	for range 2 {
		if err := setLoginFile(path, nil, false); err != nil {
			t.Fatal(err)
		}
	}
	if on, err := loginFileExists(path); err != nil || on {
		t.Fatalf("removed = %v, %v", on, err)
	}
}
