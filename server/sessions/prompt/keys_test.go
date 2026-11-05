package prompt_test

import (
	"context"
	"github.com/elentok/gx/server/servertest/rigs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

func (f promptFixture) keys(t *testing.T, req wire.SessionKeysRequest) wire.SessionResult {
	t.Helper()
	return f.fixture().Keys(t, req)
}

// keysWeb posts the request to the web mux and returns the refusal reason.
func (f promptFixture) keysWeb(t *testing.T, req wire.SessionKeysRequest) string {
	t.Helper()
	return f.fixture().KeysWeb(t, req)
}

func seedSession(t *testing.T, f promptFixture, id, state string) {
	t.Helper()
	servertest.SeedSession(t, f.fixture(), id, state)
}

func TestSessionKeys_Refusals(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	seedSession(t, f, "seeded-starting", wire.SessionStarting)
	seedSession(t, f, "seeded-exited", wire.SessionExited)
	seedSession(t, f, "seeded-nodial", wire.SessionIdle)
	for name, tc := range map[string]struct {
		req  wire.SessionKeysRequest
		want string
	}{
		"unknown session":    {wire.SessionKeysRequest{ID: "nope", Key: "Esc"}, wire.ReasonSessionUnknown},
		"bad id":             {wire.SessionKeysRequest{ID: "../x", Key: "Esc"}, wire.ReasonSessionBadRequest},
		"no input":           {wire.SessionKeysRequest{}, wire.ReasonSessionBadRequest},
		"two inputs":         {wire.SessionKeysRequest{Key: "Esc", Text: "a"}, wire.ReasonSessionBadRequest},
		"unknown key":        {wire.SessionKeysRequest{Key: "F13"}, wire.ReasonSessionBadRequest},
		"newline in text":    {wire.SessionKeysRequest{Text: "a\nb"}, wire.ReasonSessionBadRequest},
		"tab in text":        {wire.SessionKeysRequest{Text: "a\tb"}, wire.ReasonSessionBadRequest},
		"escape in text":     {wire.SessionKeysRequest{Text: "a\x1bb"}, wire.ReasonSessionBadRequest},
		"DEL in text":        {wire.SessionKeysRequest{Text: "a\x7fb"}, wire.ReasonSessionBadRequest},
		"bang prefix":        {wire.SessionKeysRequest{Text: "!ls"}, wire.ReasonPromptPrefixNotAllowed},
		"text too long":      {wire.SessionKeysRequest{Text: strings.Repeat("a", 32*1024+1)}, wire.ReasonPromptTextTooLong},
		"guard while idle":   {wire.SessionKeysRequest{Key: "Esc", OnlyBlocked: true}, wire.ReasonNotBlocked},
		"starting":           {wire.SessionKeysRequest{ID: "seeded-starting", Key: "Esc"}, wire.ReasonSessionNotLive},
		"exited":             {wire.SessionKeysRequest{ID: "seeded-exited", Key: "Esc"}, wire.ReasonSessionNotLive},
		"holder not dialled": {wire.SessionKeysRequest{ID: "seeded-nodial", Key: "Esc"}, wire.ReasonSessionNotLive},
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
	seedSession(t, f, "seeded-starting-only", wire.SessionStarting)
	got := f.keys(t, wire.SessionKeysRequest{ID: "seeded-starting-only", Key: "Esc"})
	if !got.Refused || got.Reason != wire.ReasonSessionNotLive || !strings.Contains(got.Message, "is starting") {
		t.Errorf("keys = %+v, want %s naming the starting state", got, wire.ReasonSessionNotLive)
	}
}

func TestSessionKeys_QueueAgentIsReadOnly(t *testing.T) {
	t.Parallel()
	h, row, release := rigs.StartQueueChat(t, "hello")
	defer release()
	res, err := h.Client.Sessions.Keys(context.Background(), wire.SessionKeysRequest{ID: row.ID, Key: "Esc"})
	if err != nil || !res.Refused || res.Reason != wire.ReasonSessionReadOnly {
		t.Errorf("keys = %+v, %v; want refusal %s", res, err, wire.ReasonSessionReadOnly)
	}
}

func TestSessionKeys_NamedKeysReachTheFake(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	// Esc is logged once the next key arrives.
	for _, key := range []string{"Esc", "Tab", "Up", "Down", "Right", "Left", "Backspace", "Ctrl+C", "Shift+Tab", "enter"} {
		if res := f.keys(t, wire.SessionKeysRequest{Key: key}); res.Refused {
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
			waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == tc.state })
			for _, key := range []string{"Up", "Down"} {
				if res := f.keys(t, wire.SessionKeysRequest{Key: key}); res.Refused {
					t.Fatalf("key %s = %+v", key, res)
				}
			}
			if res := f.keys(t, wire.SessionKeysRequest{Text: "typed"}); res.Refused {
				t.Fatalf("text = %+v", res)
			}
			waitForScreen(t, holder, "typed")
			if res := f.keys(t, wire.SessionKeysRequest{Key: "Enter"}); res.Refused {
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
	if res := f.keys(t, wire.SessionKeysRequest{Text: "hello there"}); res.Refused {
		t.Fatalf("text = %+v", res)
	}
	waitForScreen(t, holder, "hello there")
	if got := submitted(t, f.conv); got != "" {
		t.Errorf("submitted = %q, want the text left in the box", got)
	}
	if res := f.keys(t, wire.SessionKeysRequest{Key: "Enter"}); res.Refused {
		t.Fatalf("enter = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("hello there")+"\n")
}

func TestSessionKeys_GuardSendsNothingWhenNotBlocked(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	if res := f.keys(t, wire.SessionKeysRequest{Key: "Enter", OnlyBlocked: true}); !res.Refused || res.Reason != wire.ReasonNotBlocked {
		t.Fatalf("guarded = %+v, want %s", res, wire.ReasonNotBlocked)
	}
	// A guarded Enter that got through would log an empty entry before this one.
	if res := f.keys(t, wire.SessionKeysRequest{Key: "Tab"}); res.Refused {
		t.Fatalf("tab = %+v", res)
	}
	waitForSubmitted(t, f.conv, strconv.Quote("<tab>")+"\n")
}

func TestSessionKeys_GuardedSendWaitsForTheSessionToLeaveBlocked(t *testing.T) {
	t.Parallel()
	f := startPromptSessionWithGrace(t, 10*time.Second)
	report(t, f.conv, agentfake.Control{State: "blocked", Kind: "permission", Message: "run ls?"})
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionBlocked })
	done := make(chan wire.SessionResult, 1)
	go func() {
		res, err := f.h.Client.Sessions.Keys(f.ctx, wire.SessionKeysRequest{ID: f.id, Key: "Tab", OnlyBlocked: true})
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
	waitForSession(f.ctx, t, f.h, f.evs, f.id, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionBlocked })
	got := f.keys(t, wire.SessionKeysRequest{Key: "Tab", OnlyBlocked: true})
	if !got.Refused || got.Reason != wire.ReasonKeysNotDelivered {
		t.Errorf("guarded = %+v, want %s", got, wire.ReasonKeysNotDelivered)
	}
}

func TestSessionKeys_LogsOneEventWithoutTheText(t *testing.T) {
	t.Parallel()
	f := startPromptSession(t)
	const text = "secret-token"
	if res := f.keys(t, wire.SessionKeysRequest{Text: text}); res.Refused {
		t.Fatalf("keys = %+v", res)
	}
	if reason := f.keysWeb(t, wire.SessionKeysRequest{Key: "Tab"}); reason != "" {
		t.Fatalf("web keys refused: %s", reason)
	}
	evs, ok, err := runlog.ReadServerEvents(f.h.TicketStore)
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
	if res := f.keys(t, wire.SessionKeysRequest{Key: "Tab"}); res.Refused {
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
	f := &grillingFixture{H: h, Ctx: ctx, Evs: evs, ID: id, Conv: conv, Path: path}
	waitForHolder(t, filepath.Join(h.SessionsDir, id))
	writeAssistantMessage(t, path, runEndText)
	endTurn(t, f)
	waitForSession(ctx, t, h, evs, id, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt != nil })
	return f
}

func TestSessionKeys_DesignKeyStopsTheCountdown(t *testing.T) {
	t.Parallel()
	f := startKeysAdvancing(t)
	if res, err := f.H.Client.Sessions.Keys(f.Ctx, wire.SessionKeysRequest{ID: f.ID, Key: "Tab"}); err != nil || res.Refused {
		t.Fatalf("keys = %+v, %v", res, err)
	}
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.AutoAdvanceAt == nil && s.Actionable.Reason == "" })
	if n := autoSends(t, f); n != 0 {
		t.Errorf("auto sends = %d, want none", n)
	}
}

func TestSessionKeys_GuardedKeyKeepsTheCountdown(t *testing.T) {
	t.Parallel()
	f := startKeysAdvancing(t)
	rows := agentfake.BashPermissionDialog("ls")
	if err := agentfake.SendProgramControl(f.Conv, agentfake.Control{Action: agentfake.ActionDialogBlock, Dialog: rows, Message: "run ls?"}); err != nil {
		t.Fatal(err)
	}
	waitForSession(f.Ctx, t, f.H, f.Evs, f.ID, func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionBlocked })
	// Close dialog's Esc: whether the fake leaves Blocked on it does not matter here.
	if _, err := f.H.Client.Sessions.Keys(f.Ctx, wire.SessionKeysRequest{ID: f.ID, Key: "Esc", OnlyBlocked: true}); err != nil {
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
	res, err := f.H.Client.Sessions.Keys(f.Ctx, wire.SessionKeysRequest{ID: f.ID, Key: "Tab", OnlyBlocked: true})
	if err != nil || !res.Refused || res.Reason != wire.ReasonNotBlocked {
		t.Fatalf("keys = %+v, %v; want %s", res, err, wire.ReasonNotBlocked)
	}
	if got := sessionSnapshot(t, f); got.AutoAdvanceAt == nil {
		t.Errorf("session = %+v, want the countdown running", got)
	}
}
