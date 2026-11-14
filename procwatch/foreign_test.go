package procwatch

import (
	"reflect"
	"testing"

	"github.com/elentok/gx/syshealth"
)

const me = 501

func row(pid, ppid, uid int) syshealth.Process {
	return syshealth.Process{PID: pid, PPID: ppid, UID: uid, StartUnix: 100}
}

// kids returns n rows of uid under parent, numbered from first.
func kids(first, n, parent, uid int) []syshealth.Process {
	var out []syshealth.Process
	for i := range n {
		out = append(out, row(first+i, parent, uid))
	}
	return out
}

func concat(parts ...[]syshealth.Process) []syshealth.Process {
	var out []syshealth.Process
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestForeignStorms(t *testing.T) {
	t.Parallel()
	launchd := row(1, 0, 0)
	tests := []struct {
		name      string
		procs     []syshealth.Process
		gx        []int
		threshold int
		want      []Storm
	}{
		{
			"tree under a root owned by another user's parent",
			concat([]syshealth.Process{launchd, row(10, 1, me)}, kids(11, 5, 10, me)),
			nil, 3, []Storm{{Root: 10, Descendants: 5}},
		},
		{
			"at the threshold is not a storm",
			concat([]syshealth.Process{launchd, row(10, 1, me)}, kids(11, 3, 10, me)),
			nil, 3, nil,
		},
		{
			"gx tree under a terminal is left out of the terminal's count",
			concat([]syshealth.Process{launchd, row(10, 1, me), row(20, 10, me)}, kids(21, 8, 20, me), kids(40, 2, 10, me)),
			[]int{20}, 3, nil,
		},
		{
			"gx root's own tree is never foreign",
			concat([]syshealth.Process{launchd, row(20, 1, me)}, kids(21, 8, 20, me)),
			[]int{20}, 3, nil,
		},
		{
			"another user's tree is ignored",
			concat([]syshealth.Process{launchd, row(10, 1, 0)}, kids(11, 5, 10, 0)),
			nil, 3, nil,
		},
		{
			"only the root counts, not each process below it",
			concat([]syshealth.Process{launchd, row(10, 1, me)}, kids(11, 5, 10, me)),
			nil, 3, []Storm{{Root: 10, Descendants: 5}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := NewForest(tt.procs).ForeignStorms(me, tt.gx, tt.threshold)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

type tableReader struct{ procs []syshealth.Process }

func (r *tableReader) Read() (syshealth.ProcessTable, error) {
	return syshealth.ProcessTable{Processes: r.procs}, nil
}
func (*tableReader) Argv(int) []string { return nil }

func TestWatcher_ForeignNoticedOncePerStormAndAgainOnlyAfterADrop(t *testing.T) {
	t.Parallel()
	big := concat([]syshealth.Process{row(1, 0, 0), row(10, 1, me)}, kids(11, 5, 10, me))
	small := concat([]syshealth.Process{row(1, 0, 0), row(10, 1, me)}, kids(11, 1, 10, me))
	reader := &tableReader{procs: big}
	var got []Storm
	w := &Watcher{
		Config:  Config{Threshold: 3},
		Roots:   func() []int { return nil },
		Foreign: func(s Storm) { got = append(got, s) },
		UID:     me,
	}
	for _, step := range []struct {
		procs []syshealth.Process
		total int
	}{{big, 1}, {big, 1}, {small, 1}, {big, 2}, {big, 2}} {
		reader.procs = step.procs
		w.poll(reader)
		if len(got) != step.total {
			t.Fatalf("after a poll with %d rows: %d notices, want %d", len(step.procs), len(got), step.total)
		}
	}
}
