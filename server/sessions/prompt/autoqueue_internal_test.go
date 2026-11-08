package prompt

import (
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestEpicState_FollowUpsEpicIsNotReady(t *testing.T) {
	t.Parallel()
	s := epicState{step: tickets.StepBranch, impl: tickets.Epic{Name: tickets.FollowUpsEpic, Status: "open"}}
	if got := s.notReady(); !strings.Contains(got, "follow-ups") {
		t.Errorf("reason = %q, want follow-ups", got)
	}
}
