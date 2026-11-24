package notify

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/elentok/gx/notifykind"
	"github.com/elentok/gx/ralphloop/schedule"
)

// fakePolicy is a ChannelPolicy a test flips between sends. Channels named in
// off are switched off; ids in unchecked are missing from every kind list.
type fakePolicy struct {
	mu        sync.Mutex
	off       map[string]bool
	unchecked map[string]bool
}

func newFakePolicy() *fakePolicy {
	return &fakePolicy{off: map[string]bool{}, unchecked: map[string]bool{}}
}

func (p *fakePolicy) set(m map[string]bool, k string, v bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	m[k] = v
}

func (p *fakePolicy) ChannelOn(ch string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.off[ch]
}

func (p *fakePolicy) KindChecked(_, id string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.unchecked[id]
}

// routerRig is a ServerChat with a Slack and a Telegram destination.
type routerRig struct {
	chat      *ServerChat
	policy    *fakePolicy
	statePath string
	slack     func() int
	telegram  func() int
}

func newRouterRig(t *testing.T) *routerRig {
	t.Helper()
	slackSrv, slackReqs := fakeSlackServer(t, 200)
	tgSrv, tgReqs := fakeTelegramServer(t, 200)
	r := &routerRig{
		policy:    newFakePolicy(),
		statePath: filepath.Join(t.TempDir(), "gate.json"),
		slack:     func() int { return len(slackReqs()) },
		telegram:  func() int { return len(tgReqs()) },
	}
	r.chat = NewServerChat(ServerChatConfig{
		SlackWebhookURL:  slackSrv.URL,
		TelegramBotToken: "tok", TelegramChatID: "1", TelegramBaseURL: tgSrv.URL,
		GateStatePath: r.statePath,
		Policy:        r.policy,
	})
	return r
}

// sends closes the chat, which flushes every queued batch, and returns what
// each destination received.
func (r *routerRig) sends() (slack, telegram int) {
	r.chat.Close()
	return r.slack(), r.telegram()
}

func (r *routerRig) state() string {
	b, _ := os.ReadFile(r.statePath)
	return string(b)
}

func (r *routerRig) sink(t *testing.T, name string) *chatEventSink {
	t.Helper()
	for _, s := range r.chat.globalSinks() {
		if s.transport.name() == name {
			return s
		}
	}
	t.Fatalf("no %s sink", name)
	return nil
}

func notice(kind string) func(*ServerChat) {
	return func(c *ServerChat) { c.Notice(ServerNotice{Kind: kind, Title: "t"}) }
}

// catalogCalls maps each catalog id to a call that sends that kind.
func catalogCalls(t *testing.T) map[string]func(*ServerChat) {
	dir := t.TempDir()
	return map[string]func(*ServerChat){
		notifykind.SessionAlert:   func(c *ServerChat) { c.SessionAlert("p", nil, "n", "Blocked", "b", true) },
		notifykind.DesignQuestion: func(c *ServerChat) { c.DesignQuestion("p", nil, "e", "step", 1, "q", "l") },
		notifykind.DesignReady:    func(c *ServerChat) { c.DesignReady("p", nil, "d", "i", 2, "l") },
		notifykind.ChatQuestion:   func(c *ServerChat) { c.ChatQuestion("p", nil, "s", "q", "l") },
		notifykind.PausedTicket:   func(c *ServerChat) { c.Escalated("p", nil, "e", "/t/1.md", "01", "f") },
		notifykind.OneOffResult:   func(c *ServerChat) { c.Result("p", nil, dir, "e", "/t/1.md", "01", "res") },
		notifykind.ConflictResolutionStarted: func(c *ServerChat) {
			c.FinishSink("p", nil, "e", "/t/1.md", "01", nil).ConflictResolutionStarted("02")
		},
		notifykind.EpicComplete:    func(c *ServerChat) { c.EpicComplete("p", nil, "e", schedule.EpicCounts{}, 1, 0, true) },
		notifykind.EpicPaused:      func(c *ServerChat) { c.EpicPause("p", nil, "e", "label", "reason") },
		notifykind.ProgressUpdate:  func(c *ServerChat) { c.Progress("p", nil, "e", ProgressFacts{}) },
		notifykind.LongTicket:      notice("long-ticket"),
		notifykind.BudgetAndUsage:  notice("budget-soft-limit"),
		notifykind.Disk:            notice("disk-warn"),
		notifykind.ProjectAndTrunk: notice("trunk-missing"),
		notifykind.ServerStarted:   notice("server-started"),
		notifykind.CLIMessage:      func(c *ServerChat) { c.Notify("hi") },
	}
}

