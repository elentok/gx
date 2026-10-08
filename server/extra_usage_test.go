package server

import (
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/subscription"
)

func TestExtraUsage_EnqueueWarnsAndNotifiesOncePerDay(t *testing.T) {
	s, wait := chatServer(t, 0, 0)
	s.cfg.ExtraUsageCheck = func() subscription.State { return subscription.StateEnabled }

	for i := 0; i < 2; i++ {
		res, err := s.queueReplace(QueueRequest{Project: "gx"})
		if err != nil || res.Refused {
			t.Fatalf("replace = %+v, %v", res, err)
		}
		if !res.ExtraUsage {
			t.Fatalf("replace %d: ExtraUsage = false, want the warning", i)
		}
	}
	got := wait(1)
	if len(got) != 1 || !strings.Contains(got[0], "extra usage") {
		t.Fatalf("sends = %v, want one extra-usage notice", got)
	}

	// A new day announces again.
	s.checkExtraUsage(time.Now().AddDate(0, 0, 1))
	if got := wait(2); len(got) != 2 {
		t.Fatalf("sends = %v, want a second notice the next day", got)
	}
}

func TestExtraUsage_OffOrSuppressedStaysQuiet(t *testing.T) {
	s, wait := chatServer(t, 0, 0)
	s.cfg.ExtraUsageCheck = func() subscription.State { return subscription.StateDisabled }
	if s.checkExtraUsage(time.Now()) {
		t.Fatal("disabled: warned")
	}
	s.cfg.ExtraUsageCheck = func() subscription.State { return subscription.StateEnabled }
	s.cfg.SuppressExtraUsageWarning = true
	if s.checkExtraUsage(time.Now()) {
		t.Fatal("suppressed: warned")
	}
	if got := wait(-1); len(got) != 0 {
		t.Fatalf("sends = %v, want none", got)
	}
}
