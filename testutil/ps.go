package testutil

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// PsRow is one live process: its pid, parent pid and command line.
type PsRow struct {
	PID, PPID int
	Command   string
}

func runPs(t *testing.T, env []string, format string) []string {
	t.Helper()
	cmd := exec.Command("ps", "-axww", "-o", format)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(string(out), "\n")
}

// PsLines is the command line of every live process on the host. env is the
// environment ps runs in; a fixture passes its own so its fake ps is the one used.
func PsLines(t *testing.T, env []string) []string {
	t.Helper()
	return runPs(t, env, "command=")
}

// PsRows is PsLines with each process's pid and parent pid, for walking trees.
func PsRows(t *testing.T, env []string) []PsRow {
	t.Helper()
	var rows []PsRow
	for _, line := range runPs(t, env, "pid=,ppid=,command=") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		rows = append(rows, PsRow{PID: pid, PPID: ppid, Command: strings.Join(fields[2:], " ")})
	}
	return rows
}
