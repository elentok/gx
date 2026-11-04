package budget

import (
	"fmt"
	"sync"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/tickets"
)

const (
	NoticeBudgetSoft   = "budget-soft-limit"
	NoticeBudgetHard   = "budget-hard-limit"
	NoticeDailySummary = "daily-summary"
)

// notes latches each limit once per budget day, so a limit is announced
// when crossed and not on every poll after. State is in memory: seed marks what
// is already crossed at startup, so a restart does not announce it again.
// Like the budget latches, limits count only spend beyond today's override.
type notes struct {
	mu   sync.Mutex
	day  string
	soft bool
	hard bool
}

func (n *notes) seed(day string, spent, soft, hard float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.day = day
	n.soft = soft > 0 && spent >= soft
	n.hard = hard > 0 && spent >= hard
}

// rearm forgets each announced limit that a raise or an override put back
// above spent, so crossing it again is announced again.
func (n *notes) rearm(spent, soft, hard float64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.soft = n.soft && spent >= soft
	n.hard = n.hard && spent >= hard
}

// markHard counts the hard limit as announced today.
func (n *notes) markHard() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.hard = true
}

// SeedNotes marks the limits already crossed at startup as announced.
func (b *Budget) SeedNotes(now time.Time) {
	st := b.Status(now)
	b.notes.seed(st.Day, spent(st), st.SoftLimit, st.HardLimit)
}

func (b *Budget) rearmNotes(st wire.BudgetStatus) {
	b.notes.rearm(spent(st), st.SoftLimit, st.HardLimit)
}

// notify sends the day's summary when the budget day rolls over, then a
// notice for each limit newly crossed today.
func (b *Budget) notify(now time.Time) {
	st := b.Status(now)
	n := &b.notes
	n.mu.Lock()
	prevDay := n.day
	if st.Day != prevDay {
		n.day, n.soft, n.hard = st.Day, false, false
	}
	used := spent(st)
	softNow := !n.soft && st.SoftLimit > 0 && used >= st.SoftLimit
	// The latch, not the limit: a hard limit held after a reload has stopped nothing.
	hardNow := !n.hard && st.HardLatched
	n.soft = n.soft || softNow
	n.hard = n.hard || hardNow
	n.mu.Unlock()

	if prevDay != "" && prevDay != st.Day {
		b.chat.Notice(ralphloop.ServerNotice{Kind: NoticeDailySummary, Emoji: "📊", Title: "daily summary",
			Detail: fmt.Sprintf("%s: %s spent", prevDay, tickets.FormatCost(b.ledger.day(prevDay)))})
	}
	if softNow {
		b.chat.Notice(ralphloop.ServerNotice{Kind: NoticeBudgetSoft, Emoji: "💸", Title: "budget soft limit reached",
			Detail: fmt.Sprintf("%s of %s today", tickets.FormatCost(used), tickets.FormatCost(st.SoftLimit))})
	}
	if hardNow {
		b.chat.Notice(ralphloop.ServerNotice{Kind: NoticeBudgetHard, Emoji: "🛑", Title: "budget hard limit reached",
			Detail: fmt.Sprintf("%s of %s today", tickets.FormatCost(used), tickets.FormatCost(st.HardLimit))})
	}
}
