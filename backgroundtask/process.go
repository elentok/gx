package backgroundtask

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/elentok/gx/syshealth"
)

// lastOutputLines is how many trailing lines of a task's output file a
// ProcessMatch carries.
const lastOutputLines = 3

// ProcessMatch is one background task matched to its process.
type ProcessMatch struct {
	// Alive is true when a descendant process was matched to the task.
	Alive bool
	PID   int
	// Ambiguous is true when the output file named no process and the command
	// text fit several. Nothing is picked then, so Alive is false.
	Ambiguous bool
	// LastOutput is the output file's modification time; zero when the file
	// is unreadable. A quiet file on a live process still reads Alive.
	LastOutput time.Time
	// LastLines are the output file's last lines, oldest first.
	LastLines []string
}

// ProcessFinder matches tasks to the descendants of one root pid (claude's,
// or the holder's).
type ProcessFinder struct {
	Procs syshealth.ProcessReader
	// Stdout lists the paths pid has open as its stdout. Nil means lsof.
	Stdout func(pid int) []string
}

// Match finds the task's process among rootPID's descendants. A process whose
// stdout is outputPath wins. Only when none is, a command-text fit decides:
// exactly one fit is the match, several are ambiguous, never "the oldest".
func (f ProcessFinder) Match(rootPID int, outputPath, command string) (ProcessMatch, error) {
	m := ProcessMatch{}
	m.LastOutput, m.LastLines = readOutputTail(outputPath)
	table, err := f.Procs.Read()
	if err != nil {
		return m, err
	}
	desc := descendants(table.Processes, rootPID)
	stdout := f.Stdout
	if stdout == nil {
		stdout = lsofStdout
	}
	want := canonical(outputPath)
	if want != "" {
		for _, p := range desc {
			for _, path := range stdout(p.PID) {
				if canonical(path) == want {
					m.Alive, m.PID = true, p.PID
					return m, nil
				}
			}
		}
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return m, nil
	}
	var fits []int
	for _, p := range desc {
		if strings.Contains(strings.Join(f.Procs.Argv(p.PID), " "), command) {
			fits = append(fits, p.PID)
		}
	}
	switch len(fits) {
	case 0:
	case 1:
		m.Alive, m.PID = true, fits[0]
	default:
		m.Ambiguous = true
	}
	return m, nil
}

// descendants lists every process below root, root excluded. Root 0 is the
// whole table: its chain can break at a process gx may not read.
func descendants(all []syshealth.Process, root int) []syshealth.Process {
	if root == 0 {
		return slices.DeleteFunc(slices.Clone(all), func(p syshealth.Process) bool { return p.Unreadable })
	}
	kids := map[int][]syshealth.Process{}
	for _, p := range all {
		if !p.Unreadable {
			kids[p.PPID] = append(kids[p.PPID], p)
		}
	}
	var out []syshealth.Process
	seen := map[int]bool{root: true}
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, k := range kids[pid] {
			if seen[k.PID] {
				continue
			}
			seen[k.PID] = true
			out = append(out, k)
			queue = append(queue, k.PID)
		}
	}
	return out
}

// canonical resolves symlinks, as lsof reports the resolved path (macOS /var
// is /private/var).
func canonical(path string) string {
	if path == "" {
		return ""
	}
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return filepath.Clean(path)
}

// lsofStdout reads the file pid has open as fd 1.
func lsofStdout(pid int) []string {
	cmd := exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "1", "-Fn")
	cmd.Env = []string{"LC_ALL=C"}
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var paths []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if p, ok := strings.CutPrefix(line, "n"); ok {
			paths = append(paths, p)
		}
	}
	return paths
}

// readOutputTail reads the file's mtime and last lastOutputLines lines.
func readOutputTail(path string) (time.Time, []string) {
	if path == "" {
		return time.Time{}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, nil
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return time.Time{}, nil
	}
	var ring []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		ring = append(ring, sc.Text())
		if len(ring) > lastOutputLines {
			ring = ring[1:]
		}
	}
	if sc.Err() != nil {
		return info.ModTime(), nil
	}
	return info.ModTime(), ring
}
