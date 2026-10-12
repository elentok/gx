package nativerunner_test

import (
	"fmt"
	"path/filepath"
	"testing"

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

func TestClaudeSessionsDir(t *testing.T) {
	t.Setenv("HOME", "/h")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if got, _ := nativerunner.ClaudeSessionsDir(); got != filepath.Join("/h", ".claude", "sessions") {
		t.Errorf("default = %q", got)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "/cfg")
	if got, _ := nativerunner.ClaudeSessionsDir(); got != filepath.Join("/cfg", "sessions") {
		t.Errorf("with CLAUDE_CONFIG_DIR = %q", got)
	}
}

// pidChild writes a starting pid status file, then sets each status line it
// reads, adding a waitingFor only while waiting, as claude does.
const pidChild = `d="$HOME/.claude/sessions"; mkdir -p "$d"; f="$d/$$.json"; t=$(date +%s)000
printf '{"pid":%s,"startedAt":%s}' $$ "$t" > "$f"; printf 'ready\n'
while read -r s; do
  w=""; [ "$s" = waiting ] && w=',"waitingFor":"input needed"'
  printf '{"pid":%s,"startedAt":%s,"status":"%s"%s}' $$ "$t" "$s" "$w" > "$f.tmp" && mv "$f.tmp" "$f"
  printf 'set:%s\n' "$s"
done`

func TestHolderSocket_StatusCountsTurns(t *testing.T) {
	t.Setenv("HOME", shortTempDir(t))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	_, c, _ := startHolder(t, pidChild)

	waitForStatus(t, c, nativerunner.PidStatus{Status: nativerunner.ClaudeStarting}, 0)
	steps := []nativerunner.PidStatus{
		{Status: nativerunner.ClaudeBusy},
		{Status: nativerunner.ClaudeWaiting, WaitingFor: "input needed"},
		{Status: nativerunner.ClaudeBusy},
		{Status: nativerunner.ClaudeIdle},
	}
	for i, want := range steps {
		if err := c.SendKeys(want.Status + "\r"); err != nil {
			t.Fatal(err)
		}
		waitForStatus(t, c, want, i+1)
	}
}

func waitForStatus(t *testing.T, c *nativerunner.HolderClient, want nativerunner.PidStatus, turn int) {
	t.Helper()
	waitFor(t, fmt.Sprintf("status %+v at turn %d", want, turn), func() bool {
		got, n, err := c.Status()
		return err == nil && got != nil && *got == want && n == turn
	})
}
