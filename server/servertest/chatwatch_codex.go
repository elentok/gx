package servertest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/wire"
)

// CodexEventWithin is ChatEventWithin for a Codex rollout, which always grows.
func CodexEventWithin(evs <-chan wire.Event, t *testing.T, path, id string, d time.Duration) bool {
	t.Helper()
	deadline := time.After(d)
	tick := time.NewTicker(ChatPollTestInterval)
	defer tick.Stop()
	for {
		select {
		case ev := <-evs:
			if ev.Type == wire.EventSessionChatChanged && ev.Address == id {
				return true
			}
		case <-tick.C:
			AppendLines(t, path, CodexMessageLine)
		case <-deadline:
			return false
		}
	}
}

// StoppedCodexSession is an Exited session whose meta says it ran on Codex.
func StoppedCodexSession(t *testing.T, opts ...func(*server.Config)) (h *Harness, ctx context.Context, id, conv, dir string) {
	t.Helper()
	h, ctx, evs, id, conv := StartFakeSession(t, opts...)
	StopFakeSession(ctx, t, h, evs, id)
	meta := ReadMeta(t, h, id)
	meta["harness"] = "codex"
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.SessionsDir, id, "meta.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	dir, _ = meta["dir"].(string)
	return h, ctx, id, conv, dir
}

// WriteCodexRollout gives conv a rollout under codexHome, as Codex writes one for a session in dir.
func WriteCodexRollout(t *testing.T, codexHome, conv, dir string) {
	t.Helper()
	rollouts := filepath.Join(codexHome, "sessions", "2026", "10", "30")
	if err := os.MkdirAll(rollouts, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"session_meta","payload":{"id":` + strconv.Quote(conv) + `,"cwd":` + strconv.Quote(dir) + `}}` + "\n"
	if err := os.WriteFile(filepath.Join(rollouts, "rollout-2026-10-30T00-00-00-"+conv+".jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}
