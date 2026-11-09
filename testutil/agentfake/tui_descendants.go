package agentfake

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// ActionDescendants makes the TUI fake spawn a tree of long-lived processes
// under itself, to stage a fork storm without a real runaway: Control.Count
// sleepers at each of Control.Depth shell levels (Depth 0 or 1 is one level).
// The tree is one process group, killed by ActionStopDescendants and when the
// fake exits; a watcher inside it also ends the tree if the fake is killed
// outright.
const ActionDescendants = "descendants"

// ActionStopDescendants kills the tree an ActionDescendants started.
const ActionStopDescendants = "stopdescendants"

// SleeperCommand is the command line of each process of a descendants tree, as
// ps shows it, for a test that counts them.
const SleeperCommand = "sleep 100000"

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// descendantLevel is the script of one shell level: count sleepers and, below
// the top level, the next level's shell.
func descendantLevel(count, depth int) string {
	var b strings.Builder
	for range count {
		b.WriteString(SleeperCommand + " & ")
	}
	if depth > 1 {
		b.WriteString("sh -c ")
		b.WriteString(shellQuote(descendantLevel(count, depth-1)))
		b.WriteString(" & ")
	}
	b.WriteString("wait")
	return b.String()
}

// SpawnDescendants starts the tree under the calling process; it ends when
// the caller does. A fixture uses it for a tree that gx did not launch.
func SpawnDescendants(count, depth int) (pid int, err error) {
	var cmd *exec.Cmd
	if err := spawnDescendantsInto(&cmd, count, depth); err != nil {
		return 0, err
	}
	return cmd.Process.Pid, nil
}

func (t *tui) spawnDescendants(count, depth int) error {
	return spawnDescendantsInto(&t.descendants, count, depth)
}

// spawnDescendantsInto starts the tree into *slot, which the fake keeps so it
// can stop the tree later.
func spawnDescendantsInto(slot **exec.Cmd, count, depth int) error {
	if count <= 0 {
		return fmt.Errorf("descendants control needs a positive count, got %d", count)
	}
	if *slot != nil {
		return fmt.Errorf("descendants already running")
	}
	script := "sh -c " + shellQuote(descendantLevel(count, depth)) + " & " +
		"while kill -0 " + strconv.Itoa(os.Getpid()) + " 2>/dev/null; do sleep 1; done; kill 0"
	cmd := exec.Command("sh", "-c", script)
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	*slot = cmd
	return nil
}

func (t *tui) stopDescendants() { stopDescendantsIn(&t.descendants) }

func stopDescendantsIn(slot **exec.Cmd) {
	if *slot == nil {
		return
	}
	_ = syscall.Kill(-(*slot).Process.Pid, syscall.SIGKILL)
	_ = (*slot).Wait()
	*slot = nil
}
