package nativerunner

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Process is what reattach needs to know about a running pid.
type Process struct {
	// Start is the process start time as ps prints it; together with the
	// pid it tells a reused pid apart.
	Start   string
	Cmdline string
}

// ProcessTable looks up running processes. Tests swap in a fake one.
type ProcessTable interface {
	// Lookup reports false when no process has pid.
	Lookup(pid int) (Process, bool, error)
}

// psTable reads the process table with ps, which works the same on macOS and
// Linux and needs no cgo.
type psTable struct{}

func (psTable) Lookup(pid int) (Process, bool, error) {
	start, ok, err := psField(pid, "lstart=")
	if !ok || err != nil {
		return Process{}, ok, err
	}
	cmdline, ok, err := psField(pid, "command=")
	if !ok || err != nil {
		return Process{}, ok, err
	}
	return Process{Start: start, Cmdline: cmdline}, true, nil
}

func psField(pid int, field string) (string, bool, error) {
	cmd := exec.Command("ps", "-ww", "-p", strconv.Itoa(pid), "-o", field)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && len(strings.TrimSpace(string(out))) == 0 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(string(out)), true, nil
}
