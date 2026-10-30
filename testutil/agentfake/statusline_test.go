package agentfake

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// statusLineFake starts a fake whose statusLine command copies its stdin to the
// returned file, in a worktree carrying cfg.
func statusLineFake(t *testing.T, command string, cfg WorkerConfig) (*tuiFake, string) {
	t.Helper()
	out := filepath.Join(t.TempDir(), "payload.json")
	settings, err := json.Marshal(map[string]any{
		"statusLine": map[string]any{"type": "command", "command": command + " " + out},
	})
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"--session-id", "sess-1", "--settings", string(settings)}
	f := startTUIFake(t, args)
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.cmd.Dir, WorkerConfigFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	f.waitStatus(StatusIdle, "")
	return f, out
}

func readPayload(t *testing.T, f *tuiFake, path string) map[string]any {
	t.Helper()
	var payload map[string]any
	f.waitFor("statusLine payload", func() bool {
		data, err := os.ReadFile(path)
		return err == nil && json.Unmarshal(data, &payload) == nil
	})
	return payload
}

func TestTUIStatusLineGetsConfiguredUsage(t *testing.T) {
	usage := &UsageConfig{
		FiveHour: UsageWindow{UsedPercentage: 42.5, ResetsAt: 1900000000},
		SevenDay: UsageWindow{UsedPercentage: 7, ResetsAt: 1900100000},
	}
	f, out := statusLineFake(t, "cat >", WorkerConfig{Usage: usage})
	f.control(Control{Action: ActionFinish})

	limits, _ := readPayload(t, f, out)["rate_limits"].(map[string]any)
	five, _ := limits["five_hour"].(map[string]any)
	week, _ := limits["seven_day"].(map[string]any)
	if five["used_percentage"] != 42.5 || five["resets_at"] != float64(1900000000) {
		t.Errorf("five_hour = %v", five)
	}
	if week["used_percentage"] != float64(7) || week["resets_at"] != float64(1900100000) {
		t.Errorf("seven_day = %v", week)
	}
}

func TestTUIStatusLineWithoutUsageHasNoRateLimits(t *testing.T) {
	f, out := statusLineFake(t, "cat >", WorkerConfig{})
	f.control(Control{Action: ActionFinish})

	if _, ok := readPayload(t, f, out)["rate_limits"]; ok {
		t.Error("payload has rate_limits with no usage configured")
	}
}

func TestTUIStatusLineFailureDoesNotBreakTurn(t *testing.T) {
	f, _ := statusLineFake(t, "exit 3 #", WorkerConfig{})
	f.control(Control{Action: ActionFinish})
	f.control(Control{Action: ActionFinish})
	f.waitStatus(StatusIdle, "")
}
