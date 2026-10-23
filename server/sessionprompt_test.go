package server_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
)

// promptFixture is a design session on a fake program with an input box,
// Idle, whose transcript lives under cfg.
type promptFixture struct {
	designsFixture
	id, conv, transcript string
}

func startPromptSession(t *testing.T) promptFixture {
	t.Helper()
	t.Setenv(agentfake.ProgramInputEnv, "1")
	cfg := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfg)
	f := startDesigns(t)
	res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind), Ticket: "proj:map"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, f.h, id)["conversation_id"].(string)
	waitForHolder(t, filepath.Join(f.h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionIdle })
	return promptFixture{designsFixture: f, id: id, conv: conv, transcript: filepath.Join(cfg, "projects", "-repo", conv+".jsonl")}
}

func (f promptFixture) prompt(t *testing.T, text string) server.SessionResult {
	t.Helper()
	res, err := f.h.Client.PromptSession(f.ctx, server.SessionPromptRequest{ID: f.id, Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSessionPrompt_TypesMultiLineTextAsOnePrompt(t *testing.T) {
	f := startPromptSession(t)
	if res := f.prompt(t, "line one\r\nline two\n\nline four"); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("line one\nline two\n\nline four")+"\n")
}

func TestSessionPrompt_Refusals(t *testing.T) {
	f := startPromptSession(t)
	for name, tc := range map[string]struct{ text, want string }{
		"empty":    {"  \n", server.ReasonEmptyPrompt},
		"slash":    {"/clear", server.ReasonPrefixNotAllowed},
		"bang":     {" !ls", server.ReasonPrefixNotAllowed},
		"too long": {strings.Repeat("a", 32*1024+1), server.ReasonTextTooLong},
	} {
		if res := f.prompt(t, tc.text); !res.Refused || res.Reason != tc.want {
			t.Errorf("%s: prompt = %+v, want reason %q", name, res, tc.want)
		}
	}
}

func TestSessionPrompt_NotIdle(t *testing.T) {
	f := startPromptSession(t)
	report(t, f.conv, agentfake.Control{State: "working"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionWorking })
	if res := f.prompt(t, "hello"); !res.Refused || res.Reason != server.ReasonSessionNotIdle {
		t.Errorf("prompt while working = %+v, want %s", res, server.ReasonSessionNotIdle)
	}
}

func TestSessionPrompt_NotADesignSession(t *testing.T) {
	f := startPromptSession(t)
	res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	got, err := f.h.Client.PromptSession(f.ctx, server.SessionPromptRequest{ID: res.Session.ID, Text: "hello"})
	if err != nil || !got.Refused || got.Reason != server.ReasonNotDesignSession {
		t.Errorf("prompt = %+v, %v; want %s", got, err, server.ReasonNotDesignSession)
	}
}

func TestSessionPrompt_HarnessWithoutChat(t *testing.T) {
	f := startPromptSession(t)
	res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.NoResumeHarnessKind), Ticket: "proj:map"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	got, err := f.h.Client.PromptSession(f.ctx, server.SessionPromptRequest{ID: res.Session.ID, Text: "hello"})
	if err != nil || !got.Refused || got.Reason != server.ReasonHarnessNotSupported {
		t.Errorf("prompt = %+v, %v; want %s", got, err, server.ReasonHarnessNotSupported)
	}
}

func TestSessionPrompt_LogsOneEventWithoutTheText(t *testing.T) {
	f := startPromptSession(t)
	const text = "secret plan\nsecond line"
	if res := f.prompt(t, text); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	evs, ok, err := ralphloop.ReadServerEvents(f.h.TicketStore)
	if err != nil || !ok || len(evs) != 1 {
		t.Fatalf("server events = %+v, ok=%v, err=%v; want one", evs, ok, err)
	}
	ev := evs[0]
	if ev.Type != "session-prompted" || ev.Session != f.id || ev.Source != "cli" || ev.Action != "text" || ev.Bytes != len(text) {
		t.Errorf("event = %+v", ev)
	}
	raw, err := os.ReadFile(filepath.Join(f.h.TicketStore, "server-log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") {
		t.Errorf("server log holds the text: %s", raw)
	}
}

func TestWebSessionPrompt_StaleWhenTheTranscriptMoved(t *testing.T) {
	f := startPromptSession(t)
	writeTranscript(t, f.transcript,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"Which store?"}]}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-01-01T10:00:01Z","message":{"content":[{"type":"text","text":"Or files?"}]}}`,
	)
	post := func(last string) (int, string) {
		body := fmt.Sprintf(`{"id":%q,"text":"SQLite","last_message":%q}`, f.id, last)
		return webPostJSON(t, f.base+"/v1/sessions/prompt", body, f.ck, "", jsonPost(f.base))
	}
	if code, out := post("a1"); code != http.StatusOK || reasonOf(t, out) != server.ReasonStale {
		t.Errorf("old message id = %d %s, want stale", code, out)
	}
	if code, out := post("a2"); code != http.StatusOK || reasonOf(t, out) != "" {
		t.Errorf("current message id = %d %s, want sent", code, out)
	}
	waitForSubmitted(t, f.conv, "\"SQLite\"\n")
}

