package watch

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

// On Linux the source is inotify, which watches one directory at a time:
// each directory the watcher names, and each directory created or moved
// into one of them, so that a new directory's files are reported from the
// start. The descriptor is non-blocking and read through the runtime's
// poller, so closing it ends the reader.

const inotifyMask = syscall.IN_CREATE | syscall.IN_DELETE | syscall.IN_MODIFY | syscall.IN_CLOSE_WRITE |
	syscall.IN_ATTRIB | syscall.IN_MOVED_FROM | syscall.IN_MOVED_TO |
	syscall.IN_DELETE_SELF | syscall.IN_MOVE_SELF | syscall.IN_ONLYDIR

// errWatchLimit is inotify running out of watches.
var errWatchLimit = errors.New("the inotify watch limit is reached (raise fs.inotify.max_user_watches)")

type inotify struct {
	queue
	// fd is f's descriptor, kept since f.Fd would make f blocking, and
	// Close would then no longer end a Read.
	fd   int
	f    *os.File
	skip func(path string) bool

	mu    sync.Mutex
	wds   map[int32]string // a watch's directory
	dirs  map[string]int32 // a directory's watch
	asked map[string]bool  // the directories watch named
}

func newSource(skip func(string) bool) (source, error) {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err != nil {
		return nil, fmt.Errorf("inotify: %w", err)
	}
	s := &inotify{
		queue: newQueue(),
		fd:    fd,
		f:     os.NewFile(uintptr(fd), "inotify"),
		skip:  skip,
		wds:   map[int32]string{},
		dirs:  map[string]int32{},
		asked: map[string]bool{},
	}
	go s.read()
	return s, nil
}

func (s *inotify) watch(dirs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	asked := make(map[string]bool, len(dirs))
	for _, d := range dirs {
		asked[filepath.Clean(d)] = true
	}
	// A directory no longer asked for goes, unless it was added for
	// being created below one that still is.
	for d, wd := range s.dirs {
		if asked[d] {
			continue
		}
		if !s.asked[d] && s.below(asked, d) {
			continue
		}
		syscall.InotifyRmWatch(s.fd, uint32(wd))
		delete(s.dirs, d)
		delete(s.wds, wd)
	}
	s.asked = asked
	for d := range asked {
		if err := s.add(d); err != nil {
			return err
		}
	}
	return nil
}

// below reports whether dir is below one of dirs.
func (s *inotify) below(dirs map[string]bool, dir string) bool {
	for d := filepath.Dir(dir); ; d = filepath.Dir(d) {
		if dirs[d] {
			return true
		}
		if d == filepath.Dir(d) {
			return false
		}
	}
}

// add watches dir; a directory that does not exist is not an error.
// s.mu is held.
func (s *inotify) add(dir string) error {
	if _, ok := s.dirs[dir]; ok {
		return nil
	}
	wd, err := syscall.InotifyAddWatch(s.fd, dir, inotifyMask)
	switch {
	case errors.Is(err, syscall.ENOSPC):
		return errWatchLimit
	case errors.Is(err, syscall.ENOENT), errors.Is(err, syscall.ENOTDIR), errors.Is(err, syscall.EACCES):
		return nil
	case err != nil:
		return fmt.Errorf("inotify: %s: %w", dir, err)
	}
	// The same directory by another name, through a link, has the same
	// watch.
	if old, ok := s.wds[int32(wd)]; ok {
		delete(s.dirs, old)
	}
	s.wds[int32(wd)] = dir
	s.dirs[dir] = int32(wd)
	return nil
}

// addTree watches a directory that appeared, and those below it, and
// reports what is in them already: files made before the watch was in
// place.
func (s *inotify) addTree(dir string) {
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p != dir {
			s.push(p)
		}
		if !d.IsDir() {
			return nil
		}
		if p != dir && s.skip != nil && s.skip(p) {
			return filepath.SkipDir
		}
		s.mu.Lock()
		err = s.add(p)
		s.mu.Unlock()
		if err != nil {
			s.lost() // the scan, and the safety net, still see changes
			return filepath.SkipAll
		}
		return nil
	})
}

func (s *inotify) read() {
	buf := make([]byte, 64<<10)
	for {
		n, err := s.f.Read(buf)
		if err != nil {
			return // closed
		}
		for off := 0; off+syscall.SizeofInotifyEvent <= n; {
			ev := (*syscall.InotifyEvent)(unsafe.Pointer(&buf[off]))
			name := buf[off+syscall.SizeofInotifyEvent : off+syscall.SizeofInotifyEvent+int(ev.Len)]
			off += syscall.SizeofInotifyEvent + int(ev.Len)
			if i := bytes.IndexByte(name, 0); i >= 0 {
				name = name[:i]
			}
			s.event(ev.Wd, ev.Mask, string(name))
		}
	}
}

func (s *inotify) event(wd int32, mask uint32, name string) {
	if mask&syscall.IN_Q_OVERFLOW != 0 {
		s.lost()
		return
	}
	s.mu.Lock()
	dir, ok := s.wds[wd]
	if ok && mask&(syscall.IN_IGNORED|syscall.IN_DELETE_SELF) != 0 {
		// Gone, and its watch with it.
		delete(s.wds, wd)
		if s.dirs[dir] == wd {
			delete(s.dirs, dir)
		}
	}
	// A directory moved keeps its watch, which follows the directory, not
	// its name. Moved within a watched directory, it was reported there
	// by its new name, and adding that name found the same watch and
	// renamed it, before IN_MOVE_SELF, so the name it has is right. Moved
	// elsewhere, its name is now nothing's, and the watch goes, so that a
	// directory made under that name is watched afresh.
	if ok && mask&syscall.IN_MOVE_SELF != 0 {
		if _, err := os.Lstat(dir); err != nil {
			syscall.InotifyRmWatch(s.fd, uint32(wd))
			delete(s.wds, wd)
			if s.dirs[dir] == wd {
				delete(s.dirs, dir)
			}
		}
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	path := dir
	if name != "" {
		path = filepath.Join(dir, name)
	}
	s.push(path)
	if mask&syscall.IN_ISDIR != 0 && mask&(syscall.IN_CREATE|syscall.IN_MOVED_TO) != 0 && name != "" {
		if s.skip == nil || !s.skip(path) {
			s.addTree(path)
		}
	}
}

func (s *inotify) close() { s.f.Close() }
