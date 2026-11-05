package prompt_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/servertest"
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
	return startPromptSessionWithGrace(t, firstPromptGrace)
}

// startPromptSessionWithGrace is startPromptSession with a longer wait for a
// typed prompt's turn to start, for a test that must act inside it.
func startPromptSessionWithGrace(t *testing.T, grace time.Duration) promptFixture {
	t.Helper()
	return fromPrompt(servertest.StartPromptSession(t, grace))
}

// fixture is the servertest fixture the shared methods run on.
func (f promptFixture) fixture() servertest.PromptFixture {
	return servertest.PromptFixture{DesignsFixture: f.shared(), ID: f.id, Conv: f.conv, Transcript: f.transcript}
}

func fromPrompt(p servertest.PromptFixture) promptFixture {
	return promptFixture{designsFixture: fromShared(p.DesignsFixture), id: p.ID, conv: p.Conv, transcript: p.Transcript}
}

func (f promptFixture) prompt(t *testing.T, text string) wire.SessionResult {
	t.Helper()
	return f.fixture().Prompt(t, text)
}

func TestSessionPrompt_TypesMultiLineTextAsOnePrompt(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	if res := f.prompt(t, "line one\r\nline two\n\nline four"); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("line one\nline two\n\nline four")+"\n")
}

func TestSessionPrompt_SwallowedEnterIsNudged(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	if err := agentfake.SendProgramControl(f.conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
		t.Fatal(err)
	}
	if res := f.prompt(t, "yes to both"); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("yes to both")+"\n")
	report(t, f.conv, agentfake.Control{State: "working"})
	time.Sleep(2 * firstPromptGrace) // negative wait: a repeat Enter would come within the grace window
	if got := submitted(t, f.conv); got != strconv.Quote("yes to both")+"\n" {
		t.Errorf("submitted = %q, want exactly one submission", got)
	}
}

func TestSessionPrompt_Refusals(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	for name, tc := range map[string]struct{ text, want string }{
		"empty":    {"  \n", wire.ReasonEmptyPrompt},
		"bang":     {" !ls", wire.ReasonPromptPrefixNotAllowed},
		"too long": {strings.Repeat("a", 32*1024+1), wire.ReasonPromptTextTooLong},
	} {
		res := f.prompt(t, tc.text)
		if !res.Refused || res.Reason != tc.want {
			t.Errorf("%s: prompt = %+v, want reason %q", name, res, tc.want)
		}
		if tc.want == wire.ReasonPromptPrefixNotAllowed && (!strings.Contains(res.Message, "!") || strings.Contains(res.Message, "/")) {
			t.Errorf("%s: message %q should mention only !", name, res.Message)
		}
	}
}

func TestSessionPrompt_AcceptsSlashCommands(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"/cost", "/clear", "/compact foo"} {
		t.Run(text, func(t *testing.T) {
			f := startPromptSession(t)
			if res := f.prompt(t, text); res.Refused {
				t.Fatalf("prompt %q = %+v, want accepted", text, res)
			}
			waitForSubmitted(t, f.conv, strconv.Quote(text)+"\n")
		})
	}
}

func TestSessionPrompt_SlashCommandIsTypedOnce(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	if err := agentfake.SendProgramControl(f.conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
		t.Fatal(err)
	}
	if res := f.prompt(t, "/cost"); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	time.Sleep(2 * firstPromptGrace) // negative wait: a nudge's Enter would come within the grace window
	if got := submitted(t, f.conv); got != "" {
		t.Errorf("submitted = %q, want none: the one Enter was swallowed and not retried", got)
	}
}

func TestSessionPrompt_CompactWithArgumentsIsNudged(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	if err := agentfake.SendProgramControl(f.conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
		t.Fatal(err)
	}
	if res := f.prompt(t, "/compact foo"); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("/compact foo")+"\n")
	report(t, f.conv, agentfake.Control{State: "working"})
	time.Sleep(2 * firstPromptGrace)
	if got := submitted(t, f.conv); got != strconv.Quote("/compact foo")+"\n" {
		t.Errorf("submitted = %q, want one submission once the turn started", got)
	}
}

