//go:build darwin && cgo

package syshealth

/*
#include <errno.h>
#include <libproc.h>
#include <mach/mach_time.h>
#include <stdlib.h>
#include <sys/proc_info.h>

static int bsdinfo(int pid, struct proc_bsdinfo *bi) {
	int r = proc_pidinfo(pid, PROC_PIDTBSDINFO, 0, bi, sizeof(*bi));
	return r <= 0 ? -errno : r;
}
static int taskinfo(int pid, struct proc_taskinfo *ti) {
	int r = proc_pidinfo(pid, PROC_PIDTASKINFO, 0, ti, sizeof(*ti));
	return r <= 0 ? -errno : r;
}
static int footprint(int pid, unsigned long long *out) {
	struct rusage_info_v4 ri;
	if (proc_pid_rusage(pid, RUSAGE_INFO_V4, (rusage_info_t *)&ri) != 0) {
		return -errno;
	}
	*out = ri.ri_phys_footprint;
	return 0;
}
*/
import "C"

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"unsafe"

	"golang.org/x/sys/unix"
)

// DarwinProcessReader reads the process table through libproc.
type DarwinProcessReader struct{}

// NewDarwinProcessReader returns the libproc reader.
func NewDarwinProcessReader() *DarwinProcessReader { return &DarwinProcessReader{} }

// Read lists every pid. Another user's process, or one that vanished, is an
// Unreadable row: libproc gives no CPU or memory for it without root. Argv is
// left empty: see Argv.
func (*DarwinProcessReader) Read() (ProcessTable, error) {
	reading, err := readDarwin()
	if err != nil {
		return ProcessTable{}, err
	}
	n := C.proc_listpids(C.PROC_ALL_PIDS, 0, nil, 0)
	if n <= 0 {
		return ProcessTable{}, fmt.Errorf("proc_listpids: size %d", int(n))
	}
	// Room for pids that start between the two calls.
	pids := make([]C.int, int(n)/int(unsafe.Sizeof(C.int(0)))+64)
	n = C.proc_listpids(C.PROC_ALL_PIDS, 0, unsafe.Pointer(&pids[0]), C.int(len(pids))*C.int(unsafe.Sizeof(C.int(0))))
	if n <= 0 {
		return ProcessTable{}, fmt.Errorf("proc_listpids: %d", int(n))
	}
	var tb C.mach_timebase_info_data_t
	C.mach_timebase_info(&tb)

	table := ProcessTable{Reading: reading}
	for _, pid := range pids[:int(n)/int(unsafe.Sizeof(C.int(0)))] {
		if pid == 0 {
			continue
		}
		table.Processes = append(table.Processes, readDarwinPid(int(pid), uint64(tb.numer), uint64(tb.denom)))
	}
	return table, nil
}

func readDarwinPid(pid int, numer, denom uint64) Process {
	unreadable := Process{PID: pid, Unreadable: true}
	var bi C.struct_proc_bsdinfo
	if C.bsdinfo(C.int(pid), &bi) < 0 {
		return unreadable
	}
	var ti C.struct_proc_taskinfo
	if C.taskinfo(C.int(pid), &ti) < 0 {
		return unreadable
	}
	mem, ok := darwinFootprint(pid)
	if !ok {
		return unreadable
	}
	ns := (uint64(ti.pti_total_user) + uint64(ti.pti_total_system)) * numer / denom
	return Process{
		PID:       pid,
		PPID:      int(bi.pbi_ppid),
		StartUnix: int64(bi.pbi_start_tvsec),
		CPUTicks:  ns / (1e9 / darwinTickRate),
		MemBytes:  mem,
		Comm:      C.GoString(&bi.pbi_comm[0]),
	}
}

// darwinFootprint is ri_phys_footprint, the memory figure Activity Monitor shows.
func darwinFootprint(pid int) (uint64, bool) {
	var out C.ulonglong
	if C.footprint(C.int(pid), &out) < 0 {
		return 0, false
	}
	return uint64(out), true
}

// Argv reads the process arguments through KERN_PROCARGS2, which only works for
// the same user. It returns nil when the pid is gone, hidden or has no argv.
func (*DarwinProcessReader) Argv(pid int) []string {
	b, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return nil
	}
	return parseProcArgs2(b)
}

// parseProcArgs2 reads the KERN_PROCARGS2 layout: argc (int32), the exec path,
// NUL padding, then argc NUL-terminated strings (the environment follows).
func parseProcArgs2(b []byte) []string {
	if len(b) < 4 {
		return nil
	}
	argc := int(binary.NativeEndian.Uint32(b))
	b = b[4:]
	i := bytes.IndexByte(b, 0)
	if i < 0 {
		return nil
	}
	b = bytes.TrimLeft(b[i:], "\x00")
	var argv []string
	for len(argv) < argc {
		i := bytes.IndexByte(b, 0)
		if i < 0 {
			break
		}
		argv = append(argv, string(b[:i]))
		b = b[i+1:]
	}
	return argv
}
