package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
)

func TestResolveBundleDir(t *testing.T) {
	root := t.TempDir()
	proj := &project.Project{Root: root, Config: config.Defaults()}
	outDir := proj.Path(proj.Config.Build.Output)

	// Default is build.output.
	got, err := resolveBundleDir(proj, outDir, "", false)
	if err != nil || got != outDir {
		t.Fatalf("default = %q, %v; want %q", got, err, outDir)
	}

	// dev.bundle_dir resolves relative to project root.
	proj.Config.Dev.BundleDir = "dev-out"
	got, err = resolveBundleDir(proj, outDir, "", false)
	want := filepath.Join(root, "dev-out")
	if err != nil || got != want {
		t.Fatalf("dev.bundle_dir = %q, %v; want %q", got, err, want)
	}

	// --bundle-dir flag takes precedence over dev.bundle_dir.
	flagDir := filepath.Join(t.TempDir(), "custom")
	got, err = resolveBundleDir(proj, outDir, flagDir, true)
	if err != nil || got != flagDir {
		t.Fatalf("--bundle-dir = %q, %v; want %q", got, err, flagDir)
	}
}

func TestDevHelpMentionsBundleAndMenuBar(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"help", "dev"}, &out, &errOut); code != 0 {
		t.Fatalf("help dev exited %d: %s", code, errOut.String())
	}
	help := out.String()
	for _, want := range []string{"--bundle-dir", "--sign", "Name.dev.app", "dev.bundle_dir", "dev.identity"} {
		if !strings.Contains(help, want) {
			t.Errorf("ggui help dev lacks %q", want)
		}
	}
	if strings.Contains(help, "no tray icon") {
		t.Error("ggui help dev still claims tray icon is unsupported in remote mode")
	}
}
