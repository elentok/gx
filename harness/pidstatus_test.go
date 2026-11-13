package harness_test

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/harness"
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
		want   harness.PidStatus
	}{
		{"starting", ``, harness.PidStatus{Status: harness.ClaudeStarting}},
		{"busy", `"status":"busy",`, harness.PidStatus{Status: harness.ClaudeBusy}},
		{"idle", `"status":"idle",`, harness.PidStatus{Status: harness.ClaudeIdle}},
	}
	for _, reason := range []string{
		"permission prompt", "input needed", "dialog open",
		"goal proposal", "sandbox request", "worker request",
	} {
		tests = append(tests, struct {
			name   string
			fields string
			want   harness.PidStatus
		}{
			"waiting " + reason,
			`"status":"waiting","waitingFor":"` + reason + `",`,
			harness.PidStatus{Status: harness.ClaudeWaiting, WaitingFor: reason},
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := harness.ParsePidStatus([]byte(pidFileJSON(tt.fields)))
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
	if got, _ := harness.ClaudeSessionsDirIn([]string{"HOME=/h"}); got != filepath.Join("/h", ".claude", "sessions") {
		t.Errorf("default = %q", got)
	}
	if got, _ := harness.ClaudeSessionsDirIn([]string{"HOME=/h", "CLAUDE_CONFIG_DIR=/cfg"}); got != filepath.Join("/cfg", "sessions") {
		t.Errorf("with CLAUDE_CONFIG_DIR = %q", got)
	}
}
