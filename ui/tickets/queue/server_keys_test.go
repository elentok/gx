package queue

import (
	"testing"

	"github.com/elentok/gx/server/wire"
)

// Seam D: "p" offers override when a budget latch holds, resume when paused,
// pause otherwise.
func TestServerPauseActionFor(t *testing.T) {
	for _, tc := range []struct {
		name   string
		budget wire.BudgetStatus
		paused bool
		want   serverPauseAction
	}{
		{"running", wire.BudgetStatus{}, false, serverActionPause},
		{"paused", wire.BudgetStatus{}, true, serverActionResume},
		{"soft latched", wire.BudgetStatus{BudgetPaused: true}, false, serverActionOverride},
		{"hard latched beats paused", wire.BudgetStatus{HardLatched: true}, true, serverActionOverride},
	} {
		if got := serverPauseActionFor(tc.budget, tc.paused); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
