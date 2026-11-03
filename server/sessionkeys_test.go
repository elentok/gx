package server_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func (f promptFixture) keys(t *testing.T, req server.SessionKeysRequest) server.SessionResult {
	t.Helper()
	if req.ID == "" {
		req.ID = f.id
	}
	res, err := f.h.Client.KeysSession(f.ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// keysWeb posts the request to the web mux and returns the refusal reason.
func (f promptFixture) keysWeb(t *testing.T, req server.SessionKeysRequest) string {
	t.Helper()
	if req.ID == "" {
		req.ID = f.id
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	code, out := webPostJSON(t, f.base+"/v1/sessions/keys", string(body), f.ck, "", jsonPost(f.base))
	if code != 200 {
		t.Fatalf("web keys = %d %s", code, out)
	}
	return reasonOf(t, out)
}

// seedSession writes a session record with no holder behind it.
func seedSession(t *testing.T, f promptFixture, id, state string) {
	t.Helper()
	dir := filepath.Join(f.h.SessionsDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(map[string]any{"id": id, "harness": "claude", "project": "proj", "dir": t.TempDir(), "status": map[string]any{"state": state}})
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSessionKeys_Refusals(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	seedSession(t, f, "seeded-starting", server.SessionStarting)
	seedSession(t, f, "seeded-exited", server.SessionExited)
	seedSession(t, f, "seeded-nodial", server.SessionIdle)
	for name, tc := range map[string]struct {
		req  server.SessionKeysRequest
		want string
	}{
		"unknown session":    {server.SessionKeysRequest{ID: "nope", Key: "Esc"}, server.ReasonSessionUnknown},
		"bad id":             {server.SessionKeysRequest{ID: "../x", Key: "Esc"}, server.ReasonSessionBadRequest},
		"no input":           {server.SessionKeysRequest{}, server.ReasonSessionBadRequest},
		"two inputs":         {server.SessionKeysRequest{Key: "Esc", Text: "a"}, server.ReasonSessionBadRequest},
		"unknown key":        {server.SessionKeysRequest{Key: "F13"}, server.ReasonSessionBadRequest},
		"newline in text":    {server.SessionKeysRequest{Text: "a\nb"}, server.ReasonSessionBadRequest},
		"tab in text":        {server.SessionKeysRequest{Text: "a\tb"}, server.ReasonSessionBadRequest},
		"escape in text":     {server.SessionKeysRequest{Text: "a\x1bb"}, server.ReasonSessionBadRequest},
		"DEL in text":        {server.SessionKeysRequest{Text: "a\x7fb"}, server.ReasonSessionBadRequest},
		"bang prefix":        {server.SessionKeysRequest{Text: "!ls"}, server.ReasonPromptPrefixNotAllowed},
		"text too long":      {server.SessionKeysRequest{Text: strings.Repeat("a", 32*1024+1)}, server.ReasonPromptTextTooLong},
		"guard while idle":   {server.SessionKeysRequest{Key: "Esc", OnlyBlocked: true}, server.ReasonNotBlocked},
		"starting":           {server.SessionKeysRequest{ID: "seeded-starting", Key: "Esc"}, server.ReasonSessionNotLive},
		"exited":             {server.SessionKeysRequest{ID: "seeded-exited", Key: "Esc"}, server.ReasonSessionNotLive},
		"holder not dialled": {server.SessionKeysRequest{ID: "seeded-nodial", Key: "Esc"}, server.ReasonSessionNotLive},
	} {
		t.Run(name, func(t *testing.T) {
			if got := f.keys(t, tc.req); !got.Refused || got.Reason != tc.want {
				t.Errorf("client = %+v, want refusal %s", got, tc.want)
			}
			if got := f.keysWeb(t, tc.req); got != tc.want {
				t.Errorf("web reason = %q, want %s", got, tc.want)
			}
		})
	}
	if got := submitted(t, f.conv); got != "" {
		t.Errorf("submitted = %q, want nothing sent by a refusal", got)
	}
}

// The refusal comes from the state, not from a failed dial of the holder.
func TestSessionKeys_StartingSessionRefusesKeys(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	seedSession(t, f, "seeded-starting-only", server.SessionStarting)
	got := f.keys(t, server.SessionKeysRequest{ID: "seeded-starting-only", Key: "Esc"})
	if !got.Refused || got.Reason != server.ReasonSessionNotLive || !strings.Contains(got.Message, "is starting") {
		t.Errorf("keys = %+v, want %s naming the starting state", got, server.ReasonSessionNotLive)
	}
}

func TestSessionKeys_QueueAgentIsReadOnly(t *testing.T) {
	t.Parallel()
	h, row, release := startQueueChat(t, "hello")
	defer release()
	res, err := h.Client.KeysSession(context.Background(), server.SessionKeysRequest{ID: row.ID, Key: "Esc"})
	if err != nil || !res.Refused || res.Reason != server.ReasonSessionReadOnly {
		t.Errorf("keys = %+v, %v; want refusal %s", res, err, server.ReasonSessionReadOnly)
	}
}

func TestSessionKeys_NamedKeysReachTheFake(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	// Esc is logged once the next key arrives.
	for _, key := range []string{"Esc", "Tab", "Up", "Down", "Right", "Left", "Backspace", "Ctrl+C", "Shift+Tab", "enter"} {
		if res := f.keys(t, server.SessionKeysRequest{Key: key}); res.Refused {
			t.Fatalf("key %s = %+v", key, res)
		}
	}
	want := []string{agentfake.ProgramEscEntry, "<tab>", "<csi A>", "<csi B>", "<csi C>", "<csi D>", "<backspace>", agentfake.ProgramInterruptEntry, "<csi Z>", ""}
	var sb strings.Builder
	for _, e := range want {
		sb.WriteString(strconv.Quote(e) + "\n")
	}
	waitForSubmitted(t, f.conv, sb.String())
}

// Ports e2e flow 156 "Keys by session state". A Rate-limited session needs a real rate-limit turn
// to reach, so it is not driven here.
func TestSessionKeys_KeysAndTextReachALiveSessionInEveryReachableState(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		state   string
		control agentfake.Control
	}{
		{"idle", agentfake.Control{State: "idle"}},
		{"working", agentfake.Control{State: "working"}},
		{"blocked", agentfake.Control{State: "blocked", Kind: "permission", Message: "run ls?"}},
	} {
		t.Run(tc.state, func(t *testing.T) {
			t.Parallel()
			f := startPromptSession(t)
			holder := waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
			report(t, f.conv, tc.control)
			waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == tc.state })
			for _, key := range []string{"Up", "Down"} {
				if res := f.keys(t, server.SessionKeysRequest{Key: key}); res.Refused {
					t.Fatalf("key %s = %+v", key, res)
				}
			}
			if res := f.keys(t, server.SessionKeysRequest{Text: "typed"}); res.Refused {
				t.Fatalf("text = %+v", res)
			}
			waitForScreen(t, holder, "typed")
			if res := f.keys(t, server.SessionKeysRequest{Key: "Enter"}); res.Refused {
				t.Fatalf("enter = %+v", res)
			}
			waitForSubmitted(t, f.conv, strconv.Quote("<csi A>")+"\n"+strconv.Quote("<csi B>")+"\n"+strconv.Quote("typed")+"\n")
		})
	}
}

func TestSessionKeys_TextIsTypedWithoutEnter(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	holder := waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
	if res := f.keys(t, server.SessionKeysRequest{Text: "hello there"}); res.Refused {
		t.Fatalf("text = %+v", res)
	}
	waitForScreen(t, holder, "hello there")
	if got := submitted(t, f.conv); got != "" {
		t.Errorf("submitted = %q, want the text left in the box", got)
	}
	if res := f.keys(t, server.SessionKeysRequest{Key: "Enter"}); res.Refused {
		t.Fatalf("enter = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("hello there")+"\n")
}

func TestSessionKeys_GuardSendsNothingWhenNotBlocked(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	if res := f.keys(t, server.SessionKeysRequest{Key: "Enter", OnlyBlocked: true}); !res.Refused || res.Reason != server.ReasonNotBlocked {
		t.Fatalf("guarded = %+v, want %s", res, server.ReasonNotBlocked)
	}
	// A guarded Enter that got through would log an empty entry before this one.
	if res := f.keys(t, server.SessionKeysRequest{Key: "Tab"}); res.Refused {
		t.Fatalf("tab = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("<tab>")+"\n")
}

func TestSessionKeys_GuardedSendWaitsForTheSessionToLeaveBlocked(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	report(t, f.conv, agentfake.Control{State: "blocked", Kind: "permission", Message: "run ls?"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionBlocked })
	done := make(chan server.SessionResult, 1)
	go func() {
		res, err := f.h.Client.KeysSession(f.ctx, server.SessionKeysRequest{ID: f.id, Key: "Tab", OnlyBlocked: true})
		if err != nil {
			t.Error(err)
		}
		done <- res
	}()
	waitForSubmitted(t, f.conv, strconv.Quote("<tab>")+"\n")
	select {
	case res := <-done:
		t.Fatalf("returned %+v while still blocked", res)
	default:
	}
	report(t, f.conv, agentfake.Control{State: "idle"})
	select {
	case res := <-done:
		if res.Refused {
			t.Errorf("guarded = %+v, want accepted once the session left Blocked", res)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("guarded send never returned")
	}
}

func TestSessionKeys_GuardedSendThatLeavesBlockedIsNotDelivered(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	report(t, f.conv, agentfake.Control{State: "blocked", Kind: "permission", Message: "run ls?"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionBlocked })
	got := f.keys(t, server.SessionKeysRequest{Key: "Tab", OnlyBlocked: true})
	if !got.Refused || got.Reason != server.ReasonKeysNotDelivered {
		t.Errorf("guarded = %+v, want %s", got, server.ReasonKeysNotDelivered)
	}
}

func TestSessionKeys_LogsOneEventWithoutTheText(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	const text = "secret-token"
	if res := f.keys(t, server.SessionKeysRequest{Text: text}); res.Refused {
		t.Fatalf("keys = %+v", res)
	}
	if reason := f.keysWeb(t, server.SessionKeysRequest{Key: "Tab"}); reason != "" {
		t.Fatalf("web keys refused: %s", reason)
	}
	evs, ok, err := ralphloop.ReadServerEvents(f.h.TicketStore)
	if err != nil || !ok || len(evs) != 2 {
		t.Fatalf("server events = %+v, ok=%v, err=%v; want two", evs, ok, err)
	}
	if ev := evs[0]; ev.Type != "session-keyed" || ev.Session != f.id || ev.Source != "cli" || ev.Action != "text" || ev.Bytes != len(text) {
		t.Errorf("text event = %+v", ev)
	}
	if ev := evs[1]; ev.Type != "session-keyed" || ev.Source != "web" || ev.Action != "key" || ev.Bytes != 1 {
		t.Errorf("key event = %+v", ev)
	}
	raw, err := os.ReadFile(filepath.Join(f.h.TicketStore, "server-log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") {
		t.Errorf("server log holds the text: %s", raw)
	}
}

// A swallowed Enter would be nudged again after the grace; a key from the
// person ends the nudging.
func TestSessionKeys_StopTheNudges(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, time.Second)
	holder := waitForHolder(t, filepath.Join(f.h.SessionsDir, f.id))
	if err := agentfake.SendProgramControl(f.conv, agentfake.Control{Action: agentfake.ActionSwallowEnter}); err != nil {
		t.Fatal(err)
	}
	if res := f.prompt(t, "yes to both"); res.Refused {
		t.Fatalf("prompt = %+v", res)
	}
	// The key counts as a person's only once gx has typed.
	waitForScreen(t, holder, "yes to both")
	if res := f.keys(t, server.SessionKeysRequest{Key: "Tab"}); res.Refused {
		t.Fatalf("tab = %+v", res)
	}
	time.Sleep(3 * time.Second) // negative wait: a nudge would come within two graces
	if got := submitted(t, f.conv); got != strconv.Quote("<tab>")+"\n" {
		t.Errorf("submitted = %q, want the Tab and no nudged Enter", got)
	}
}

func startKeysAdvancing(t *testing.T) *grillingFixture {
	t.Helper()
	h, ctx, evs, id, conv, path := startGrillingSessionIn(t, "proj:map", storeWithMapStep(t, "spec"),
		servertest.WithAutoAdvanceDelay(5*time.Second), servertest.WithEnv(agentfake.ProgramInputEnv+"=1"))
	f := &grillingFixture{h, ctx, evs, id, conv, path}
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	writeAssistantMessage(t, path, runEndText)
	endTurn(t, f)
	waitForSession(ctx, t, h, evs, id, func(s server.SessionInfo) bool { return s.AutoAdvanceAt != nil })
	return f
}

func TestSessionKeys_DesignKeyStopsTheCountdown(t *testing.T) {
	t.Parallel()
	f := startKeysAdvancing(t)
	if res, err := f.h.Client.KeysSession(f.ctx, server.SessionKeysRequest{ID: f.id, Key: "Tab"}); err != nil || res.Refused {
		t.Fatalf("keys = %+v, %v", res, err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.AutoAdvanceAt == nil && s.Actionable.Reason == "" })
	if n := autoSends(t, f); n != 0 {
		t.Errorf("auto sends = %d, want none", n)
	}
}

func TestSessionKeys_GuardedKeyKeepsTheCountdown(t *testing.T) {
	t.Parallel()
	f := startKeysAdvancing(t)
	rows := agentfake.BashPermissionDialog("ls")
	if err := agentfake.SendProgramControl(f.conv, agentfake.Control{Action: agentfake.ActionDialogBlock, Dialog: rows, Message: "run ls?"}); err != nil {
		t.Fatal(err)
	}
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s server.SessionInfo) bool { return s.Status.State == server.SessionBlocked })
	// Close dialog's Esc: whether the fake leaves Blocked on it does not matter here.
	if _, err := f.h.Client.KeysSession(f.ctx, server.SessionKeysRequest{ID: f.id, Key: "Esc", OnlyBlocked: true}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second) // negative wait: the holder wakes the session's stream within this window
	if got := sessionSnapshot(t, f); got.AutoAdvanceAt == nil {
		t.Errorf("session = %+v, want the countdown running after a guarded key", got)
	}
}

func TestSessionKeys_GuardedRefusalKeepsTheCountdown(t *testing.T) {
	t.Parallel()
	f := startKeysAdvancing(t)
	res, err := f.h.Client.KeysSession(f.ctx, server.SessionKeysRequest{ID: f.id, Key: "Tab", OnlyBlocked: true})
	if err != nil || !res.Refused || res.Reason != server.ReasonNotBlocked {
		t.Fatalf("keys = %+v, %v; want %s", res, err, server.ReasonNotBlocked)
	}
	if got := sessionSnapshot(t, f); got.AutoAdvanceAt == nil {
		t.Errorf("session = %+v, want the countdown running", got)
	}
}
