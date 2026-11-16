//go:build unix

package proc

import (
	"errors"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// gone reports whether pid has exited, waiting up to within for it to. A
// child killed and reparented is reaped by init soon after, so this polls.
func gone(pid int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// Stopping the app stops what it started: the group goes, not only its
// leader.
func TestStopLeavesNoOrphans(t *testing.T) {
	p, out := start(t, "parent")
	s := line(t, out)
	pidText, ok := strings.CutPrefix(s, "child ")
	if !ok {
		t.Fatalf("helper said %q", s)
	}
	child, err := strconv.Atoi(pidText)
	if err != nil {
		t.Fatal(err)
	}
	if gone(child, 0) {
		t.Fatal("the grandchild is not running to begin with")
	}
	p.Stop(2 * time.Second)
	if !gone(child, 5*time.Second) {
		syscall.Kill(child, syscall.SIGKILL)
		t.Fatalf("the grandchild %d outlived the stop", child)
	}
}

// The child is in its own process group, so a Ctrl-C at the terminal,
// which signals ggui's group, does not reach it.
func TestChildHasItsOwnGroup(t *testing.T) {
	p, out := start(t, "graceful")
	line(t, out)
	pgid, err := syscall.Getpgid(p.Pid())
	if err != nil {
		t.Fatal(err)
	}
	if pgid != p.Pid() {
		t.Errorf("child's group = %d, want its own, %d", pgid, p.Pid())
	}
	if pgid == syscall.Getpgrp() {
		t.Error("child shares ggui's process group")
	}
}
