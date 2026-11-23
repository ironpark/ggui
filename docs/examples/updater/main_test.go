package main

import (
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/updater"
)

func waitUntil(t *testing.T, probe *ggui.Probe, timeout time.Duration, desc string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		probe.Advance(30 * time.Millisecond)
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", desc)
}

func clickButton(t *testing.T, probe *ggui.Probe, label string) {
	t.Helper()
	probe.Advance(300 * time.Millisecond)
	f, ok := probe.FindRole(ggui.RoleButton, label)
	if !ok {
		t.Fatalf("button %q not found; semantics:\n%s", label, probe.Semantics())
	}
	probe.Click(f.Center())
	probe.Frame()
}

func TestUpdaterExampleFlow(t *testing.T) {
	demo, err := newDemoApp(0)
	if err != nil {
		t.Fatalf("newDemoApp: %v", err)
	}
	defer demo.Close()

	probe := ggui.ProbeBuilder(demo.buildUI, ggui.Sz(720, 600))
	defer probe.Close()

	probe.Frame()
	if got := ggui.Peek(demo.currentVersion); got != "1.0.0" {
		t.Fatalf("initial version = %q; want 1.0.0", got)
	}

	// 1. Click "Check for Updates…" -> sees stable v2.0.0 release notes.
	clickButton(t, probe, "Check for Updates…")
	waitUntil(t, probe, 3*time.Second, "v2.0.0 available", func() bool {
		c := ggui.Peek(demo.ctrl)
		return c != nil && ggui.Peek(c.Phase) == updater.PhaseAvailable
	})
	probe.Advance(300 * time.Millisecond)
	if sem := probe.Semantics().String(); !strings.Contains(sem, "Acme Notes 2.0.0") {
		t.Errorf("dialog missing 2.0.0 release notes:\n%s", sem)
	}

	// 2. Skip -> clear skipped -> simulate background check opens v2.0.0 again.
	clickButton(t, probe, "Skip This Version")
	clickButton(t, probe, "Clear Skipped Version")
	clickButton(t, probe, "Simulate Background Check")
	waitUntil(t, probe, 3*time.Second, "background check opens v2.0.0", func() bool {
		c := ggui.Peek(demo.ctrl)
		return c != nil && ggui.Peek(c.Open) && ggui.Peek(c.Phase) == updater.PhaseAvailable
	})

	// 3. Click "Install Update" -> downloads, verifies Ed25519 signature -> Ready to Install.
	clickButton(t, probe, "Install Update")
	waitUntil(t, probe, 3*time.Second, "PhaseReadyToInstall", func() bool {
		c := ggui.Peek(demo.ctrl)
		return c != nil && ggui.Peek(c.Phase) == updater.PhaseReadyToInstall
	})

	// 4. Click "Install and Relaunch" -> simulated applier bumps version to 2.0.0 live.
	clickButton(t, probe, "Install and Relaunch")
	waitUntil(t, probe, 3*time.Second, "live version bumped to 2.0.0", func() bool {
		return ggui.Peek(demo.currentVersion) == "2.0.0"
	})

	logText := strings.Join(ggui.Peek(demo.logs), "\n")
	if !strings.Contains(logText, "Verified Ed25519 signature (verified=true)") {
		t.Errorf("activity log missing Ed25519 verification entry:\n%s", logText)
	}

	// 5. Checking again on stable channel reports up to date.
	clickButton(t, probe, "Check for Updates…")
	waitUntil(t, probe, 3*time.Second, "PhaseUpToDate", func() bool {
		c := ggui.Peek(demo.ctrl)
		return c != nil && ggui.Peek(c.Phase) == updater.PhaseUpToDate
	})
	clickButton(t, probe, "OK")

	// 6. Switch to beta channel -> discovers v2.1.0-beta.1.
	demo.channel.Set("beta")
	demo.onChannelChange("beta")
	probe.Frame()

	clickButton(t, probe, "Check for Updates…")
	waitUntil(t, probe, 3*time.Second, "beta 2.1.0-beta.1 available", func() bool {
		c := ggui.Peek(demo.ctrl)
		if c == nil || ggui.Peek(c.Phase) != updater.PhaseAvailable {
			return false
		}
		rel := ggui.Peek(c.Release)
		return rel != nil && rel.DisplayVersion() == "2.1.0-beta.1"
	})
}
