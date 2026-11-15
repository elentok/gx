package sessions_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/server/wire"
)

func TestSessionInfoModel_ShownModelFollowsTheSwitchTime(t *testing.T) {
	t.Parallel()
	switchedAt := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	older, newer := switchedAt.Add(-time.Minute).Format(time.RFC3339), switchedAt.Add(time.Minute).Format(time.RFC3339)
	claudeReply := func(at string) string {
		return `{"type":"assistant","timestamp":"` + at + `","message":{"model":"claude-opus-5-5","usage":{"input_tokens":10}}}`
	}
	codexTurn := func(at string) string {
		return `{"type":"turn_context","timestamp":"` + at + `","payload":{"model":"gpt-6.1-sol","effort":"low"}}`
	}
	writeRollout := func(t *testing.T, line string) string {
		t.Helper()
		home := t.TempDir()
		dir := filepath.Join(home, "sessions", "2026", "10", "30")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "rollout-conv.jsonl"), []byte(line+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return home
	}
	claudeMeta := sessions.Meta{Harness: "claude", Dir: "/work/proj", ConversationID: "conv", Tier: "worker", Model: "claude-sonnet-5-5", Effort: "high", SwitchedAt: switchedAt}
	codexMeta := sessions.Meta{Harness: "codex", ConversationID: "conv", Tier: "worker", Model: "gpt-6.1-terra", Effort: "high", SwitchedAt: switchedAt}

	for name, tc := range map[string]struct {
		info                  func(t *testing.T) wire.SessionInfo
		wantModel, wantEffort string
	}{
		"claude before a newer turn shows the record": {func(t *testing.T) wire.SessionInfo {
			return modelTestServer(t, "/work/proj", claudeReply(older)).SessionInfo(claudeMeta)
		}, "claude-sonnet-5-5", "high"},
		"claude after a newer turn shows the reply and the record's effort": {func(t *testing.T) wire.SessionInfo {
			return modelTestServer(t, "/work/proj", claudeReply(newer)).SessionInfo(claudeMeta)
		}, "claude-opus-5-5", "high"},
		"codex before a newer turn shows the record": {func(t *testing.T) wire.SessionInfo {
			return infoManager(nil, writeRollout(t, codexTurn(older))).SessionInfo(codexMeta)
		}, "gpt-6.1-terra", "high"},
		"codex after a newer turn shows the reported model and effort": {func(t *testing.T) wire.SessionInfo {
			return infoManager(nil, writeRollout(t, codexTurn(newer))).SessionInfo(codexMeta)
		}, "gpt-6.1-sol", "low"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			info := tc.info(t)
			if info.Model != tc.wantModel || info.Effort != tc.wantEffort {
				t.Errorf("model/effort = %q/%q, want %s/%s", info.Model, info.Effort, tc.wantModel, tc.wantEffort)
			}
		})
	}

	// A switch back to the tier's model shows no flag before the next turn.
	back := claudeMeta
	back.Model = "claude-opus-5-5"
	if modelTestServer(t, "/work/proj", claudeReply(older)).SessionInfo(back).ModelSwitched {
		t.Error("switch back to the tier model: ModelSwitched = true, want false")
	}
}
