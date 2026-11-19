package fswatch

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"syscall"
	"unsafe"
)

// On Windows the source is ReadDirectoryChangesW on each root, the fewest
// directories whose trees hold those watched, with the whole tree below
// each watched by the one call. The reads are overlapped and complete on
// one I/O completion port, which one goroutine waits on. A read whose
// records did not fit its buffer completes with none, which is reported
// as lost events: the watcher rescans everything.

const rdcwMask = syscall.FILE_NOTIFY_CHANGE_FILE_NAME | syscall.FILE_NOTIFY_CHANGE_DIR_NAME |
	syscall.FILE_NOTIFY_CHANGE_SIZE | syscall.FILE_NOTIFY_CHANGE_LAST_WRITE |
	syscall.FILE_NOTIFY_CHANGE_CREATION

// rdcwBufSize is the size of a read's buffer, at most 64 KiB, the most a
// network share takes.
const rdcwBufSize = 64 << 10

// fileNotifyInformation is FILE_NOTIFY_INFORMATION, followed by the rest
// of its name.
type fileNotifyInformation struct {
	NextEntryOffset uint32
	Action          uint32
	FileNameLength  uint32
	FileName        [1]uint16
}

// rdcwDir is one root being read. The kernel writes into buf and ov while
// a read is pending, so a dir stays referenced, in rdcw.dirs, until its
// read completes, cancelled or not.
type rdcwDir struct {
	buf     [rdcwBufSize]byte // first, for the alignment it needs
	ov      syscall.Overlapped
	path    string
	h       syscall.Handle
	pending bool // a read is issued and has not completed
	dropped bool // to be closed when its read completes
}

type rdcw struct {
	queue
	port syscall.Handle

	mu      sync.Mutex
	dirs    map[*syscall.Overlapped]*rdcwDir
	roots   map[string]*rdcwDir
	closing bool
	done    chan struct{}
}

// The completion port's functions, which package syscall declares with
// a key of the wrong size.
var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procCreateIoCompletionPort     = kernel32.NewProc("CreateIoCompletionPort")
	procGetQueuedCompletionStatus  = kernel32.NewProc("GetQueuedCompletionStatus")
	procPostQueuedCompletionStatus = kernel32.NewProc("PostQueuedCompletionStatus")
)

func createIoCompletionPort(file, port syscall.Handle, key uintptr, threads uint32) (syscall.Handle, error) {
	r, _, err := procCreateIoCompletionPort.Call(uintptr(file), uintptr(port), key, uintptr(threads))
	if r == 0 {
		return 0, err
	}
	return syscall.Handle(r), nil
}

func getQueuedCompletionStatus(port syscall.Handle, n *uint32, key *uintptr, ov **syscall.Overlapped, timeout uint32) error {
	r, _, err := procGetQueuedCompletionStatus.Call(uintptr(port), uintptr(unsafe.Pointer(n)),
		uintptr(unsafe.Pointer(key)), uintptr(unsafe.Pointer(ov)), uintptr(timeout))
	if r == 0 {
		return err
	}
	return nil
}

func postQueuedCompletionStatus(port syscall.Handle, n uint32, key uintptr, ov *syscall.Overlapped) error {
	r, _, err := procPostQueuedCompletionStatus.Call(uintptr(port), uintptr(n), key, uintptr(unsafe.Pointer(ov)))
	if r == 0 {
		return err
	}
	return nil
}

// leaked keeps the dirs whose reads never completed after close alive,
// since the kernel may still write to them.
var leaked struct {
	sync.Mutex
	dirs []*rdcwDir
}

func newSource(bool, func(string) bool) (source, error) {
	port, err := createIoCompletionPort(syscall.InvalidHandle, 0, 0, 1)
	if err != nil {
		return nil, fmt.Errorf("CreateIoCompletionPort: %w", err)
	}
	s := &rdcw{
		queue: newQueue(),
		port:  port,
		dirs:  map[*syscall.Overlapped]*rdcwDir{},
		roots: map[string]*rdcwDir{},
		done:  make(chan struct{}),
	}
	go s.loop()
	return s, nil
}

func (s *rdcw) watch(dirs []string) error {
	rs := roots(dirs)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return nil
	}
	for r, d := range s.roots {
		if !slices.Contains(rs, r) {
			s.drop(d)
			delete(s.roots, r)
		}
	}
	for _, r := range rs {
		if _, ok := s.roots[r]; ok {
			continue
		}
		d, err := s.open(r)
		if err != nil {
			if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) || errors.Is(err, syscall.ERROR_PATH_NOT_FOUND) {
				continue
			}
			return err
		}
		s.roots[r] = d
	}
	return nil
}

