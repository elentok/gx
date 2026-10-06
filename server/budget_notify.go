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
type budgetNotes struct {
	mu   sync.Mutex
	day  string
	soft bool
	hard bool
}

func (b *budgetNotes) seed(day string, total, soft, hard float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.day = day
	b.soft = soft > 0 && total >= soft
	b.hard = hard > 0 && total >= hard
}

func (s *Server) seedBudgetNotes(now time.Time) {
	st := s.budgetStatus(now)
	s.budgetNotes.seed(st.Day, st.Total, st.SoftLimit, st.HardLimit)
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
	softNow := !b.soft && st.SoftLimit > 0 && st.Total >= st.SoftLimit
	hardNow := !b.hard && st.HardLimit > 0 && st.Total >= st.HardLimit
	b.soft = b.soft || softNow
	b.hard = b.hard || hardNow
	b.mu.Unlock()

	if prevDay != "" && prevDay != st.Day {
		s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeDailySummary, Emoji: "📊", Title: "daily summary",
			Detail: fmt.Sprintf("%s: %s spent", prevDay, tickets.FormatCost(s.ledger.day(prevDay)))})
	}
	if softNow {
		s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeBudgetSoft, Emoji: "💸", Title: "budget soft limit reached",
			Detail: fmt.Sprintf("%s of %s today", tickets.FormatCost(st.Total), tickets.FormatCost(st.SoftLimit))})
	}
	if hardNow {
		s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeBudgetHard, Emoji: "🛑", Title: "budget hard limit reached",
			Detail: fmt.Sprintf("%s of %s today", tickets.FormatCost(st.Total), tickets.FormatCost(st.HardLimit))})
	}
}
