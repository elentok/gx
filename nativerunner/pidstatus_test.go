package nativerunner_test

import (
	"path/filepath"
	"testing"

	claudeharness "github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner"
)

// pidFileJSON is a pid status file as claude writes it, with the given
// status fields spliced in.
func pidFileJSON(statusFields string) string {
	return `{"pid":4242,"sessionId":"s1","cwd":"/w","startedAt":1,"procStart":"x",` +
		`"version":"2.1.296","kind":"interactive","entrypoint":"cli",` + statusFields + `"updatedAt":2}`
}

func TestParsePidStatus(t *testing.T) {
	tests := []struct {
		name   string
		fields string
		want   nativerunner.PidStatus
	}{
		{"starting", ``, nativerunner.PidStatus{Status: nativerunner.ClaudeStarting}},
		{"busy", `"status":"busy",`, nativerunner.PidStatus{Status: nativerunner.ClaudeBusy}},
		{"idle", `"status":"idle",`, nativerunner.PidStatus{Status: nativerunner.ClaudeIdle}},
	}
	for _, reason := range []string{
		"permission prompt", "input needed", "dialog open",
		"goal proposal", "sandbox request", "worker request",
	} {
		tests = append(tests, struct {
			name   string
			fields string
			want   nativerunner.PidStatus
		}{
			"waiting " + reason,
			`"status":"waiting","waitingFor":"` + reason + `",`,
			nativerunner.PidStatus{Status: nativerunner.ClaudeWaiting, WaitingFor: reason},
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nativerunner.ParsePidStatus([]byte(pidFileJSON(tt.fields)))
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v; want %+v", got, tt.want)
			}
		})
	}
}

func TestClaudeSessionsDirIn(t *testing.T) {
	if got, _ := claudeharness.ClaudeSessionsDirIn([]string{"HOME=/h"}); got != filepath.Join("/h", ".claude", "sessions") {
		t.Errorf("default = %q", got)
	}
	if got, _ := claudeharness.ClaudeSessionsDirIn([]string{"HOME=/h", "CLAUDE_CONFIG_DIR=/cfg"}); got != filepath.Join("/cfg", "sessions") {
		t.Errorf("with CLAUDE_CONFIG_DIR = %q", got)
	}
}
