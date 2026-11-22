package proc

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW        = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJob      = kernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject      = kernel32.NewProc("TerminateJobObject")
	procGenerateConsoleCtrl     = kernel32.NewProc("GenerateConsoleCtrlEvent")
)

const (
	createNewProcessGroup           = 0x00000200
	ctrlBreakEvent                  = 1
	jobObjectExtendedLimitInfoClass = 9
	jobObjectLimitKillOnJobClose    = 0x00002000
	processSetQuota                 = 0x0100
	processTerminate                = 0x0001
)

// The layout of JOBOBJECT_EXTENDED_LIMIT_INFORMATION, as golang.org/x/sys
// declares it; spelt out here so the CLI needs no module for one call.
type ioCounters struct {
	ReadOperationCount, WriteOperationCount, OtherOperationCount uint64
	ReadTransferCount, WriteTransferCount, OtherTransferCount    uint64
}

type basicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type extendedLimitInformation struct {
	BasicLimitInformation basicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// sysProcAttr starts the child in a console process group of its own, so
// Ctrl-C in the terminal reaches ggui alone and Ctrl-Break can be sent to
// the child by itself.
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: createNewProcessGroup}
}

// group is a job object holding the child and everything it starts.
type group struct {
	pid int
	job syscall.Handle
}

// newGroup puts p in a new job that kills its members when the last handle
// to it closes: ggui holds the only one, so ggui exiting, however it
// exits, ends the app too. A process the child starts in the moment
// before it joins escapes the job; nothing in a Go program's start does.
func newGroup(p *os.Process) (group, error) {
	job, _, err := procCreateJobObjectW.Call(0, 0)
	if job == 0 {
		return group{}, fmt.Errorf("CreateJobObject: %w", err)
	}
	info := extendedLimitInformation{}
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose
	if r, _, err := procSetInformationJobObject.Call(job, jobObjectExtendedLimitInfoClass,
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info)); r == 0 {
		syscall.CloseHandle(syscall.Handle(job))
		return group{}, fmt.Errorf("SetInformationJobObject: %w", err)
	}
	h, err := syscall.OpenProcess(processSetQuota|processTerminate, false, uint32(p.Pid))
	if err != nil {
		syscall.CloseHandle(syscall.Handle(job))
		return group{}, fmt.Errorf("OpenProcess: %w", err)
	}
	defer syscall.CloseHandle(h)
	if r, _, err := procAssignProcessToJob.Call(job, uintptr(h)); r == 0 {
		syscall.CloseHandle(syscall.Handle(job))
		return group{}, fmt.Errorf("AssignProcessToJobObject: %w", err)
	}
	return group{pid: p.Pid, job: syscall.Handle(job)}, nil
}

// interrupt sends Ctrl-Break to the child's console group, which a Go
// program exits on by default and can catch as os.Interrupt. A program
// with no console of its own does not receive it, and is killed after
// the timeout instead.
func (g group) interrupt() { procGenerateConsoleCtrl.Call(ctrlBreakEvent, uintptr(g.pid)) }

func (g group) kill() {
	if g.job != 0 {
		procTerminateJobObject.Call(uintptr(g.job), 1)
	}
}

func (g *group) close() {
	if g.job != 0 {
		syscall.CloseHandle(g.job)
		g.job = 0
	}
}
