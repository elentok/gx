package budget

import (
	"sync"
	"time"

	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/ralphloop/agent"
	harnesschoice "github.com/elentok/gx/server/harness"
	"github.com/elentok/gx/subscription"
)

const NoticeExtraUsage = "extra-usage"

// extraUsageNotes latches the extra-usage notification once per day.
type extraUsageNotes struct {
	mu  sync.Mutex
	day string
}

// firstToday reports whether day has not been announced yet, and marks it.
func (n *extraUsageNotes) firstToday(day string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.day == day {
		return false
	}
	n.day = day
	return true
}

// ExtraUsageOn re-reads the subscription state; a suppressed warning counts as off.
func (b *Budget) ExtraUsageOn() bool {
	if b.settings.Load().SuppressExtraUsageWarning {
		return false
	}
	check := b.cfg.ExtraUsageCheck
	if check == nil {
		home := harness.LookupEnv(b.cfg.Env, "HOME")
		check = func() subscription.State { return subscription.CheckFresh(home) }
	}
	return check() == subscription.StateEnabled
}

// StartExtraUsageCheck is the server-start check. No item is enqueued, so it
// runs only when the global Default harness is Claude.
func (b *Budget) StartExtraUsageCheck(now time.Time) {
	if harnesschoice.DefaultOrClaude(b.settings.Load().AgentsDefaultHarness) == agent.AgentClaude {
		b.CheckExtraUsage(now)
	}
}

// CheckExtraUsage runs at server start and on every enqueue. When extra usage
// is on it sends at most one chat notice per day, and reports the state so the
// caller can hand the client its warning.
func (b *Budget) CheckExtraUsage(now time.Time) bool {
	if !b.ExtraUsageOn() {
		return false
	}
	if b.extraUsage.firstToday(now.Format(time.DateOnly)) {
		b.chat.Notice(ralphloop.ServerNotice{Kind: NoticeExtraUsage, Emoji: "⚠️", Title: "extra usage is on",
			Detail: "Your Claude account will auto-purchase extra usage once included usage runs out."})
	}
	return true
}