func TestSessionPrompt_DesignSlashCommandStopsTheCountdown(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"/clear", "/compact"} {
		t.Run(text, func(t *testing.T) {
			f := startAdvancing(t, storeWithMapStep(t, "spec"))
			waitForHolder(t, filepath.Join(f.H.SessionsDir, f.ID))
			writeAssistantMessage(t, f.Path, runEndText)
			endTurn(t, f)
			waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt != nil })
			res, err := f.H.Client.Sessions.Prompt(f.Ctx, wire.SessionPromptRequest{ID: f.ID, Text: text})
			if err != nil || res.Refused {
				t.Fatalf("prompt = %+v, %v", res, err)
			}
			waitUntil(t, "the countdown stopped", func() bool { return sessionSnapshot(t, f).AutoAdvanceAt == nil })
			time.Sleep(2 * countdownDelay) // negative wait: a surviving countdown would send Continue within this window
			// As typed text does, the command stops only the countdown: the
			// run-end Continue stays raised for the person to press.
			if got := sessionSnapshot(t, f); got.AutoAdvanceAt != nil || got.Actionable.Reason == "" || autoSends(t, f) != 0 {
				t.Errorf("session = %+v, want no countdown, Continue still raised, no auto send", got)
			}
		})
	}
}

func TestSessionPrompt_ASlashCommandThatStartsNoTurnRaisesNothing(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	if res := f.prompt(t, "/cost"); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("/cost")+"\n")
	time.Sleep(2 * firstPromptGrace) // negative wait: a failure would raise within this window
	got := sessionSnapshotOf(t, f.designsFixture, f.id)
	if got.Actionable.Reason != "" {
		t.Errorf("actionable = %+v, want none", got.Actionable)
	}
}

func TestSessionPrompt_NotIdle(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	report(t, f.conv, agentfake.Control{State: "working"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking })
	if res := f.prompt(t, "hello"); !res.Refused || res.Reason != wire.ReasonSessionNotIdle {
		t.Errorf("prompt while working = %+v, want %s", res, wire.ReasonSessionNotIdle)
	}
}

func TestSessionPrompt_PlainSessionAcceptsText(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	res, err := f.h.Client.CreateSession(f.ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	id := res.Session.ID
	conv, _ := readMeta(t, f.h, id)["conversation_id"].(string)
	waitForHolder(t, filepath.Join(f.h.SessionsDir, id))
	report(t, conv, agentfake.Control{State: "idle"})
	waitForSession(f.ctx, t, f.h, f.evs, id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionIdle })
	got, err := f.h.Client.Sessions.Prompt(f.ctx, wire.SessionPromptRequest{ID: id, Text: "hello"})
	if err != nil || got.Refused {
		t.Fatalf("prompt = %+v, %v; want accepted", got, err)
	}
	waitForSubmitted(t, conv, strconv.Quote("hello")+"\n")
}

func TestSessionPrompt_HarnessWithoutChat(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	res, err := f.h.Client.CreateSession(f.ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.NoResumeHarnessKind), Ticket: "proj:map"})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	got, err := f.h.Client.Sessions.Prompt(f.ctx, wire.SessionPromptRequest{ID: res.Session.ID, Text: "hello"})
	if err != nil || !got.Refused || got.Reason != wire.ReasonPromptHarnessNotSupported {
		t.Errorf("prompt = %+v, %v; want %s", got, err, wire.ReasonPromptHarnessNotSupported)
	}
}