func TestRouter_EveryCatalogKindReachesBothChatsOnlyWhenChecked(t *testing.T) {
	calls := catalogCalls(t)
	if len(calls) != len(notifykind.IDs()) {
		t.Fatalf("table covers %d kinds, catalog has %d", len(calls), len(notifykind.IDs()))
	}
	for _, id := range notifykind.IDs() {
		call := calls[id]
		t.Run(id+"/checked", func(t *testing.T) {
			r := newRouterRig(t)
			call(r.chat)
			if slack, tg := r.sends(); slack != 1 || tg != 1 {
				t.Errorf("slack=%d telegram=%d, want 1 and 1", slack, tg)
			}
		})
		t.Run(id+"/unchecked", func(t *testing.T) {
			r := newRouterRig(t)
			r.policy.set(r.policy.unchecked, id, true)
			call(r.chat)
			if slack, tg := r.sends(); slack != 0 || tg != 0 {
				t.Errorf("slack=%d telegram=%d, want nothing", slack, tg)
			}
			if got := r.state(); got != "" {
				t.Errorf("an unchecked kind wrote the state file:\n%s", got)
			}
		})
	}
}

func TestRouter_RootPauseCountsAsAnEpicPause(t *testing.T) {
	r := newRouterRig(t)
	r.policy.set(r.policy.unchecked, notifykind.PausedTicket, true)
	r.chat.RootPause("p", nil, "e", "/t/1.md", "01", "answer", "blocked")
	if slack, tg := r.sends(); slack != 1 || tg != 1 {
		t.Errorf("slack=%d telegram=%d, want 1 and 1 with only paused-ticket unchecked", slack, tg)
	}
}

func TestRouter_SwitchOffSendsNothingAndKeepsTheDestination(t *testing.T) {
	r := newRouterRig(t)
	r.policy.set(r.policy.off, "slack", true)
	r.chat.Notify("hi")
	r.sink(t, "slack").enqueueMessage(mutedMessage("cli", ""))
	r.sink(t, "slack").enqueueMessage(globallyMutedMessage("slack"))
	if got := len(r.chat.globalSinks()); got != 2 {
		t.Errorf("global sinks = %d, want 2: the switch keeps the destination", got)
	}
	if !r.chat.Enabled() {
		t.Error("Enabled = false with Telegram still on")
	}
	r.policy.set(r.policy.off, "telegram", true)
	if r.chat.Enabled() {
		t.Error("Enabled = true with both switches off")
	}
	if slack, tg := r.sends(); slack != 0 || tg != 0 {
		t.Errorf("slack=%d telegram=%d, want nothing, mute notices included", slack, tg)
	}
}

func TestRouter_SystemMessageSkipsTheKindListButNotTheSwitch(t *testing.T) {
	r := newRouterRig(t)
	for _, id := range notifykind.IDs() {
		r.policy.set(r.policy.unchecked, id, true)
	}
	r.sink(t, "slack").enqueueMessage(globallyMutedMessage("slack"))
	r.chat.Notify("hi")
	if slack, tg := r.sends(); slack != 1 || tg != 0 {
		t.Errorf("slack=%d telegram=%d, want only the system message on slack", slack, tg)
	}

	r = newRouterRig(t)
	r.policy.set(r.policy.off, "slack", true)
	r.sink(t, "slack").enqueueMessage(globallyMutedMessage("slack"))
	if slack, _ := r.sends(); slack != 0 {
		t.Errorf("slack=%d, want a system message dropped by the switch", slack)
	}
}

func TestRouter_BatchQueuedBeforeTheSwitchWentOffIsDroppedAtFlushAndClose(t *testing.T) {
	for _, via := range []string{"flush", "close"} {
		t.Run(via, func(t *testing.T) {
			r := newRouterRig(t)
			r.chat.Notify("queued")
			r.policy.set(r.policy.off, "slack", true)
			if via == "flush" {
				r.sink(t, "slack").flush()
			}
			if slack, tg := r.sends(); slack != 0 || tg != 1 {
				t.Errorf("slack=%d telegram=%d, want the slack batch dropped and telegram sent", slack, tg)
			}
		})
	}
}

func TestRouter_UnknownNoticeKindIsASystemMessage(t *testing.T) {
	if got := noticeCatalogID("something-new"); got != "" {
		t.Errorf("unknown notice kind maps to %q, want no catalog id", got)
	}
}
