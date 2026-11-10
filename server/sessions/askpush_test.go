package sessions_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/servertest/rigs"
	"github.com/elentok/gx/server/wire"
)

// pushRig is an ask fixture whose server chats to a recording Slack hook.
type pushRig struct {
	f       askFixture
	chat    *rigs.SlackHook
	started int
	gate    string
}

func startPushRig(t *testing.T, link *wire.LinkSessionRequest, seed func(gate string)) *pushRig {
	t.Helper()
	chat := rigs.NewSlackHook(t)
	gate := filepath.Join(t.TempDir(), "gate.json")
	if seed != nil {
		seed(gate)
	}
	f := startAskFixture(t, link, func(c *server.Config) {
		c.Chat = notify.ServerChatConfig{SlackWebhookURL: chat.URL, GateStatePath: gate, FlushInterval: servertest.FlushInterval}
	})
	return &pushRig{f: f, chat: chat, started: chat.WaitStarted(t), gate: gate}
}

// questions waits out the chat flush and returns the Question posts.
func (r *pushRig) questions() int {
	time.Sleep(10 * servertest.FlushInterval) // negative wait: no event marks the chat flush
	n := 0
	for _, p := range r.chat.PostsFrom(r.started) {
		n += strings.Count(p, "❓ Question")
	}
	return n
}

func TestChatQuestionPush_HeldDialogSendsOnePushAndLinksNothingTwice(t *testing.T) {
	t.Parallel()
	r := startPushRig(t, nil, nil)
	r.f.ask(t)
	r.f.waitBlocked(t)
	servertest.WaitFor(t, 10*time.Second, "the question push", func() bool {
		return len(r.chat.PostsFrom(r.started)) > 0
	})
	// More status changes of the same dialog must not push again.
	r.f.cards(t)
	r.f.info(t)
	if got := r.questions(); got != 1 {
		t.Errorf("Question pushes = %d, want 1", got)
	}
}

func TestChatQuestionPush_AMarkedCardIsNeverResent(t *testing.T) {
	t.Parallel()
	var id string
	r := startPushRig(t, nil, nil)
	id = r.f.id
	// What a restart leaves behind: the card's mark already on disk.
	if isNew, err := notify.MarkOnce(r.gate, "chat-question:"+id+":"+wire.CardSourceClaudeDialog+":toolu_ask"); err != nil || !isNew {
		t.Fatalf("seed mark = %v, %v", isNew, err)
	}
	r.f.ask(t)
	r.f.waitBlocked(t)
	if got := r.questions(); got != 0 {
		t.Errorf("Question pushes = %d, want none for a card already marked", got)
	}
}

func TestChatQuestionPush_NotSentWhileOpenOnScreen(t *testing.T) {
	t.Parallel()
	r := startPushRig(t, nil, nil)
	servertest.OpenTUI(r.f.ctx, t, r.f.h, "tui-1")
	servertest.ReportFocus(r.f.ctx, t, r.f.h, wire.FocusReport{Client: "tui-1", Focused: true, Session: r.f.id})
	r.f.ask(t)
	r.f.waitBlocked(t)
	if got := r.questions(); got != 0 {
		t.Errorf("Question pushes = %d, want none while open", got)
	}
}

func TestChatQuestionPush_DesignAndTriageSessionsSendNone(t *testing.T) {
	t.Parallel()
	for name, link := range map[string]wire.LinkSessionRequest{
		"design": {Ticket: "proj:map"},
		"triage": {Flow: "triage", FlowKey: "proj"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := startPushRig(t, &link, nil)
			r.f.ask(t)
			r.f.waitBlocked(t)
			if got := r.questions(); got != 0 {
				t.Errorf("Question pushes = %d, want none", got)
			}
		})
	}
}