func TestSessionPrompt_LogsOneEventWithoutTheText(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	f := startPromptSession(t)
	writeTranscript(t, f.transcript,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"Which store?"}]}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-01-01T10:00:01Z","message":{"content":[{"type":"text","text":"Or files?"}]}}`,
	)
	post := func(last string) (int, string) {
		body := fmt.Sprintf(`{"id":%q,"text":"SQLite","last_message":%q}`, f.id, last)
		return servertest.WebPostJSON(t, f.base+"/v1/sessions/prompt", body, f.ck, "", servertest.JSONPost(f.base))
	}
	if code, out := post("a1"); code != http.StatusOK || servertest.ReasonOf(t, out) != wire.ReasonStale {
		t.Errorf("old message id = %d %s, want stale", code, out)
	}
	if code, out := post("a2"); code != http.StatusOK || servertest.ReasonOf(t, out) != "" {
		t.Errorf("current message id = %d %s, want sent", code, out)
	}
	waitForSubmitted(t, f.conv, "\"SQLite\"\n")
}

func (f promptFixture) cont(t *testing.T) wire.SessionResult {
	t.Helper()
	res, err := f.h.Client.Sessions.Prompt(f.ctx, wire.SessionPromptRequest{ID: f.id, Continue: true})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestSessionPrompt_ContinueTypesClearThenGxDesign(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	f := startPromptSession(t)
	res, err := f.h.Client.CreateSession(f.ctx, wire.CreateSessionRequest{Project: "proj", Dir: t.TempDir(), Harness: string(agentfake.HarnessKind)})
	if err != nil || res.Session == nil {
		t.Fatalf("create = %+v, %v", res, err)
	}
	got, err := f.h.Client.Sessions.Prompt(f.ctx, wire.SessionPromptRequest{ID: res.Session.ID, Continue: true})
	if err != nil || !got.Refused || got.Reason != wire.ReasonSessionNotDesign {
		t.Errorf("continue = %+v, %v; want %s", got, err, wire.ReasonSessionNotDesign)
	}
}

func TestWebSessionPrompt_StaleContinueIsRefused(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	writeTranscript(t, f.transcript,
		`{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"Which store?"}]}}`,
		`{"type":"assistant","uuid":"a2","timestamp":"2026-01-01T10:00:01Z","message":{"content":[{"type":"text","text":"Or files?"}]}}`,
	)
	post := func(last string) (int, string) {
		body := fmt.Sprintf(`{"id":%q,"continue":true,"last_message":%q}`, f.id, last)
		return servertest.WebPostJSON(t, f.base+"/v1/sessions/prompt", body, f.ck, "", servertest.JSONPost(f.base))
	}
	if code, out := post("a1"); code != http.StatusOK || servertest.ReasonOf(t, out) != wire.ReasonStale {
		t.Errorf("old message id = %d %s, want stale", code, out)
	}
	if got := submitted(t, f.conv); got != "" {
		t.Errorf("stale continue typed %q", got)
	}
	if code, out := post("a2"); code != http.StatusOK || servertest.ReasonOf(t, out) != "" {
		t.Errorf("current message id = %d %s, want sent", code, out)
	}
	waitForSubmitted(t, f.conv, "\"/clear\"\n\"/gx-design map\"\n")
}

func TestWebSessionPrompt_KeepsTheWriteChecks(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	url := f.base + "/v1/sessions/prompt"
	body := fmt.Sprintf(`{"id":%q,"text":"hi"}`, f.id)
	if code, _ := servertest.WebPostJSON(t, url, body, nil, "", servertest.JSONPost(f.base)); code != http.StatusUnauthorized {
		t.Errorf("no token = %d, want 401", code)
	}
	if code, _ := servertest.WebPostJSON(t, url, body, f.ck, "", map[string]string{"Origin": "http://evil.example", "Content-Type": "application/json"}); code != http.StatusForbidden {
		t.Errorf("foreign Origin = %d, want 403", code)
	}
	if code, _ := servertest.WebPostJSON(t, url, body, f.ck, "", map[string]string{"Origin": f.base, "Content-Type": "text/plain"}); code != http.StatusForbidden {
		t.Errorf("text/plain = %d, want 403", code)
	}
}
