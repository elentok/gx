package procwatch

import (
	"testing"

	"github.com/elentok/gx/syshealth"
)

// syntheticTable is n processes in chains of 10 under launchd-like roots.
func syntheticTable(n int) []syshealth.Process {
	procs := make([]syshealth.Process, 0, n)
	for i := range n {
		ppid := 1
		if i%10 != 0 {
			ppid = i - 1 + 100
		}
		procs = append(procs, syshealth.Process{PID: i + 100, PPID: ppid, StartUnix: int64(i)})
	}
	return procs
}

func BenchmarkForeignStorms(b *testing.B) {
	procs := syntheticTable(4000)
	for b.Loop() {
		ForeignStorms(procs, 0, []int{100}, 5)
	}
}

func TestForeignStorms_LargeTableFindsChains(t *testing.T) {
	t.Parallel()
	procs := syntheticTable(4000)
	if got := len(ForeignStorms(procs, 0, nil, 5)); got != 400 {
		t.Fatalf("storms = %d, want 400", got)
	}
}

func TestCount(t *testing.T) {
	t.Parallel()
	p := func(pid, ppid int, start int64) syshealth.Process {
		return syshealth.Process{PID: pid, PPID: ppid, StartUnix: start}
	}
	tests := []struct {
		name  string
		procs []syshealth.Process
		root  int
		want  int
	}{
		{"children and grandchildren", []syshealth.Process{p(10, 1, 100), p(11, 10, 101), p(12, 10, 101), p(13, 11, 102), p(20, 1, 100)}, 10, 3},
		{"root alone", []syshealth.Process{p(10, 1, 100), p(20, 1, 100)}, 10, 0},
		{"root absent", []syshealth.Process{p(11, 10, 101)}, 10, 0},
		{"reused parent pid started after the child", []syshealth.Process{p(10, 1, 200), p(11, 10, 100)}, 10, 0},
		{"unreadable row breaks the chain", []syshealth.Process{p(10, 1, 100), {PID: 11, Unreadable: true}, p(12, 11, 102)}, 10, 0},
		{"pid cycle terminates", []syshealth.Process{p(10, 1, 100), p(11, 12, 101), p(12, 11, 101)}, 10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Count(tt.procs, tt.root); got != tt.want {
				t.Fatalf("Count = %d, want %d", got, tt.want)
			}
		})
	}
}
