package syshealth

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// linuxTickRate is USER_HZ, which is 100 on every Linux gx supports.
const linuxTickRate = 100

// ProcStat is what gx takes from /proc/stat.
type ProcStat struct {
	CPUIdle  uint64
	CPUTotal uint64
	// BootTime is btime, the boot moment in Unix seconds. Zero when absent.
	BootTime int64
}

// ParseProcStat reads the first "cpu" line of /proc/stat, and btime when
// present. Idle is idle + iowait. Total is user, nice, system, idle, iowait,
// irq, softirq and steal; guest and guest_nice are already inside user and nice.
func ParseProcStat(b []byte) (ProcStat, error) {
	var out ProcStat
	var haveCPU bool
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if f[0] == "btime" && len(f) >= 2 {
			bt, err := strconv.ParseInt(f[1], 10, 64)
			if err != nil {
				return ProcStat{}, fmt.Errorf("/proc/stat: btime: %w", err)
			}
			out.BootTime = bt
			continue
		}
		if f[0] != "cpu" || haveCPU {
			continue
		}
		if len(f) < 9 {
			return ProcStat{}, fmt.Errorf("/proc/stat: cpu line has %d fields, want at least 9", len(f))
		}
		var v [8]uint64
		for i := range v {
			n, err := strconv.ParseUint(f[i+1], 10, 64)
			if err != nil {
				return ProcStat{}, fmt.Errorf("/proc/stat: cpu field %d: %w", i+1, err)
			}
			v[i] = n
		}
		var total uint64
		for _, n := range v {
			total += n
		}
		out.CPUIdle, out.CPUTotal, haveCPU = v[3]+v[4], total, true
	}
	if !haveCPU {
		return ProcStat{}, fmt.Errorf("/proc/stat: no cpu line")
	}
	return out, nil
}

// MemInfo is what gx takes from /proc/meminfo, in bytes.
type MemInfo struct {
	Total     uint64
	Available uint64
}

// ParseMeminfo reads MemTotal and MemAvailable (both in kB) from /proc/meminfo.
func ParseMeminfo(b []byte) (MemInfo, error) {
	var total, avail uint64
	var haveTotal, haveAvail bool
	for _, line := range bytes.Split(b, []byte("\n")) {
		key, rest, ok := strings.Cut(string(line), ":")
		if !ok || (key != "MemTotal" && key != "MemAvailable") {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			return MemInfo{}, fmt.Errorf("/proc/meminfo: %s has no value", key)
		}
		kb, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			return MemInfo{}, fmt.Errorf("/proc/meminfo: %s: %w", key, err)
		}
		if key == "MemTotal" {
			total, haveTotal = kb*1024, true
		} else {
			avail, haveAvail = kb*1024, true
		}
	}
	if !haveTotal || !haveAvail {
		return MemInfo{}, fmt.Errorf("/proc/meminfo: missing MemTotal or MemAvailable")
	}
	if total == 0 || avail > total {
		return MemInfo{}, fmt.Errorf("/proc/meminfo: implausible MemTotal %d, MemAvailable %d", total, avail)
	}
	return MemInfo{Total: total, Available: avail}, nil
}

func readLinux() (Reading, error) { return readProcFS(os.DirFS("/proc")) }

// readProcFS builds a reading from "stat" and "meminfo" in procfs.
func readProcFS(procfs fs.FS) (Reading, error) {
	statBytes, err := fs.ReadFile(procfs, "stat")
	if err != nil {
		return Reading{}, err
	}
	stat, err := ParseProcStat(statBytes)
	if err != nil {
		return Reading{}, err
	}
	memBytes, err := fs.ReadFile(procfs, "meminfo")
	if err != nil {
		return Reading{}, err
	}
	mem, err := ParseMeminfo(memBytes)
	if err != nil {
		return Reading{}, err
	}
	return Reading{
		CPUIdle:    stat.CPUIdle,
		CPUTotal:   stat.CPUTotal,
		TickRate:   linuxTickRate,
		Cores:      runtime.NumCPU(),
		MemPercent: 100 - 100*float64(mem.Available)/float64(mem.Total),
		MemUsed:    mem.Total - mem.Available,
		MemTotal:   mem.Total,
	}, nil
}
