package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/elentok/gx/ui/terminalrun"
)

func TestDoctor_CheckGhostty(t *testing.T) {
	t.Parallel()
	ghostty := map[string]string{"TERM_PROGRAM": "ghostty", "TERM_PROGRAM_VERSION": "1.3.1"}
	tests := []struct {
		name       string
		vars       map[string]string
		probeErr   error
		want       string
		wantErr    bool
		wantProbed bool
	}{
		{"pass", ghostty, nil, "PASS: Ghostty AppleScript reachable", false, true},
		{"probe hint", ghostty, &terminalrun.FallbackError{Hint: "set macos-applescript = true"}, "FAIL: set macos-applescript = true", true, true},
		{"gate fail skips probe", map[string]string{"TERM_PROGRAM": "ghostty", "TERM_PROGRAM_VERSION": "1.2.0"}, nil, "FAIL: Ghostty 1.2.0 has no AppleScript splits", true, false},
		{"not ghostty", map[string]string{"TMUX": "x"}, nil, "SKIP: terminal is tmux, not ghostty", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			probed := false
			d := deps{
				stdout: &stdout,
				stderr: bytes.NewBuffer(nil),
				goos:   "darwin",
				getenv: func(key string) string { return tt.vars[key] },
				probeGhostty: func() error {
					probed = true
					return tt.probeErr
				},
			}
			err := execute([]string{"doctor", "--check-ghostty"}, d)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !strings.Contains(stdout.String(), tt.want) {
				t.Errorf("output %q missing %q", stdout.String(), tt.want)
			}
			if probed != tt.wantProbed {
				t.Errorf("probed = %v, want %v", probed, tt.wantProbed)
			}
		})
	}
}

func TestDoctor_RuntimeGhosttyLines(t *testing.T) {
	t.Parallel()
	for version, want := range map[string]string{
		"1.3.0": "ghostty version: 1.3.0 ok",
		"1.2.3": "ghostty version: 1.2.3 < 1.3.0, splits run in place",
	} {
		var out bytes.Buffer
		printDoctorRuntime(&out, func(k string) string {
			return map[string]string{"TERM_PROGRAM": "ghostty", "TERM_PROGRAM_VERSION": version}[k]
		}, "darwin")
		for _, w := range []string{`TERM_PROGRAM="ghostty"`, `TERM_PROGRAM_VERSION="` + version + `"`, want} {
			if !strings.Contains(out.String(), w) {
				t.Errorf("version %s: output %q missing %q", version, out.String(), w)
			}
		}
	}
	var out bytes.Buffer
	printDoctorRuntime(&out, func(string) string { return "" }, "darwin")
	if strings.Contains(out.String(), "ghostty version") {
		t.Errorf("gate line shown outside ghostty: %q", out.String())
	}
}
