// Package proctable reads the process table and watches adopted holders for exit.
package proctable

import (
	"errors"
	"fmt"
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

// PS is the real process table.
var PS ProcessTable = psTable{}

// OrPS returns p, or the real process table when p is nil.
func OrPS(p ProcessTable) ProcessTable {
	if p == nil {
		return psTable{}
	}
	return p
}

// psTable reads the process table with ps, which works the same on macOS and
// Linux and needs no cgo.
type psTable struct{}

func (psTable) Lookup(pid int) (Process, bool, error) {
	// One exec for both fields: Lookup runs on every poll of every adopted
	// session, so a second ps doubles the server's baseline child CPU. In the C
	// locale lstart is a fixed-width date and the command is the rest.
	out, ok, err := psOutput(pid, "lstart=,command=")
	if !ok || err != nil {
		return Process{}, ok, err
	}
	if len(out) <= lstartWidth {
		return Process{}, false, fmt.Errorf("ps output %q is too short for lstart and command", out)
	}
	return Process{Start: strings.TrimSpace(out[:lstartWidth]), Cmdline: strings.TrimSpace(out[lstartWidth:])}, true, nil
}

// lstartWidth is the length of ps's lstart in the C locale, "Mon Nov  9 07:33:28 2026".
const lstartWidth = 24

func psOutput(pid int, field string) (string, bool, error) {
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
