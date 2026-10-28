package syshealth

import (
	"fmt"
	"io/fs"
	"os"
	"runtime"
)

// ProcessStart returns one pid's start time in Unix seconds, from the same
// source as the process reader, so it compares equal to Process.StartUnix.
func ProcessStart(pid int) (int64, error) {
	switch runtime.GOOS {
	case "linux":
		return linuxProcessStart(os.DirFS("/proc"), pid)
	case "darwin":
		return darwinProcessStart(pid)
	}
	return 0, ErrUnavailable
}

func linuxProcessStart(procfs fs.FS, pid int) (int64, error) {
	statBytes, err := fs.ReadFile(procfs, "stat")
	if err != nil {
		return 0, err
	}
	stat, err := ParseProcStat(statBytes)
	if err != nil {
		return 0, err
	}
	if stat.BootTime == 0 {
		return 0, fmt.Errorf("/proc/stat: no btime")
	}
	b, err := fs.ReadFile(procfs, fmt.Sprintf("%d/stat", pid))
	if err != nil {
		return 0, err
	}
	st, err := ParsePidStat(b)
	if err != nil {
		return 0, err
	}
	return stat.BootTime + int64(st.StartTicks/linuxTickRate), nil
}
