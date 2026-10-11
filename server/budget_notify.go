package server

import (
	"fmt"
	"sync"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/tickets"
)

const (
	NoticeServerStarted = "server-started"
	NoticeBudgetSoft    = "budget-soft-limit"
	NoticeBudgetHard    = "budget-hard-limit"
	NoticeDailySummary  = "daily-summary"
)

// budgetNotes latches each limit once per budget day, so a limit is announced
// when crossed and not on every poll after. State is in memory: seed marks what
// is already crossed at startup, so a restart does not announce it again.
// Like the budget latches, limits count only spend beyond today's override.
type budgetNotes struct {
	mu   sync.Mutex
	day  string
	soft bool
	hard bool
}

func (b *budgetNotes) seed(day string, spent, soft, hard float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.day = day
	b.soft = soft > 0 && spent >= soft
	b.hard = hard > 0 && spent >= hard
}

// rearm forgets each announced limit that a raise or an override put back
// above spent, so crossing it again is announced again.
func (b *budgetNotes) rearm(spent, soft, hard float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.soft = b.soft && spent >= soft
	b.hard = b.hard && spent >= hard
}

// markHard counts the hard limit as announced today.
func (b *budgetNotes) markHard() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hard = true
}

func (s *Server) seedBudgetNotes(now time.Time) {
	st := s.budgetStatus(now)
	s.budgetNotes.seed(st.Day, st.spent(), st.SoftLimit, st.HardLimit)
}

func (s *Server) rearmBudgetNotes(st BudgetStatus) {
	s.budgetNotes.rearm(st.spent(), st.SoftLimit, st.HardLimit)
}

// notifyBudget sends the day's summary when the budget day rolls over, then a
// notice for each limit newly crossed today.
func (s *Server) notifyBudget(now time.Time) {
	st := s.budgetStatus(now)
	b := &s.budgetNotes
	b.mu.Lock()
	prevDay := b.day
	if st.Day != prevDay {
		b.day, b.soft, b.hard = st.Day, false, false
	}
	spent := st.spent()
	softNow := !b.soft && st.SoftLimit > 0 && spent >= st.SoftLimit
	// The latch, not the limit: a hard limit held after a reload has stopped nothing.
	hardNow := !b.hard && st.HardLatched
	b.soft = b.soft || softNow
	b.hard = b.hard || hardNow
	b.mu.Unlock()

	if prevDay != "" && prevDay != st.Day {
		s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeDailySummary, Emoji: "📊", Title: "daily summary",
			Detail: fmt.Sprintf("%s: %s spent", prevDay, tickets.FormatCost(s.ledger.day(prevDay)))})
	}
	if softNow {
		s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeBudgetSoft, Emoji: "💸", Title: "budget soft limit reached",
			Detail: fmt.Sprintf("%s of %s today", tickets.FormatCost(spent), tickets.FormatCost(st.SoftLimit))})
	}
	if hardNow {
		s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeBudgetHard, Emoji: "🛑", Title: "budget hard limit reached",
			Detail: fmt.Sprintf("%s of %s today", tickets.FormatCost(spent), tickets.FormatCost(st.HardLimit))})
	}
}
