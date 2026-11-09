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

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// descendantLevel is the script of one shell level: count sleepers and, below
// the top level, the next level's shell.
func descendantLevel(count, depth int) string {
	var b strings.Builder
	for range count {
		b.WriteString("sleep 100000 & ")
	}
	if depth > 1 {
		b.WriteString("sh -c ")
		b.WriteString(shellQuote(descendantLevel(count, depth-1)))
		b.WriteString(" & ")
	}
	b.WriteString("wait")
	return b.String()
}

func (t *tui) spawnDescendants(count, depth int) error {
	if count <= 0 {
		return fmt.Errorf("descendants control needs a positive count, got %d", count)
	}
	if t.descendants != nil {
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
	t.descendants = cmd
	return nil
}

func (t *tui) stopDescendants() {
	if t.descendants == nil {
		return
	}
	_ = syscall.Kill(-t.descendants.Process.Pid, syscall.SIGKILL)
	_ = t.descendants.Wait()
	t.descendants = nil
}