func (f promptFixture) cont(t *testing.T) server.SessionResult {
	t.Helper()
	res, err := f.h.Client.PromptSession(f.ctx, server.SessionPromptRequest{ID: f.id, Continue: true})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSessionPrompt_ContinueTypesClearThenGxDesign(t *testing.T) {
	f := startPromptSession(t)
	if res := f.cont(t); res.Refused {
		t.Fatalf("continue = %+v", res)
	}
	waitForSubmitted(t, f.conv, "\"/clear\"\n\"/gx-design map\"\n")
	evs, ok, err := ralphloop.ReadServerEvents(f.h.TicketStore)
	if err != nil || !ok || len(evs) != 1 || evs[0].Action != "continue" || evs[0].Bytes != len("/clear")+len("/gx-design map") {
		t.Errorf("server events = %+v, ok=%v, err=%v; want one continue", evs, ok, err)
	}
}

func TestSessionPrompt_ContinueRefusedOnANonDesignSession(t *testing.T) {
	f := startPromptSession(t)
	res, err := f.h.Client.CreateSession(f.ctx, server.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	got, err := f.h.Client.PromptSession(f.ctx, server.SessionPromptRequest{ID: res.Session.ID, Continue: true})
	if err != nil || !got.Refused || got.Reason != server.ReasonNotDesignSession {
		t.Errorf("continue = %+v, %v; want %s", got, err, server.ReasonNotDesignSession)
	}
}

func TestWebSessionPrompt_StaleContinueIsRefused(t *testing.T) {
	f := startPromptSession(t)
	writeTranscript(t, f.transcript,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"Which store?"}]}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-01-01T10:00:01Z","message":{"content":[{"type":"text","text":"Or files?"}]}}`,
	)
	post := func(last string) (int, string) {
		body := fmt.Sprintf(`{"id":%q,"continue":true,"last_message":%q}`, f.id, last)
		return webPostJSON(t, f.base+"/v1/sessions/prompt", body, f.ck, "", jsonPost(f.base))
	}
	if code, out := post("a1"); code != http.StatusOK || reasonOf(t, out) != server.ReasonStale {
		t.Errorf("old message id = %d %s, want stale", code, out)
	}
	if got := submitted(t, f.conv); got != "" {
		t.Errorf("stale continue typed %q", got)
	}
	if code, out := post("a2"); code != http.StatusOK || reasonOf(t, out) != "" {
		t.Errorf("current message id = %d %s, want sent", code, out)
	}
	waitForSubmitted(t, f.conv, "\"/clear\"\n\"/gx-design map\"\n")
}

func TestWebSessionPrompt_KeepsTheWriteChecks(t *testing.T) {
	f := startPromptSession(t)
	url := f.base + "/v1/sessions/prompt"
	body := fmt.Sprintf(`{"id":%q,"text":"hi"}`, f.id)
	if code, _ := webPostJSON(t, url, body, nil, "", jsonPost(f.base)); code != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", code)
	}
	if code, _ := webPostJSON(t, url, body, f.ck, "", map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}); code != http.StatusForbidden {
		t.Errorf("foreign Origin = %d, want 403", code)
	}
	if code, _ := webPostJSON(t, url, body, f.ck, "", map[string]string{"Origin": f.base, "Content-Type": "text/plain"}); code != http.StatusForbidden {
		t.Errorf("text/plain = %d, want 403", code)
	}
}
