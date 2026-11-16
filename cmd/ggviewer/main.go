//go:build (unix && !ios && !android) || windows

package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ironpark/ggwd"
	"github.com/ironpark/ggwd/remote"
)

// maxSocketPath is the longest path a unix socket can have on every
// platform the viewer runs on: sun_path is 104 bytes on macOS and the
// BSDs, with its terminating 0.
const maxSocketPath = 103

type options struct {
	socket  string
	project string
	debug   bool
	// windowFramesOnly replays in window frames alone, as the viewer once
	// did, so that the stall of an app whose viewer's window is hidden can
	// be shown. For -debug runs.
	windowFramesOnly bool
}

func main() {
	var o options
	flag.StringVar(&o.socket, "socket", "", "the unix socket to listen on, which apps find in "+remote.EnvSocket)
	flag.StringVar(&o.project, "project", "", "the project the window is remembered for, such as its directory; none remembers nothing")
	flag.BoolVar(&o.debug, "debug", false, "accept the debug commands on the control channel")
	flag.BoolVar(&o.windowFramesOnly, "debug-window-frames-only", false, "replay in window frames only, which stalls an app while the window is hidden (with -debug)")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: ggviewer -socket path [-project key]\n\n")
		fmt.Fprintf(flag.CommandLine.Output(), "ggviewer shows the window of ggui apps built with -tags ggwd_remote, and keeps it while they restart.\nIts stdin and stdout are the control channel of its launcher; it exits when its stdin closes.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	log.SetFlags(0)
	log.SetPrefix("ggviewer: ")
	if flag.NArg() > 0 {
		flag.Usage()
		os.Exit(2)
	}
	if o.socket == "" {
		log.Print("-socket is required")
		flag.Usage()
		os.Exit(2)
	}
	if o.windowFramesOnly && !o.debug {
		log.Fatal("-debug-window-frames-only needs -debug")
	}
	if err := run(o); err != nil {
		log.Fatal(err)
	}
}

// listen listens at path, which a viewer that went away without removing
// it may have left: such a socket, that nothing answers, is replaced.
func listen(path string) (*net.UnixListener, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if len(abs) > maxSocketPath {
		return nil, fmt.Errorf("the socket path %s is %d bytes; a unix socket's is at most %d", abs, len(abs), maxSocketPath)
	}
	if _, err := os.Lstat(abs); err == nil {
		if c, err := net.DialTimeout("unix", abs, time.Second); err == nil {
			_ = c.Close()
			return nil, fmt.Errorf("another viewer listens at %s", abs)
		}
		if err := os.Remove(abs); err != nil {
			return nil, err
		}
	}
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: abs, Net: "unix"})
	if err != nil {
		return nil, err
	}
	// The viewer removes the file itself, once, when it ends.
	ln.SetUnlinkOnClose(false)
	return ln, nil
}

func run(o options) error {
	// A launcher that is gone makes writes to stdout fail rather than end
	// the viewer before it cleans up.
	signal.Ignore(syscall.SIGPIPE)

	ln, err := listen(o.socket)
	if err != nil {
		return err
	}
	socket := ln.Addr().String()
	defer func() {
		_ = ln.Close()
		_ = os.Remove(socket)
	}()

	v := newSession(o, &events{w: os.Stdout})
	if o.project != "" {
		if dir, err := os.UserCacheDir(); err == nil {
			v.statePath = statePath(dir, o.project)
			v.saved, v.haveSaved = loadState(v.statePath, o.project)
		} else {
			log.Printf("remembering no window: %v", err)
		}
	}
	defer v.detachAll()

	conns := make(chan *net.UnixConn)
	go func() {
		for {
			c, err := ln.AcceptUnix()
			if err != nil {
				close(conns)
				return
			}
			conns <- c
		}
	}()
	go v.readControl(os.Stdin)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigs
		v.quit()
	}()

	v.events.send(event{Type: evReady, Socket: socket})

	// The window opens with the first app, whose options it takes, or when
	// the launcher has a failure to show before any app came.
	var first *net.UnixConn
	select {
	case first = <-conns:
	case <-v.needWindow:
	case <-v.quitting:
		return nil
	}
	v.first = first
	v.conns = conns
	err = ggwd.Run(v, nil)
	v.saveState()
	if errors.Is(err, ggwd.Termination) {
		err = nil
	}
	return err
}

// readControl reads the launcher's commands until r ends, and then ends
// the viewer.
func (v *session) readControl(r *os.File) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 64<<20)
	for sc.Scan() {
		if len(sc.Bytes()) == 0 {
			continue
		}
		c, err := parseCommand(sc.Bytes(), v.opts.debug)
		if err != nil {
			log.Print(err)
			continue
		}
		v.command(c)
	}
	if err := sc.Err(); err != nil {
		log.Printf("reading the control channel: %v", err)
	}
	v.quit()
}
