package syshealth

import (
	"math"
	"testing"
)

// The fake machine has 10 cores and 100 ticks a second. The window is 10
// seconds, so it holds 10000 ticks and one full core is 1000 ticks (10%).
const (
	fakeCores    = 10
	fakeRate     = 100
	windowTicks  = fakeCores * fakeRate * 10
	oneCoreTicks = fakeRate * 10
)

func fakeTable(idleDelta uint64, procs ...Process) (prev, cur ProcessTable) {
	base := Reading{CPUIdle: 1000, CPUTotal: 5000, TickRate: fakeRate, Cores: fakeCores}
	next := base
	next.CPUIdle += idleDelta
	next.CPUTotal += windowTicks
	return ProcessTable{Reading: base}, ProcessTable{Reading: next, Processes: procs}
}

// withTicks puts the process in both tables, burning delta ticks in the window.
func withTicks(prev, cur *ProcessTable, p Process, delta uint64) {
	before := p
	before.CPUTicks = 500
	p.CPUTicks = 500 + delta
	prev.Processes = append(prev.Processes, before)
	cur.Processes = append(cur.Processes, p)
}

func argvFrom(m map[int][]string) func(int) []string {
	return func(pid int) []string { return m[pid] }
}

func pr(pid, ppid int, comm string, mem uint64) Process {
	return Process{PID: pid, PPID: ppid, StartUnix: 1000 + int64(pid), Comm: comm, MemBytes: mem}
}

func key(p Process) OwnerKey { return OwnerKey{p.PID, p.StartUnix} }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestTopCauses_AgentTreeIsOneRow(t *testing.T) {
	prev, cur := fakeTable(windowTicks / 2)
	runner := pr(10, 1, "runner", 100)
	claude := pr(11, 10, "claude", 200)
	tests := pr(12, 11, "go", 300)
	withTicks(&prev, &cur, runner, 0)
	withTicks(&prev, &cur, claude, oneCoreTicks)
	withTicks(&prev, &cur, tests, 2*oneCoreTicks)
	owners := map[OwnerKey]Owner{key(runner): {OwnerTicket, "gx:epic/01"}}
	argv := argvFrom(map[int][]string{12: {"go", "test", "./..."}, 11: {"claude"}})

	got := TopCauses(prev, cur, owners, argv)

	want := Cause{Kind: KindAgent, Label: "gx:epic/01", Ticket: "gx:epic/01", Busiest: "go test", MemBytes: 600}
	if len(got.CPU) != 1 || got.CPU[0].Kind != want.Kind || got.CPU[0].Label != want.Label ||
		got.CPU[0].Ticket != want.Ticket || got.CPU[0].Busiest != want.Busiest || got.CPU[0].MemBytes != want.MemBytes {
		t.Fatalf("cpu rows = %+v, want one %+v", got.CPU, want)
	}
	if !near(got.CPU[0].CPU, 30) {
		t.Errorf("cpu = %v, want 30", got.CPU[0].CPU)
	}
	if len(got.Memory) != 1 || got.Memory[0].Busiest != "go test" {
		t.Errorf("memory rows = %+v", got.Memory)
	}
}

