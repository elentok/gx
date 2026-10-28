package syshealth

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// PidStat is what gx takes from /proc/<pid>/stat.
type PidStat struct {
	Comm       string
	PPID       int
	CPUTicks   uint64 // utime + stime
	StartTicks uint64 // since boot
	RSSPages   uint64
}

// ParsePidStat parses /proc/<pid>/stat. comm is split at the last ')', so a
// comm with spaces or ')' parses.
func ParsePidStat(b []byte) (PidStat, error) {
	open := bytes.IndexByte(b, '(')
	closeIdx := bytes.LastIndexByte(b, ')')
	if open < 0 || closeIdx < open {
		return PidStat{}, fmt.Errorf("/proc/<pid>/stat: no comm")
	}
	// rest[0] is field 3 (state), so field n is rest[n-3].
	rest := strings.Fields(string(b[closeIdx+1:]))
	if len(rest) < 22 {
		return PidStat{}, fmt.Errorf("/proc/<pid>/stat: %d fields after comm, want at least 22", len(rest))
	}
	st := PidStat{Comm: string(b[open+1 : closeIdx])}
	ppid, err := strconv.Atoi(rest[1])
	if err != nil {
		return PidStat{}, fmt.Errorf("/proc/<pid>/stat: ppid: %w", err)
	}
	st.PPID = ppid
	var u [4]uint64
	for i, idx := range []int{11, 12, 19, 21} {
		u[i], err = strconv.ParseUint(rest[idx], 10, 64)
		if err != nil {
			return PidStat{}, fmt.Errorf("/proc/<pid>/stat: field %d: %w", idx+3, err)
		}
	}
	st.CPUTicks, st.StartTicks, st.RSSPages = u[0]+u[1], u[2], u[3]
	return st, nil
}

// LinuxProcessReader reads the process table from a procfs.
type LinuxProcessReader struct {
	// FS is rooted at /proc.
	FS       fs.FS
	PageSize int
}

// NewLinuxProcessReader reads the real /proc.
func NewLinuxProcessReader() *LinuxProcessReader {
	return &LinuxProcessReader{FS: os.DirFS("/proc"), PageSize: os.Getpagesize()}
}

// Read lists every pid. A pid that vanished or is hidden is an Unreadable
// row, not an error. Argv is left empty: see Argv.
func (r *LinuxProcessReader) Read() (ProcessTable, error) {
	reading, err := readProcFS(r.FS)
	if err != nil {
		return ProcessTable{}, err
	}
	statBytes, err := fs.ReadFile(r.FS, "stat")
	if err != nil {
		return ProcessTable{}, err
	}
	stat, err := ParseProcStat(statBytes)
	if err != nil {
		return ProcessTable{}, err
	}
	if stat.BootTime == 0 {
		return ProcessTable{}, fmt.Errorf("/proc/stat: no btime")
	}
	entries, err := fs.ReadDir(r.FS, ".")
	if err != nil {
		return ProcessTable{}, err
	}
	table := ProcessTable{Reading: reading}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || !e.IsDir() {
			continue
		}
		table.Processes = append(table.Processes, r.readPid(pid, stat.BootTime))
	}
	return table, nil
}

func (r *LinuxProcessReader) readPid(pid int, boot int64) Process {
	b, err := fs.ReadFile(r.FS, fmt.Sprintf("%d/stat", pid))
	if err != nil {
		return Process{PID: pid, Unreadable: true}
	}
	st, err := ParsePidStat(b)
	if err != nil {
		return Process{PID: pid, Unreadable: true}
	}
	return Process{
		PID:       pid,
		PPID:      st.PPID,
		StartUnix: boot + int64(st.StartTicks/linuxTickRate),
		CPUTicks:  st.CPUTicks,
		MemBytes:  st.RSSPages * uint64(r.PageSize),
		Comm:      st.Comm,
	}
}

// Argv reads /proc/<pid>/cmdline. It returns nil when the pid is gone,
// hidden or has no argv (a kernel thread).
func (r *LinuxProcessReader) Argv(pid int) []string {
	b, err := fs.ReadFile(r.FS, fmt.Sprintf("%d/cmdline", pid))
	if err != nil {
		return nil
	}
	b = bytes.TrimSuffix(b, []byte{0})
	if len(b) == 0 {
		return nil
	}
	return strings.Split(string(b), "\x00")
}
