package ggui

import (
	"path/filepath"
	"testing"
)

// setAppID sets the id ggui build would link in and the id SingleInstance
// would record, until t ends. A test that calls it must not be parallel:
// parallel tests read both.
func setAppID(t *testing.T, build, instance string) {
	t.Helper()
	oldBuild, oldInstance := buildID, instanceName.Load()
	t.Cleanup(func() {
		buildID = oldBuild
		instanceName.Store(oldInstance)
	})
	buildID = build
	if instance == "" {
		instanceName.Store(nil)
	} else {
		instanceName.Store(&instance)
	}
}

func TestChooseAppID(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		bundle, build, instance, exe string
		want                         string
		src                          idSource
	}{
		{"com.example.bundle", "com.example.build", "com.example.instance", "/x/app", "com.example.bundle", idBundle},
		{"", "com.example.build", "com.example.instance", "/x/app", "com.example.build", idBuild},
		{"", "", "com.example.instance", "/x/app", "com.example.instance", idInstance},
		{"", "", "", "/usr/local/bin/hello", "hello", idExecutable},
		{"", "", "", "/x/hello.exe", "hello", idExecutable},
		{"", "", "", "", "ggui", idExecutable},
		{"", "", "", "/x/.hidden", "ggui", idExecutable},
	} {
		exe := filepath.FromSlash(c.exe)
		id, src := chooseAppID(c.bundle, c.build, c.instance, exe)
		if id != c.want || src != c.src {
			t.Errorf("chooseAppID(%q, %q, %q, %q) = %q, %d; want %q, %d",
				c.bundle, c.build, c.instance, exe, id, src, c.want, c.src)
		}
	}
}

// TestAppIDOrder is not parallel: it sets the app's id.
func TestAppIDOrder(t *testing.T) {
	if bundleID() != "" {
		t.Skip("the test runs from a bundle, whose identifier names it")
	}
	setAppID(t, "", "")
	exeID := AppID()
	if exeID == "" || exeID == "com.example.instance" {
		t.Fatalf("AppID() = %q without an id; want the executable's name", exeID)
	}
	setAppID(t, "", "com.example.instance")
	if got := AppID(); got != "com.example.instance" {
		t.Errorf("AppID() = %q; want SingleInstance's id", got)
	}
	setAppID(t, "com.example.build", "com.example.instance")
	if got := AppID(); got != "com.example.build" {
		t.Errorf("AppID() = %q; want the linked id before SingleInstance's", got)
	}
	setAppID(t, "com.example.build", "")
	if got := AppID(); got != "com.example.build" {
		t.Errorf("AppID() = %q; want the linked id", got)
	}
}

// TestPlacementsAreNamedForTheLinkedID is not parallel: it sets the app's
// id.
func TestPlacementsAreNamedForTheLinkedID(t *testing.T) {
	if bundleID() != "" {
		t.Skip("the test runs from a bundle, whose identifier names it")
	}
	a := New(Config{Title: "Notes"}, func() Widget { return sized(0, 0) })
	defer a.Close()
	setAppID(t, "com.example.notes", "")
	if got := a.identity(); got != "com.example.notes" {
		t.Errorf("identity %q; want the linked id", got)
	}
	setAppID(t, "", "")
	if got := a.identity(); got != "Notes" {
		t.Errorf("identity %q; want the title without an id", got)
	}
}

func TestRunOptions(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		id        string
		src       idSource
		wantClass string
	}{
		{"com.example.bundle", idBundle, "com.example.bundle"},
		{"com.example.build", idBuild, "com.example.build"},
		{"com.example.instance", idInstance, "com.example.instance"},
		{"hello", idExecutable, ""},
	} {
		o := runOptions(c.id, c.src)
		if !o.MacOS.PressAndHold {
			t.Errorf("runOptions(%q, %d) turns press and hold off", c.id, c.src)
		}
		if o.X11.ClassName != c.wantClass {
			t.Errorf("runOptions(%q, %d).X11.ClassName = %q; want %q", c.id, c.src, o.X11.ClassName, c.wantClass)
		}
		if o.X11.InstanceName != "" {
			t.Errorf("runOptions(%q, %d).X11.InstanceName = %q; want ggwd's default", c.id, c.src, o.X11.InstanceName)
		}
	}
}
