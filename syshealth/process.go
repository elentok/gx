package syshealth

import "runtime"

// Process is one row of the process table.
type Process struct {
	PID  int
	PPID int
	// StartUnix is the process start in Unix seconds.
	StartUnix int64
	// CPUTicks is cumulative CPU time (user + system) in ticks of the
	// ProcessTable's Reading.TickRate.
	CPUTicks uint64
	// MemBytes is resident memory.
	MemBytes uint64
	Comm     string
	// Argv is empty until a separate argv read fills it.
	Argv []string
	// Unreadable marks a pid that vanished or is hidden; only PID is set.
	Unreadable bool
}

// ProcessReader reads the process table. Argv is a separate, costlier call.
type ProcessReader interface {
	Read() (ProcessTable, error)
	Argv(pid int) []string
}

// DefaultProcess returns the process reader for the OS gx is running on. An
// unknown OS gets a reader that reports ErrUnavailable.
func DefaultProcess() ProcessReader {
	if _, r := selectProcessBackend(runtime.GOOS); r != nil {
		return r
	}
	return unavailableProcessReader{}
}

type unavailableProcessReader struct{}

func (unavailableProcessReader) Read() (ProcessTable, error) { return ProcessTable{}, ErrUnavailable }
func (unavailableProcessReader) Argv(int) []string           { return nil }

// selectProcessBackend picks the process backend for goos, like selectBackend.
// An unknown OS gets nil.
func selectProcessBackend(goos string) (string, ProcessReader) {
	switch goos {
	case "linux":
		return "linux", NewLinuxProcessReader()
	case "darwin":
		return "darwin", NewDarwinProcessReader()
	}
	return "none", nil
}

// ProcessTable is one read of every process, with the machine reading taken
// at the same moment, so callers share one window for rows and busy total.
type ProcessTable struct {
	Reading   Reading
	Processes []Process
}
