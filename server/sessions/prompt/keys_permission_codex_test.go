package prompt_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/testutil/agentfake"
)

// startCodexPermissionSession is a Codex session with its own conversation id,
// because the fake's control channel is named by it and these tests run in
// parallel. withDialog makes the fake draw its own approval dialog.
func startCodexPermissionSession(t *testing.T, withDialog bool) promptFixture {
	t.Helper()
	return fromPrompt(rigs.StartCodexPermissionSession(t, withDialog))
}

func (f promptFixture) blockCodex(t *testing.T, command string) {
	t.Helper()
	f.fixture().BlockCodex(t, command)
}

func TestSessionKeys_CodexScreenDialogTakesYAndEsc(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		decision string
		want     func(wire.SessionInfo) bool
	}{
		"allow": {"allow", func(s wire.SessionInfo) bool { return s.Status.State == wire.SessionWorking }},
		"deny":  {"deny", func(s wire.SessionInfo) bool { return s.Status.State != wire.SessionBlocked }},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := startCodexPermissionSession(t, true)
			f.blockCodex(t, "rg foo")
			if res := f.permission(t, tc.decision, ""); res.Refused {
				t.Fatalf("%s = %+v", tc.decision, res)
			}
			waitForSession(f.ctx, t, f.h, f.evs, f.id, tc.want)
		})
	}
}

// Without a hook request and without a dialog gx can read, Esc is still sent
// but Allow has no key to press.
func TestSessionKeys_CodexUnreadableScreenDeniesButNeverApproves(t *testing.T) {
	t.Parallel()
	f := startCodexPermissionSession(t, true)
	// A dialog with no command under its title is not one gx answers.
	f.blockCodex(t, "")
	if res := f.permission(t, "allow", ""); !res.Refused || res.Reason != wire.ReasonNotApprovable {
		t.Fatalf("allow = %+v, want %s", res, wire.ReasonNotApprovable)
	}
	// The refused Allow sent no y: the fake is still blocked, and Esc ends it.
	if got := sessionSnapshotOf(t, f.designsFixture, f.id).Status.State; got != wire.SessionBlocked {
		t.Fatalf("state after the refusal = %s, want blocked", got)
	}
	if res := f.permission(t, "deny", ""); res.Refused {
		t.Fatalf("deny = %+v", res)
	}
}

func TestSessionKeys_CodexHookRequestIsAnsweredByTheDecisionFile(t *testing.T) {
	t.Parallel()
	f := startCodexPermissionSession(t, false)
	dir := filepath.Join(f.h.SessionsDir, f.id)
	if err := holder.WritePermissionRequest(dir, holder.PermissionRequest{ID: "req-1", Tool: "Bash", Description: "rg foo"}); err != nil {
		t.Fatal(err)
	}
	f.block(t, agentfake.Control{Action: agentfake.ActionReport, State: "blocked", Kind: "permission"})

	if res := f.permission(t, "allow", "req-0"); !res.Refused || res.Reason != wire.ReasonRequestChanged {
		t.Fatalf("allow of another request = %+v, want %s", res, wire.ReasonRequestChanged)
	}
	if _, ok, err := holder.ReadPermissionDecision(dir); err != nil || ok {
		t.Fatalf("decision after a mismatch = %v, %v; want none", ok, err)
	}

	// Nothing here plays the hook, which would read the file and let the agent
	// go on, so the session stays Blocked: the answer is written, not delivered.
	if res := f.permission(t, "allow", "req-1"); !res.Refused || res.Reason != wire.ReasonKeysNotDelivered {
		t.Fatalf("allow = %+v, want %s", res, wire.ReasonKeysNotDelivered)
	}
	d, ok, err := holder.ReadPermissionDecision(dir)
	if err != nil || !ok || d.ID != "req-1" || !d.Allow {
		t.Errorf("decision = %+v, %v, %v; want req-1 allowed", d, ok, err)
	}
	if got := submitted(t, f.conv); got != "" {
		t.Errorf("submitted = %q, want no key with a hook request", got)
	}
}

func (f promptFixture) hookCodex(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(f.h.SessionsDir, f.id)
	if err := holder.WritePermissionRequest(dir, holder.PermissionRequest{ID: "req-1", Tool: "Bash", Description: "rg foo"}); err != nil {
		t.Fatal(err)
	}
	f.block(t, agentfake.Control{Action: agentfake.ActionReport, State: "blocked", Kind: "permission"})
	return dir
}

func TestCodexPermissionCard_HookedRequestListsAllowAndDeny(t *testing.T) {
	t.Parallel()
	f := startCodexPermissionSession(t, false)
	f.hookCodex(t)
	card := f.permissionCard(t)
	q := card.Questions[0]
	var ids []string
	for _, o := range q.Options {
		ids = append(ids, o.ID+"="+o.Label)
	}
	if card.ID != "req-1" || q.Body != "rg foo" || !slices.Equal(ids, []string{"allow=Allow", "deny=Deny"}) {
		t.Errorf("card = %+v, want req-1 with the request text and Allow and Deny", card)
	}
}

func TestCodexPermissionCard_AnswerWritesTheDecision(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		answers []wire.SessionAnswer
		allow   bool
	}{
		"allow":   {[]wire.SessionAnswer{{Choice: "allow"}}, true},
		"deny":    {[]wire.SessionAnswer{{Choice: "deny"}}, false},
		"dismiss": {nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := startCodexPermissionSession(t, false)
			dir := f.hookCodex(t)
			f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourcePermission, CardID: "req-1", Answers: tc.answers})
			d, ok, err := holder.ReadPermissionDecision(dir)
			if err != nil || !ok || d.ID != "req-1" || d.Allow != tc.allow {
				t.Errorf("decision = %+v, %v, %v; want req-1 allow=%v", d, ok, err, tc.allow)
			}
		})
	}
}

func TestCodexPermissionCard_StaleIdSendsNothing(t *testing.T) {
	t.Parallel()
	f := startCodexPermissionSession(t, false)
	dir := f.hookCodex(t)
	res := f.answer(t, wire.SessionAnswerRequest{Source: wire.CardSourcePermission, CardID: "req-0", Answers: []wire.SessionAnswer{{Choice: "allow"}}})
	if !res.Refused || res.Reason != wire.ReasonStale {
		t.Errorf("answer = %+v, want %s", res, wire.ReasonStale)
	}
	if _, ok, err := holder.ReadPermissionDecision(dir); err != nil || ok {
		t.Errorf("decision = %v, %v; want none", ok, err)
	}
}

func TestCodexPermissionCard_ScreenOnlyApprovalListsNoCardAndKeepsTheBanner(t *testing.T) {
	t.Parallel()
	f := startCodexPermissionSession(t, true)
	f.blockCodex(t, "rg foo")
	chat, err := f.h.Client.Sessions.Chat(f.ctx, f.id)
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.Questions) != 0 {
		t.Errorf("questions = %+v, want none", chat.Questions)
	}
	if chat.Failure == nil || chat.Failure.Kind != wire.FailureKindBlocked {
		t.Errorf("failure = %+v, want the blocked banner", chat.Failure)
	}
}
