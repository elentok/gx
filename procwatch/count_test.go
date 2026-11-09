package procwatch

import (
	"testing"

	"github.com/elentok/gx/syshealth"
)

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
