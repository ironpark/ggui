package proc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The test binary doubles as the program being run: with GGUI_PROC_HELPER
// set it plays one of the roles below instead of running tests.
func TestMain(m *testing.M) {
	switch role := os.Getenv("GGUI_PROC_HELPER"); role {
	case "":
		os.Exit(m.Run())
	case "graceful":
		// Exits cleanly when asked, as an app that saves first would.
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, os.Interrupt)
		fmt.Println("ready")
		<-c
		fmt.Println("bye")
		os.Exit(0)
	case "stubborn":
		// Takes the interrupt and goes on. Ignoring it would not do on
		// Windows, where a Ctrl-Break that no handler of the process takes
		// goes on to the default one, which exits.
		signal.Notify(make(chan os.Signal, 1), syscall.SIGTERM, os.Interrupt)
		fmt.Println("ready")
		time.Sleep(time.Hour)
	case "parent":
		// Starts a child of its own and reports its PID: the process a
		// careless stop would leave behind.
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), "GGUI_PROC_HELPER=sleep")
		if err := cmd.Start(); err != nil {
			fmt.Println("error", err)
			os.Exit(1)
		}
		fmt.Println("child", cmd.Process.Pid)
		time.Sleep(time.Hour)
	case "sleep":
		time.Sleep(time.Hour)
	case "exit3":
		os.Exit(3)
	default:
		fmt.Fprintln(os.Stderr, "unknown role", role)
		os.Exit(2)
	}
}

// start runs the test binary in role and returns it with its output.
func start(t *testing.T, role string) (*Process, *bufio.Reader) {
	t.Helper()
	pr, pw := io.Pipe()
	p, err := Start(os.Args[0], nil, Options{
		Env:    append(os.Environ(), "GGUI_PROC_HELPER="+role),
		Stdout: pw,
		Stderr: os.Stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		p.Kill()
		p.Stop(time.Second)
		pw.Close()
	})
	return p, bufio.NewReader(pr)
}

// line reads one line of output, failing the test if none comes.
func line(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	got := make(chan string, 1)
	go func() {
		s, _ := r.ReadString('\n')
		got <- strings.TrimSpace(s)
	}()
	select {
	case s := <-got:
		return s
	case <-time.After(10 * time.Second):
		t.Fatal("no output from the helper")
		return ""
	}
}

func TestStopGraceful(t *testing.T) {
	if runtime.GOOS == "windows" {
		// The request to exit is Ctrl-Break, which reaches only a process
		// sharing ggui's console, and a CI runner's tests may have none.
		t.Skip("needs a console")
	}
	p, out := start(t, "graceful")
	if s := line(t, out); s != "ready" {
		t.Fatalf("helper said %q", s)
	}
	go io.Copy(io.Discard, out)
	start := time.Now()
	if err := p.Stop(5 * time.Second); err != nil {
		t.Fatalf("Stop = %v, want a graceful exit", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("a graceful stop took %s", d)
	}
	if p.Err() != nil {
		t.Errorf("exit = %v, want status 0", p.Err())
	}
}

func TestStopKillsAfterTimeout(t *testing.T) {
	p, out := start(t, "stubborn")
	if s := line(t, out); s != "ready" {
		t.Fatalf("helper said %q", s)
	}
	if err := p.Stop(200 * time.Millisecond); !errors.Is(err, ErrKilled) {
		t.Fatalf("Stop = %v, want ErrKilled", err)
	}
	select {
	case <-p.Done():
	default:
		t.Fatal("Stop returned before the process exited")
	}
}

func TestKillSkipsTheWait(t *testing.T) {
	p, out := start(t, "stubborn")
	line(t, out)
	go func() {
		time.Sleep(100 * time.Millisecond)
		p.Kill()
	}()
	start := time.Now()
	p.Stop(time.Minute)
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("Kill did not cut the stop short: %s", d)
	}
}

func TestExitOnItsOwn(t *testing.T) {
	p, _ := start(t, "exit3")
	select {
	case <-p.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("the helper did not exit")
	}
	var ee *exec.ExitError
	if !errors.As(p.Err(), &ee) || ee.ExitCode() != 3 {
		t.Errorf("Err = %v, want exit status 3", p.Err())
	}
	if err := p.Stop(time.Second); err != nil {
		t.Errorf("Stop after exit = %v", err)
	}
}
