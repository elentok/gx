package server

import (
	"sync"
	"time"

	"github.com/elentok/gx/ralphloop"
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

// extraUsageOn re-reads the subscription state; a suppressed warning counts as off.
func (s *Server) extraUsageOn() bool {
	if s.cfg.SuppressExtraUsageWarning {
		return false
	}
	check := s.cfg.ExtraUsageCheck
	if check == nil {
		check = subscription.CheckFresh
	}
	return check() == subscription.StateEnabled
}

// checkExtraUsage runs at server start and on every enqueue. When extra usage
// is on it sends at most one chat notice per day, and reports the state so the
// caller can hand the client its warning.
func (s *Server) checkExtraUsage(now time.Time) bool {
	if !s.extraUsageOn() {
		return false
	}
	if s.extraUsage.firstToday(now.Format(time.DateOnly)) {
		s.chat.Notice(ralphloop.ServerNotice{Kind: NoticeExtraUsage, Emoji: "⚠️", Title: "extra usage is on",
			Detail: "Your Claude account will auto-purchase extra usage once included usage runs out."})
	}
	return true
}
