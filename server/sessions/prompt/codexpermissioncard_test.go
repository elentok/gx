package prompt_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil/agentfake"
)

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