func TestTopCauses_Labels(t *testing.T) {
	session := pr(20, 1, "node", 10)
	server := pr(30, 1, "gx", 10)
	gitChild := pr(31, 30, "git", 10)
	loneClaude := pr(40, 1, "claude", 10)
	vim := pr(50, 1, "vim", 10)
	reparented := pr(60, 1, "tail", 10) // its owner died: parent is pid 1
	deadOwner := pr(70, 1, "runner", 10)
	reparented.PPID = 1

	cases := []struct {
		name   string
		proc   Process
		owners map[OwnerKey]Owner
		want   Cause
	}{
		{"session without ticket", session, map[OwnerKey]Owner{key(session): {OwnerSession, "mysession"}},
			Cause{Kind: KindSession, Label: "mysession"}},
		{"claude without owner", loneClaude, nil, Cause{Kind: KindProcess, Label: "claude (not gx)"}},
		{"unowned process", vim, nil, Cause{Kind: KindProcess, Label: "vim"}},
		{"gx server", server, map[OwnerKey]Owner{key(server): {Kind: OwnerServer}},
			Cause{Kind: KindProcess, Label: "gx server"}},
		{"git child of the server", gitChild, map[OwnerKey]Owner{key(server): {Kind: OwnerServer}},
			Cause{Kind: KindProcess, Label: "git status"}},
		{"reparented to pid 1", reparented, map[OwnerKey]Owner{key(deadOwner): {OwnerTicket, "gx:e/09"}},
			Cause{Kind: KindProcess, Label: "tail"}},
		{"start time one second off still matches", session,
			map[OwnerKey]Owner{{20, session.StartUnix + 1}: {OwnerSession, "mysession"}},
			Cause{Kind: KindSession, Label: "mysession"}},
	}
	argv := argvFrom(map[int][]string{
		40: {"/home/u/.local/share/claude/versions/2.0/claude"},
		50: {"vim", "x.go"}, 31: {"git", "status"}, 60: {"tail", "-f", "log"},
	})
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prev, cur := fakeTable(windowTicks / 2)
			if c.proc.PID == 31 {
				srv := server
				withTicks(&prev, &cur, srv, 0)
			}
			withTicks(&prev, &cur, c.proc, oneCoreTicks)
			rows := TopCauses(prev, cur, c.owners, argv).CPU
			var got *Cause
			for i := range rows {
				if rows[i].Label == c.want.Label {
					got = &rows[i]
				}
			}
			if got == nil || got.Kind != c.want.Kind {
				t.Fatalf("rows = %+v, want label %q kind %q", rows, c.want.Label, c.want.Kind)
			}
		})
	}
}

func TestTopCauses_ServerDoesNotAbsorbChild(t *testing.T) {
	server := pr(30, 1, "gx", 10)
	child := pr(31, 30, "git", 10)
	prev, cur := fakeTable(windowTicks / 2)
	withTicks(&prev, &cur, server, oneCoreTicks)
	withTicks(&prev, &cur, child, 3*oneCoreTicks)
	owners := map[OwnerKey]Owner{key(server): {Kind: OwnerServer}}

	rows := TopCauses(prev, cur, owners, argvFrom(map[int][]string{31: {"git", "gc"}})).CPU

	if len(rows) != 2 || rows[0].Label != "git gc" || rows[1].Label != "gx server" {
		t.Fatalf("rows = %+v", rows)
	}
	if !near(rows[1].CPU, 10) || !near(rows[0].CPU, 30) {
		t.Errorf("cpu = %v and %v", rows[0].CPU, rows[1].CPU)
	}
}

func TestTopCauses_FullCoreIsTenPercentAndRowsFitBusy(t *testing.T) {
	prev, cur := fakeTable(windowTicks * 9 / 10) // busy = 10%
	a := pr(1000, 1, "a", 1)
	withTicks(&prev, &cur, a, oneCoreTicks)

	got := TopCauses(prev, cur, nil, argvFrom(nil)).CPU

	if len(got) != 1 || !near(got[0].CPU, 10) {
		t.Fatalf("rows = %+v, want one at 10%%", got)
	}
	busy, _ := BusyPercent(&prev.Reading, cur.Reading)
	var sum float64
	for _, c := range got {
		sum += c.CPU
	}
	if sum > busy+1e-9 {
		t.Errorf("rows sum %v above busy %v", sum, busy)
	}
}

