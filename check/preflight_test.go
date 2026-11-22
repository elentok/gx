package check

import "testing"

func TestPreflight_TopCPUPicksBusiestCommand(t *testing.T) {
	name, cpu := topCPU("  1.0 /sbin/launchd\n 87.5 /usr/bin/some tool\n  3.2 /bin/zsh\n")
	if name != "some tool" || cpu != 87.5 {
		t.Errorf("got %q %.1f", name, cpu)
	}
}

func TestPreflight_CountOrphansCountsOnlyDeadOwners(t *testing.T) {
	procs := []OrphanProc{
		{PID: 1, Command: "/tmp/gxfb-10-abc/gx serve"},
		{PID: 2, Command: "/tmp/gxfb-20-abc/gx serve"},
		{PID: 3, Command: "/usr/bin/other"},
	}
	n := CountOrphans("/tmp", procs, func(pid int) bool { return pid == 20 })
	if n != 1 {
		t.Errorf("got %d orphans, want 1", n)
	}
}
