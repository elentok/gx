package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/nativerunner"
)

const usagePayload = `{"model":{"display_name":"Claude"},"rate_limits":{"five_hour":{"used_percentage":11.5,"resets_at":1790000000},"seven_day":{"used_percentage":72,"resets_at":1790500000}}}`

// runStatuslineCmd runs `claude statusline args...` on payload and returns the
// rendered output and the command error.
func runStatuslineCmd(t *testing.T, payload string, args ...string) (string, error) {
	t.Helper()
	out := &strings.Builder{}
	d := deps{
		stdin:  strings.NewReader(payload),
		stdout: out,
		getwd:  func() (string, error) { return "/nonexistent", nil },
	}
	cmd := newClaudeStatuslineCmd(d)
	cmd.SetArgs(args)
	cmd.SilenceUsage, cmd.SilenceErrors = true, true
	err := cmd.Execute()
	return out.String(), err
}

func TestClaudeStatusline_AgentDirWritesUsageRecord(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	before := time.Now().Add(-time.Second)
	withFlag, err := runStatuslineCmd(t, usagePayload, "--agent-dir", dir)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := runStatuslineCmd(t, usagePayload)
	if err != nil {
		t.Fatal(err)
	}
	if withFlag != plain {
		t.Errorf("output with flag = %q, want %q", withFlag, plain)
	}

	data, err := os.ReadFile(filepath.Join(dir, nativerunner.UsageFile))
	if err != nil {
		t.Fatal(err)
	}
	var rec nativerunner.UsageRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.FiveHour == nil || *rec.FiveHour != (nativerunner.UsageWindow{UsedPercentage: 11.5, ResetsAt: 1790000000}) {
		t.Errorf("five_hour = %+v", rec.FiveHour)
	}
	if rec.SevenDay == nil || *rec.SevenDay != (nativerunner.UsageWindow{UsedPercentage: 72, ResetsAt: 1790500000}) {
		t.Errorf("seven_day = %+v", rec.SevenDay)
	}
	if rec.SeenAt.Before(before) || rec.SeenAt.After(time.Now()) {
		t.Errorf("seen_at = %v, want about now", rec.SeenAt)
	}
}

func TestClaudeStatusline_UnwritableAgentDirKeepsLineAndExitCode(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "gone")
	got, err := runStatuslineCmd(t, usagePayload, "--agent-dir", missing)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	want, _ := runStatuslineCmd(t, usagePayload)
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestClaudeStatusline_NoRateLimitsWritesNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	older := []byte(`{"seen_at":"2026-01-01T00:00:00Z"}`)
	path := filepath.Join(dir, nativerunner.UsageFile)
	if err := os.WriteFile(path, older, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runStatuslineCmd(t, `{"model":{"display_name":"Claude"}}`, "--agent-dir", dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(older) {
		t.Errorf("record = %q, %v; want the older record untouched", got, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want only the older record", len(entries))
	}
}
