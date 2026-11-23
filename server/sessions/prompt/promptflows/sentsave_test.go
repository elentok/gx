package promptflows_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/drafthistory"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

// webPost sends body to a web route of the session's server.
func (f promptFixture) webPost(t *testing.T, route string, body any) wire.SessionResult {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	_, out := servertest.WebPostJSON(t, f.base+route, string(b), f.ck, "", servertest.JSONPost(f.base))
	var res wire.SessionResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("%s = %s: %v", route, out, err)
	}
	return res
}

func (f promptFixture) webPrompt(t *testing.T, text string) wire.SessionResult {
	t.Helper()
	return f.webPost(t, "/v1/sessions/prompt", wire.SessionPromptRequest{ID: f.id, Text: text})
}

func (f promptFixture) sent(t *testing.T) []drafthistory.Item {
	t.Helper()
	items, err := drafthistory.New(f.h.StateDir).List(drafthistory.Sent)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

// waitSent waits for the Sent messages list to hold want entries and returns it.
func (f promptFixture) waitSent(t *testing.T, want int) []drafthistory.Item {
	t.Helper()
	servertest.WaitFor(t, 10*time.Second, fmt.Sprintf("%d sent entries", want), func() bool { return len(f.sent(t)) == want })
	return f.sent(t)
}

func TestSentSave_WebSendsSaveForChatAndDesignOnly(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		create wire.CreateSessionRequest
		want   *drafthistory.Entry
	}{
		"design": {wire.CreateSessionRequest{Ticket: "proj:map"}, &drafthistory.Entry{Text: "hello", Project: "proj", Slug: "map", Composer: drafthistory.ComposerDesign}},
		"chat":   {wire.CreateSessionRequest{Name: "my chat"}, &drafthistory.Entry{Text: "hello", Project: "proj", Name: "my chat", Harness: string(agentfake.HarnessKind), Composer: drafthistory.ComposerChat}},
		"ticket": {wire.CreateSessionRequest{Ticket: "proj:plain"}, nil},
		"fix":    {wire.CreateSessionRequest{FixOf: "proj:plain/02"}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			f := fromPrompt(rigs.StartPromptSessionAs(t, firstPromptGrace, tc.create))
			if res := f.webPrompt(t, "hello"); res.Refused {
				t.Fatalf("prompt = %+v", res)
			}
			waitForSubmitted(t, f.conv, "\"hello\"\n")
			got := f.sent(t)
			if tc.want == nil {
				if len(got) != 0 {
					t.Fatalf("entries = %+v, want none", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("entries = %+v, want one", got)
			}
			e := got[0].Entry
			if e.Text != tc.want.Text || e.Project != tc.want.Project || e.Slug != tc.want.Slug || e.Name != tc.want.Name ||
				e.Composer != tc.want.Composer || (tc.want.Harness != "" && e.Harness == "") {
				t.Errorf("entry = %+v, want %+v", e, *tc.want)
			}
		})
	}
}

func TestSentSave_CliContinuePromoteAndRefusalsSaveNothing(t *testing.T) {
	t.Parallel()
	f := fromPrompt(rigs.StartPromptSessionAs(t, firstPromptGrace, wire.CreateSessionRequest{Name: "my chat"}))
	if res := f.prompt(t, "from the cli"); res.Refused {
		t.Fatalf("cli prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, "\"from the cli\"\n")
	for name, text := range map[string]string{"empty": "  \n", "bang": " !ls"} {
		if res := f.webPrompt(t, text); !res.Refused {
			t.Errorf("%s: web prompt = %+v, want refused", name, res)
		}
	}
	if res := f.webPost(t, "/v1/sessions/prompt", wire.SessionPromptRequest{ID: f.id, Promote: true}); res.Refused {
		t.Fatalf("promote = %+v", res)
	}
	if got := f.sent(t); len(got) != 0 {
		t.Errorf("entries = %+v, want none", got)
	}
	d := startPromptSession(t)
	if res := d.webPost(t, "/v1/sessions/prompt", wire.SessionPromptRequest{ID: d.id, Continue: true}); res.Refused {
		t.Fatalf("continue = %+v", res)
	}
	if got := d.sent(t); len(got) != 0 {
		t.Errorf("continue: entries = %+v, want none", got)
	}
}

func TestSentSave_QueueAgentIsRefusedWithNoEntry(t *testing.T) {
	t.Parallel()
	h, agent, stop := rigs.StartQueueChat(t, "working")
	defer stop()
	f := promptFixture{designsFixture: designsFixture{h: h}, id: agent.ID}
	if res, err := h.Client.Sessions.Prompt(context.Background(), wire.SessionPromptRequest{ID: agent.ID, Text: "hi"}); err != nil || !res.Refused {
		t.Fatalf("prompt = %+v, %v, want refused", res, err)
	}
	if got := f.sent(t); len(got) != 0 {
		t.Errorf("entries = %+v, want none", got)
	}
}

func TestSentSave_AcceptedSendThatFailsKeepsItsEntry(t *testing.T) {
	t.Parallel()
	f := fromPrompt(rigs.StartCodexPromptSession(t))
	if res := f.webPrompt(t, "/handoff"); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Actionable.GxRaised })
	if got := f.sent(t); len(got) != 1 || got[0].Text != "/handoff" {
		t.Errorf("entries = %+v, want the failed send", got)
	}
}

func TestSentSave_RoundAnswerSavesTheNoteOnce(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	writeRounds(t, f.transcript, 1)
	chat, err := f.h.Client.Sessions.Chat(f.ctx, f.id)
	if err != nil || len(chat.Questions) != 1 {
		t.Fatalf("chat = %+v, %v, want one card", chat, err)
	}
	answer := func(note string) wire.SessionResult {
		return f.webPost(t, "/v1/sessions/answer", wire.SessionAnswerRequest{
			ID: f.id, Source: wire.CardSourceRound, CardID: chat.Questions[0].ID, LastMessage: "round-1",
			Answers: []wire.SessionAnswer{{Choice: "1", Text: "the first"}}, Note: note,
		})
	}
	if res := answer("  "); res.Refused {
		t.Fatalf("answer without a note = %+v", res)
	}
	servertest.WaitFor(t, 10*time.Second, "the round text submitted", func() bool { return submitted(t, f.conv) != "" })
	report(t, f.conv, agentfake.Control{State: "working"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	if got := f.sent(t); len(got) != 0 {
		t.Fatalf("no note: entries = %+v, want none", got)
	}
	report(t, f.conv, agentfake.Control{State: "idle"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	if res := answer("also, mind the cache"); res.Refused {
		t.Fatalf("answer with a note = %+v", res)
	}
	got := f.waitSent(t, 1)
	if got[0].Text != "also, mind the cache" || strings.Contains(got[0].Text, "Q1") {
		t.Errorf("text = %q, want the typed note only", got[0].Text)
	}
}
