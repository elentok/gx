package prompt_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// retry posts the web Retry of the session's blocking card.
func (f promptFixture) retry(t *testing.T) (int, string) {
	t.Helper()
	body := fmt.Sprintf(`{"id":%q}`, f.id)
	return servertest.WebPostJSON(t, f.base+"/v1/sessions/run-action", body, f.ck, "", servertest.JSONPost(f.base))
}

// seedActionable writes a card into the session's meta and waits for the
// server to show it.
func (f promptFixture) seedActionable(t *testing.T, a wire.SessionActionable) {
	t.Helper()
	meta := servertest.ReadMeta(t, f.h, f.id)
	meta["actionable"] = a
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.h.SessionsDir, f.id, "meta.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Actionable.Reason == a.Reason })
}

func TestRetry(t *testing.T) {
	t.Parallel()
	for _, blocking := range []string{wire.BlockingDraft, wire.BlockingStarting} {
		t.Run("clears a typed draft then sends the unsent prompt: "+blocking, func(t *testing.T) {
			t.Parallel()
			f := startPromptSession(t)
			f.seedActionable(t, wire.SessionActionable{Reason: "card", Action: []string{"the first prompt"}, GxRaised: true, Blocking: blocking})
			if res := f.keys(t, wire.SessionKeysRequest{Text: "typed meanwhile"}); res.Refused {
				t.Fatalf("keys = %+v", res)
			}
			// The keys return before the TUI redraws; a retry that read the old frame sees no draft.
			servertest.WaitForScreen(t, servertest.WaitForHolder(t, filepath.Join(f.h.SessionsDir, f.id)), "typed meanwhile")
			if code, body := f.retry(t); code != http.StatusOK || strings.Contains(body, `"refused":true`) {
				t.Fatalf("retry = %d %s", code, body)
			}
			waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Actionable.Reason == "" })
			servertest.WaitForSubmitted(t, f.conv, "\"the first prompt\"\n")
			if got := servertest.Submitted(t, f.conv); strings.Contains(got, "typed meanwhile") {
				t.Errorf("submitted = %q, want the draft cleared first", got)
			}
		})
	}
	t.Run("refuses a card that is not blocking", func(t *testing.T) {
		t.Parallel()
		f := startPromptSession(t)
		f.seedActionable(t, wire.SessionActionable{Kind: wire.DeliverySendFailed, Reason: "x", Action: []string{"p"}, GxRaised: true})
		if _, body := f.retry(t); !strings.Contains(body, wire.ReasonSessionNoAction) {
			t.Errorf("retry body = %s, want the no-action refusal", body)
		}
		if got := servertest.Submitted(t, f.conv); got != "" {
			t.Errorf("submitted = %q, want nothing typed", got)
		}
	})
	t.Run("a draft that will not clear raises the blocking card again", func(t *testing.T) {
		t.Parallel()
		f := fromPrompt(rigs.StartPromptSessionAs(t, firstPromptGrace, wire.CreateSessionRequest{Name: "old"}, servertest.WithEnv(agentfake.ProgramDraftEnv+"=my draft", agentfake.ProgramClearNeverEnv+"=1")))
		f.seedActionable(t, wire.SessionActionable{Reason: "card", Action: []string{"the prompt"}})
		if res, err := f.h.Client.Sessions.RunAction(f.ctx, f.id); err != nil || res.Refused {
			t.Fatalf("run-action = %+v, %v", res, err)
		}
		draftCard := func(s wire.SessionInfo) bool {
			return s.Actionable.Blocking == wire.BlockingDraft && slices.Equal(s.Actionable.Action, []string{"the prompt"})
		}
		waitForSession(f.ctx, t, f.h, f.evs, f.id, draftCard)
		f.retry(t)
		waitForSession(f.ctx, t, f.h, f.evs, f.id, draftCard)
		logs, _ := os.ReadFile(server.LogPath(f.h.StateDir))
		if n := strings.Count(string(logs), "draft would not clear"); n < 2 {
			t.Errorf("clear attempts = %d, want one for the run and one for Retry:\n%s", n, logs)
		}
	})
}