// open starts reading the changes below path. s.mu is held.
func (s *rdcw) open(path string) (*rdcwDir, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, syscall.FILE_LIST_DIRECTORY,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("watching %s: %w", path, err)
	}
	if _, err := createIoCompletionPort(h, s.port, 0, 0); err != nil {
		syscall.CloseHandle(h)
		return nil, fmt.Errorf("watching %s: %w", path, err)
	}
	d := &rdcwDir{path: path, h: h}
	s.dirs[&d.ov] = d
	if err := s.read(d); err != nil {
		delete(s.dirs, &d.ov)
		syscall.CloseHandle(h)
		return nil, fmt.Errorf("watching %s: %w", path, err)
	}
	return d, nil
}

// read issues the next read on d. s.mu is held.
func (s *rdcw) read(d *rdcwDir) error {
	d.ov = syscall.Overlapped{}
	err := syscall.ReadDirectoryChanges(d.h, &d.buf[0], rdcwBufSize, true, rdcwMask, nil, &d.ov, 0)
	if err != nil {
		return err
	}
	d.pending = true
	return nil
}

// drop stops reading d: its read is cancelled and it is closed when the
// cancellation completes. s.mu is held.
func (s *rdcw) drop(d *rdcwDir) {
	d.dropped = true
	if d.pending {
		syscall.CancelIoEx(d.h, &d.ov)
		return
	}
	syscall.CloseHandle(d.h)
	delete(s.dirs, &d.ov)
}

func (s *rdcw) loop() {
	defer close(s.done)
	defer syscall.CloseHandle(s.port)
	wait := uint32(syscall.INFINITE)
	for {
		var n uint32
		var key uintptr
		var ov *syscall.Overlapped
		err := getQueuedCompletionStatus(s.port, &n, &key, &ov, wait)
		s.mu.Lock()
		if s.closing {
			// Finish once every read has completed, or after a second
			// without one; close keeps what is left alive.
			wait = 1000
		}
		if ov == nil {
			done := err != nil || s.closing && len(s.dirs) == 0
			s.mu.Unlock()
			if done {
				return // the port is broken, timed out closing, or all is closed
			}
			continue
		}
		d := s.dirs[ov]
		if d == nil {
			s.mu.Unlock()
			continue
		}
		d.pending = false
		if d.dropped || s.closing {
			syscall.CloseHandle(d.h)
			delete(s.dirs, ov)
			done := s.closing && len(s.dirs) == 0
			s.mu.Unlock()
			if done {
				return
			}
			continue
		}
		var events []string
		switch {
		case err != nil || n == 0:
			// The buffer overflowed (n is 0), or the read failed, as when
			// the root itself is deleted.
			events = nil
		default:
			events = parseNotify(d.path, d.buf[:n])
		}
		if rerr := s.read(d); rerr != nil {
			// The root is gone or unreadable: stop watching it; the scan
			// and the safety net see what became of it.
			syscall.CloseHandle(d.h)
			delete(s.dirs, ov)
			delete(s.roots, d.path)
			err = rerr
		}
		s.mu.Unlock()
		if err != nil || n == 0 {
			s.lost()
			continue
		}
		for _, p := range events {
			s.push(p)
		}
	}
}

// parseNotify returns the paths named by the FILE_NOTIFY_INFORMATION
// records in buf, below root.
func parseNotify(root string, buf []byte) []string {
	var out []string
	for off := 0; off+int(unsafe.Offsetof(fileNotifyInformation{}.FileName)) <= len(buf); {
		fi := (*fileNotifyInformation)(unsafe.Pointer(&buf[off]))
		size := int(fi.FileNameLength / 2)
		name := unsafe.Slice(&fi.FileName[0], size)
		out = append(out, filepath.Join(root, syscall.UTF16ToString(name)))
		if fi.NextEntryOffset == 0 {
			break
		}
		off += int(fi.NextEntryOffset)
	}
	return out
}

func (s *rdcw) close() {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return
	}
	s.closing = true
	for _, d := range s.roots {
		s.drop(d)
	}
	s.roots = nil
	s.mu.Unlock()
	postQueuedCompletionStatus(s.port, 0, 0, nil)
	<-s.done
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.dirs) > 0 {
		leaked.Lock()
		for _, d := range s.dirs {
			leaked.dirs = append(leaked.dirs, d)
		}
		leaked.Unlock()
	}
}
