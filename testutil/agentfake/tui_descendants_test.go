package agentfake

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// descendantsOf is the pids below root in the process table.
func descendantsOf(t *testing.T, root int) []int {
	t.Helper()
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=").Output()
	if err != nil {
		t.Fatal(err)
	}
	children := map[int][]int{}
	for line := range strings.SplitSeq(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		pid, _ := strconv.Atoi(f[0])
		ppid, _ := strconv.Atoi(f[1])
		children[ppid] = append(children[ppid], pid)
	}
	var all []int
	for queue := []int{root}; len(queue) > 0; queue = queue[1:] {
		kids := children[queue[0]]
		all = append(all, kids...)
		queue = append(queue, kids...)
	}
	return all
}

func (f *tuiFake) waitDescendants(what string, ok func(n int) bool) {
	f.t.Helper()
	f.waitFor(what, func() bool { return ok(len(descendantsOf(f.t, f.cmd.Process.Pid))) })
}

func TestTUIClaude_DescendantsAreSpawnedNestedAndStopped(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")

	// 3 sleepers on each of 2 levels, plus the shells and the watcher.
	f.control(Control{Action: ActionDescendants, Count: 3, Depth: 2})
	f.waitDescendants("at least 6 descendants", func(n int) bool { return n >= 6 })

	f.control(Control{Action: ActionStopDescendants})
	f.waitDescendants("no descendants after stop", func(n int) bool { return n == 0 })
}

func TestTUIClaude_DescendantsDieWithTheFake(t *testing.T) {
	f := startTUIFake(t, tuiLaunchArgs)
	f.waitStatus(StatusIdle, "")
	f.control(Control{Action: ActionDescendants, Count: 4})
	var pids []int
	f.waitFor("4 descendants", func() bool {
		pids = descendantsOf(t, f.cmd.Process.Pid)
		return len(pids) >= 4
	})

	f.control(Control{Action: ActionExit})
	f.waitExit()
	f.waitFor("the descendants to be gone", func() bool {
		for _, pid := range pids {
			if exec.Command("kill", "-0", strconv.Itoa(pid)).Run() == nil {
				return false
			}
		}
		return true
	})
}