func TestTopCauses_OtherUsers(t *testing.T) {
	a := pr(100, 1, "a", 5)
	hidden := Process{PID: 101, Unreadable: true}

	t.Run("shows when it makes the top 3", func(t *testing.T) {
		prev, cur := fakeTable(windowTicks / 2) // busy 50%
		withTicks(&prev, &cur, a, oneCoreTicks) // 10%
		cur.Processes = append(cur.Processes, hidden)

		rows := TopCauses(prev, cur, nil, argvFrom(nil)).CPU

		if len(rows) != 2 || rows[0].Kind != KindOtherUsers || !near(rows[0].CPU, 40) {
			t.Fatalf("rows = %+v", rows)
		}
	})
	t.Run("hidden when it does not make the top 3", func(t *testing.T) {
		prev, cur := fakeTable(windowTicks * 6 / 10) // busy 40%
		for i := range 4 {
			withTicks(&prev, &cur, pr(200+i, 1, "p", 5), 9*oneCoreTicks/10) // 9% each
		}
		cur.Processes = append(cur.Processes, hidden)

		rows := TopCauses(prev, cur, nil, argvFrom(nil)).CPU

		if len(rows) != 3 {
			t.Fatalf("rows = %+v", rows)
		}
		for _, r := range rows {
			if r.Kind == KindOtherUsers {
				t.Errorf("other users listed: %+v", rows)
			}
		}
	})
	t.Run("vanished pid counts as unreadable", func(t *testing.T) {
		prev, cur := fakeTable(windowTicks / 2)
		withTicks(&prev, &cur, a, oneCoreTicks)
		prev.Processes = append(prev.Processes, pr(300, 1, "gone", 1))

		rows := TopCauses(prev, cur, nil, argvFrom(nil)).CPU

		if rows[0].Kind != KindOtherUsers || !near(rows[0].CPU, 40) {
			t.Fatalf("rows = %+v", rows)
		}
	})
	t.Run("never shows when every process was readable", func(t *testing.T) {
		prev, cur := fakeTable(windowTicks / 2)
		withTicks(&prev, &cur, a, oneCoreTicks)

		rows := TopCauses(prev, cur, nil, argvFrom(nil)).CPU

		if len(rows) != 1 || rows[0].Kind == KindOtherUsers {
			t.Fatalf("rows = %+v", rows)
		}
	})
	t.Run("floors at zero", func(t *testing.T) {
		prev, cur := fakeTable(windowTicks * 9 / 10) // busy 10%
		withTicks(&prev, &cur, a, 2*oneCoreTicks)    // 20%: more than busy
		cur.Processes = append(cur.Processes, hidden)

		for _, r := range TopCauses(prev, cur, nil, argvFrom(nil)).CPU {
			if r.Kind == KindOtherUsers {
				t.Fatalf("other users listed: %+v", r)
			}
		}
	})
}

func TestTopCauses_MemoryList(t *testing.T) {
	prev, cur := fakeTable(windowTicks / 2)
	withTicks(&prev, &cur, pr(1, 1, "small", 10), 0)
	withTicks(&prev, &cur, pr(2, 1, "big", 900), 0)
	withTicks(&prev, &cur, pr(3, 1, "mid", 500), 0)
	withTicks(&prev, &cur, pr(4, 1, "mid2", 400), 0)
	cur.Processes = append(cur.Processes, Process{PID: 5, Unreadable: true})

	got := TopCauses(prev, cur, nil, argvFrom(nil))

	var labels []string
	for _, r := range got.Memory {
		if r.Kind == KindOtherUsers {
			t.Fatalf("memory has other users: %+v", got.Memory)
		}
		labels = append(labels, r.Label)
	}
	if len(labels) != 3 || labels[0] != "big" || labels[1] != "mid" || labels[2] != "mid2" {
		t.Errorf("memory labels = %v", labels)
	}
}

func TestTopCauses_PidReuse(t *testing.T) {
	prev, cur := fakeTable(windowTicks / 2)
	old := pr(10, 1, "old", 1)
	old.CPUTicks = 500
	reused := pr(10, 1, "new", 1)
	reused.StartUnix = old.StartUnix + 500
	reused.CPUTicks = 500 + 5*oneCoreTicks
	prev.Processes = []Process{old}
	cur.Processes = []Process{reused}
	owners := map[OwnerKey]Owner{key(old): {OwnerTicket, "gx:e/01"}}

	rows := TopCauses(prev, cur, owners, argvFrom(nil)).CPU

	for _, r := range rows {
		if r.Kind == KindAgent || (r.Kind == KindProcess && r.CPU > 0) {
			t.Errorf("reused pid carried over: %+v", r)
		}
	}
}

func TestTopCauses_NoUsableWindow(t *testing.T) {
	prev, cur := fakeTable(windowTicks / 2)
	cur.Reading.CPUTotal = prev.Reading.CPUTotal // no ticks in the window
	cur.Reading.CPUIdle = prev.Reading.CPUIdle
	withTicks(&prev, &cur, pr(1, 1, "a", 10), oneCoreTicks)

	got := TopCauses(prev, cur, nil, argvFrom(nil))

	if len(got.CPU) != 0 || len(got.Memory) != 1 {
		t.Errorf("got %+v", got)
	}
}
