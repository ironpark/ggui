package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/installer"
)

func TestInstallerExampleFlow(t *testing.T) {
	pb := installer.NewPreviewBackend()
	defer pb.Close()

	ctx, err := installer.NewContext(newInstallerApp(), nil, pb)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	var probe *ggui.Probe
	probe = ggui.ProbeBuilder(func() ggui.Widget {
		return ctx.BuildWidget(probe.Post, probe.Dialogs(), func() { closed = true })
	}, ggui.Sz(560, 420))
	defer probe.Close()

	click := func(role ggui.Role, label string) {
		t.Helper()
		f, ok := probe.FindRole(role, label)
		if !ok {
			t.Fatalf("could not find %s %q", role, label)
		}
		probe.Click(f.Center())
		probe.Frame()
	}

	probe.Frame()
	// 1. Welcome -> Next
	click(ggui.RoleButton, "Next")
	// 2. License -> accept -> Next
	click(ggui.RoleCheckbox, "I accept the terms in the License Agreement")
	click(ggui.RoleButton, "Next")
	// 3. Location -> Next
	click(ggui.RoleButton, "Next")
	// 4. Custom Options -> Install
	click(ggui.RoleButton, "Install")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		probe.Frame()
		if cur, _ := ctx.CurrentStep(); cur.Kind == installer.StepFinish {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if cur, _ := ctx.CurrentStep(); cur.Kind != installer.StepFinish {
		t.Fatalf("did not reach finish step, current=%q err=%q", cur.ID, ggui.Peek(ctx.ErrText))
	}

	dir := ggui.Peek(ctx.InstallDir)
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Errorf("missing config.json: %v", err)
	}
	if !slices.Contains(pb.PathDirs(installer.ScopeUser), dir) {
		t.Errorf("PATH missing %s: %v", dir, pb.PathDirs(installer.ScopeUser))
	}
	if v, ok, _ := pb.GetEnv(installer.ScopeUser, "ACME_CHANNEL"); !ok || v != "stable" {
		t.Errorf("ACME_CHANNEL = (%q, %v), want stable", v, ok)
	}
	if rv, ok, _ := pb.GetRegistryValue(installer.RegistryHKCU, `Software\Classes\.acme`, ""); !ok || rv.String != "AcmeStudio.Document" {
		t.Errorf(".acme registry = (%+v, %v)", rv, ok)
	}

	// 5. Finish closes window.
	click(ggui.RoleButton, "Finish")
	if !closed {
		t.Error("Finish did not close window")
	}
}
