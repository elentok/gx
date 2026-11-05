package prompt_test

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// plainSession starts a session with no ticket beside f's design session and
// returns its id and conversation id, Idle.
func plainSession(t *testing.T, f promptFixture) (id, conv string) {
	t.Helper()
	res, err := f.h.Client.Sessions.Create(f.ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id = res.Session.ID
	conv, _ = readMeta(t, f.h, id)["conversation_id"].(string)
	waitForHolder(t, f.h.SessionsDir+"/"+id)
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	return id, conv
}

func TestSessionPrompt_PromoteTypesTheSkillCommandAndLogsIt(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id, conv := plainSession(t, f)
	res, err := f.h.Client.Sessions.Prompt(f.ctx, wire.SessionPromptRequest{ID: id, Text: "ignored", Promote: true})
	if err != nil || res.Refused {
		t.Fatalf("promote = %+v, %v", res, err)
	}
	waitForSubmitted(t, conv, strconv.Quote("/gx-promote-to-design")+"\n")
	evs, ok, err := runlog.ReadServerEvents(f.h.TicketStore)
	if err != nil || !ok || len(evs) != 1 || evs[0].Action != "promote" || evs[0].Bytes != len("/gx-promote-to-design") {
		t.Errorf("server events = %+v, ok=%v, err=%v; want one promote", evs, ok, err)
	}
}

func TestSessionPrompt_PromoteOnCodexTypesADollarCommand(t *testing.T) {
	t.Parallel()
	f := startCodexPromptSession(t)
	res, err := f.h.Client.Sessions.Prompt(f.ctx, wire.SessionPromptRequest{ID: f.id, Promote: true})
	if err != nil || res.Refused {
		t.Fatalf("promote = %+v, %v", res, err)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("$gx-promote-to-design")+"\n")
}

func TestWebSessionPrompt_StalePromoteIsRefused(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	id, conv := plainSession(t, f)
	body := fmt.Sprintf(`{"id":%q,"promote":true,"last_message":"old"}`, id)
	code, out := servertest.WebPostJSON(t, f.base+"/v1/sessions/prompt", body, f.ck, "", servertest.JSONPost(f.base))
	if code != http.StatusOK || servertest.ReasonOf(t, out) != wire.ReasonStale {
		t.Errorf("stale promote = %d %s, want %s", code, out, wire.ReasonStale)
	}
	if got := submitted(t, conv); got != "" {
		t.Errorf("stale promote typed %q", got)
	}
}

func TestSessionPrompt_PromoteRefusals(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	h, row, release := rigs.StartQueueChat(t, "hello")
	defer release()
	for name, tc := range map[string]struct {
		client func() wire.SessionResult
		reason string
	}{
		"design session": {func() wire.SessionResult {
			return promptAs(t, f, wire.SessionPromptRequest{ID: f.id, Promote: true})
		}, wire.ReasonSessionNotPlain},
		"queue agent": {func() wire.SessionResult {
			res, err := h.Client.Sessions.Prompt(context.Background(), wire.SessionPromptRequest{ID: row.ID, Promote: true})
			if err != nil {
				t.Fatal(err)
			}
			return res
		}, wire.ReasonSessionUnknown},
		"continue and promote": {func() wire.SessionResult {
			return promptAs(t, f, wire.SessionPromptRequest{ID: f.id, Promote: true, Continue: true})
		}, wire.ReasonSessionBadRequest},
		"continue and promote on a plain session": {func() wire.SessionResult {
			id, _ := plainSession(t, f)
			return promptAs(t, f, wire.SessionPromptRequest{ID: id, Promote: true, Continue: true})
		}, wire.ReasonSessionBadRequest},
	} {
		if got := tc.client(); !got.Refused || got.Reason != tc.reason {
			t.Errorf("%s = %+v, want %s", name, got, tc.reason)
		}
	}
	if got := submitted(t, f.conv); got != "" {
		t.Errorf("a refused promote typed %q", got)
	}
}

func promptAs(t *testing.T, f promptFixture, req wire.SessionPromptRequest) wire.SessionResult {
	t.Helper()
	res, err := f.h.Client.Sessions.Prompt(f.ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
