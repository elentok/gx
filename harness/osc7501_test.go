package harness_test

import (
	"encoding/base64"
	"reflect"
	"testing"

	"github.com/elentok/gx/harness"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func TestProgramStatusParser(t *testing.T) {
	report := func(r harness.ProgramReport) harness.ProgramReport {
		if r.Progress == 0 {
			r.Progress = -1
		}
		return r
	}
	tests := []struct {
		name    string
		chunks  []string
		probes  int
		reports []harness.ProgramReport
	}{
		{
			name:   "probe",
			chunks: []string{"\x1b]7501;?\a"},
			probes: 1,
		},
		{
			name:    "each state",
			chunks:  []string{"\x1b]7501;state=idle:app=claude-code\a\x1b]7501;state=working:app=claude-code\a\x1b]7501;state=done:app=claude-code\a"},
			reports: []harness.ProgramReport{report(harness.ProgramReport{State: "idle", App: "claude-code"}), report(harness.ProgramReport{State: "working", App: "claude-code"}), report(harness.ProgramReport{State: "done", App: "claude-code"})},
		},
		{
			name:   "blocked kind with base64 message",
			chunks: []string{"\x1b]7501;state=blocked:app=claude-code:kind=permission:msg=" + b64("approve Write: a.txt") + "\a"},
			reports: []harness.ProgramReport{report(harness.ProgramReport{
				State: "blocked", App: "claude-code", Kind: "permission", Message: "approve Write: a.txt",
			})},
		},
		{
			name:   "task id, progress and title",
			chunks: []string{"\x1b]7501;state=working:app=x:id=t1:progress=40:title=" + b64("Build") + "\a"},
			reports: []harness.ProgramReport{{
				State: "working", App: "x", TaskID: "t1", Progress: 40, Title: "Build",
			}},
		},
		{
			name:    "clear on exit",
			chunks:  []string{"\x1b]7501;state=clear\a"},
			reports: []harness.ProgramReport{report(harness.ProgramReport{State: "clear"})},
		},
		{
			name:    "ST terminator",
			chunks:  []string{"\x1b]7501;state=idle:app=a\x1b\\"},
			reports: []harness.ProgramReport{report(harness.ProgramReport{State: "idle", App: "a"})},
		},
		{
			name:    "split across two chunks",
			chunks:  []string{"text\x1b]75", "01;state=working:app=a\a more"},
			reports: []harness.ProgramReport{report(harness.ProgramReport{State: "working", App: "a"})},
		},
		{
			name:    "split inside the payload",
			chunks:  []string{"\x1b]7501;state=wor", "king:app=a\x1b", "\\"},
			reports: []harness.ProgramReport{report(harness.ProgramReport{State: "working", App: "a"})},
		},
		{
			name:    "probe split across chunks",
			chunks:  []string{"\x1b", "]7501;?", "\a"},
			probes:  1,
			reports: nil,
		},
		{
			name:    "junk around it",
			chunks:  []string{"\x1b[31mred\x1b]0;title\a\x1b]7501;state=idle:app=a\ahello\x1b]8;;x\a"},
			reports: []harness.ProgramReport{report(harness.ProgramReport{State: "idle", App: "a"})},
		},
		{
			name:    "a sequence cut by another escape is dropped",
			chunks:  []string{"\x1b]7501;state=working\x1b[0m\x1b]7501;state=idle\a"},
			reports: []harness.ProgramReport{report(harness.ProgramReport{State: "idle"})},
		},
		{
			name:   "report without state is dropped",
			chunks: []string{"\x1b]7501;app=a\a"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p harness.ProgramStatusParser
			probes := 0
			var reports []harness.ProgramReport
			for _, c := range tt.chunks {
				n, rs := p.Feed([]byte(c))
				probes += n
				reports = append(reports, rs...)
			}
			if probes != tt.probes {
				t.Errorf("probes = %d; want %d", probes, tt.probes)
			}
			if !reflect.DeepEqual(reports, tt.reports) {
				t.Errorf("reports = %+v; want %+v", reports, tt.reports)
			}
		})
	}
}
