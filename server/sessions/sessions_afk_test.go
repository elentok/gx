package sessions_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
)

// startAFKSession starts a working session whose server chats to a recording
// Slack hook, with AFK set to afk; it returns the posts index after the start
// notice, the gate state path and the session id.
func startAFKSession(t *testing.T, afk bool) (*servertest.Harness, *slackHook, int, string, string, func(agentfake.Control, string)) {
	t.Helper()
	chat := newSlackHook(t)
	gate := filepath.Join(t.TempDir(), "gate.json")
	h, ctx, evs, id, conv := startBannerSession(t, func(c *server.Config) {
		c.Chat = ralphloop.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: gate, FlushInterval: testFlushInterval}
	})
	started := chat.WaitStarted(t)
	if _, err := h.Client.Sessions.SetAFK(ctx, &afk); err != nil {
		t.Fatal(err)
	}
	return h, chat, started, gate, id, func(c agentfake.Control, state string) {
		reach(ctx, t, h, evs, id, conv, c, state)
	}
}

func TestSessionAFK_DoneSendsBannerTextToChatEvenOnScreen(t *testing.T) {
	t.Parallel()
	h, chat, started, _, id, step := startAFKSession(t, true)
	ctx := t.Context()
	openTUI(ctx, t, h, "tui-1")
	reportFocus(ctx, t, h, wire.FocusReport{Client: "tui-1", Focused: true, Session: id})

	step(agentfake.Control{State: "done"}, wire.SessionIdle)
	posts := chat.PostsSince(started)
	if len(posts) != 1 || !strings.Contains(posts[0], "proj · fix-build") || !strings.Contains(posts[0], "Done") {
		t.Errorf("chat posts = %q, want one with the banner text", posts)
	}
	if banners := h.Banners.Sent(); len(banners) != 0 {
		t.Errorf("banners = %+v, want none while on screen", banners)
	}
}

func TestSessionAFK_OffSendsNoChat(t *testing.T) {
	t.Parallel()
	h, chat, started, _, _, step := startAFKSession(t, false)
	step(agentfake.Control{State: "done"}, wire.SessionIdle)
	if posts := chat.PostsAfterAlert(h, started); len(posts) != 0 {
		t.Errorf("chat posts = %q, want none", posts)
	}
	if banners := h.Banners.Sent(); len(banners) != 1 {
		t.Errorf("banners = %+v, want one", banners)
	}
}

func TestSessionAFK_NotificationsDisabledSuppressesChat(t *testing.T) {
	t.Parallel()
	h, chat, started, gate, _, step := startAFKSession(t, true)
	muted, _ := json.Marshal(ralphloop.NotificationState{Transports: map[string]ralphloop.TransportState{"slack": {Muted: true, Reason: "manual-disable"}}})
	if err := os.WriteFile(gate, muted, 0o600); err != nil {
		t.Fatal(err)
	}
	step(agentfake.Control{State: "done"}, wire.SessionIdle)
	if posts := chat.PostsAfterAlert(h, started); len(posts) != 0 {
		t.Errorf("chat posts = %q, want none while muted", posts)
	}
	if banners := h.Banners.Sent(); len(banners) != 1 {
		t.Errorf("banners = %+v, want the banner to keep going", banners)
	}
}
