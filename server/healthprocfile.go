package server

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/syshealth"
)

// FileProcesses is a Config.ProcessSource and ProcessArgv pair for tests that
// steer the process table from outside the process. The file at path holds one
// line per entry, fields separated by spaces:
//
//	busy 3000                                 machine ticks busy per read (optional)
//	proc <pid> <ppid> <start> <cpu> <mem> <comm> [argv...]
//	hidden <pid>                              an unreadable row
//
// start is Unix seconds, mem takes a size such as 200M. Both tick values are
// per read: the source adds them to a running total on each read, so the
// shares between two reads are cpu over the fixed 10000 machine ticks a read
// reports. Hidden rows and a busy above the rows' sum give "other users". An
// empty file is a failed read.
func FileProcesses(path string) (func() (syshealth.ProcessTable, error), func(int) []string) {
	var mu sync.Mutex
	var idle, total uint64
	cum := map[int]uint64{}
	argvs := map[int][]string{}
	read := func() (syshealth.ProcessTable, error) {
		busy, procs, err := parseProcessFile(path)
		if err != nil {
			return syshealth.ProcessTable{}, err
		}
		mu.Lock()
		defer mu.Unlock()
		clear(argvs)
		for i, p := range procs {
			if p.Unreadable {
				continue
			}
			cum[p.PID] += p.CPUTicks
			procs[i].CPUTicks = cum[p.PID]
			argvs[p.PID] = p.Argv
			procs[i].Argv = nil
		}
		total += fileTicksPerRead
		idle += fileTicksPerRead - min(busy, fileTicksPerRead)
		return syshealth.ProcessTable{
			Reading:   syshealth.Reading{CPUIdle: idle, CPUTotal: total, TickRate: fileTickRate, Cores: fileCores},
			Processes: procs,
		}, nil
	}
	argv := func(pid int) []string {
		mu.Lock()
		defer mu.Unlock()
		return argvs[pid]
	}
	return read, argv
}

// parseProcessFile returns the busy ticks per read (the rows' sum unless a busy
// line sets it) and the rows, with Argv filled and CPUTicks per read.
func parseProcessFile(path string) (uint64, []syshealth.Process, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, nil, err
	}
	var busy, sum uint64
	var busySet bool
	var procs []syshealth.Process
	for n, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		fail := func(err error) error { return fmt.Errorf("%s:%d: %w", path, n+1, err) }
		switch f[0] {
		case "busy":
			if len(f) != 2 {
				return 0, nil, fail(fmt.Errorf("busy takes one value"))
			}
			if busy, err = strconv.ParseUint(f[1], 10, 64); err != nil {
				return 0, nil, fail(err)
			}
			busySet = true
		case "hidden":
			if len(f) != 2 {
				return 0, nil, fail(fmt.Errorf("hidden takes a pid"))
			}
			pid, err := strconv.Atoi(f[1])
			if err != nil {
				return 0, nil, fail(err)
			}
			procs = append(procs, syshealth.Process{PID: pid, Unreadable: true})
		case "proc":
			p, err := parseProcRow(f[1:])
			if err != nil {
				return 0, nil, fail(err)
			}
			sum += p.CPUTicks
			procs = append(procs, p)
		default:
			return 0, nil, fail(fmt.Errorf("unknown entry %q", f[0]))
		}
	}
	if len(procs) == 0 {
		return 0, nil, fmt.Errorf("%s has no rows", path)
	}
	if !busySet {
		busy = sum
	}
	return busy, procs, nil
}

func parseProcRow(f []string) (syshealth.Process, error) {
	if len(f) < 6 {
		return syshealth.Process{}, fmt.Errorf("proc needs pid ppid start cpu mem comm")
	}
	var p syshealth.Process
	var err error
	if p.PID, err = strconv.Atoi(f[0]); err != nil {
		return p, fmt.Errorf("pid: %w", err)
	}
	if p.PPID, err = strconv.Atoi(f[1]); err != nil {
		return p, fmt.Errorf("ppid: %w", err)
	}
	if p.StartUnix, err = strconv.ParseInt(f[2], 10, 64); err != nil {
		return p, fmt.Errorf("start: %w", err)
	}
	if p.CPUTicks, err = strconv.ParseUint(f[3], 10, 64); err != nil {
		return p, fmt.Errorf("cpu: %w", err)
	}
	mem, err := config.ParseByteSize(f[4])
	if err != nil {
		return p, fmt.Errorf("mem: %w", err)
	}
	p.MemBytes = uint64(mem)
	p.Comm = f[5]
	p.Argv = f[6:]
	return p, nil
}
