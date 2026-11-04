package server_test

import (
	"testing"

	"github.com/elentok/gx/server/wire"
)

func TestSessionStatus_Promptable(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		st   wire.SessionStatus
		want bool
	}{
		"idle":                  {wire.SessionStatus{State: wire.SessionIdle}, true},
		"turn over, shell left": {wire.SessionStatus{State: wire.SessionWorking, Background: true}, true},
		"working":               {wire.SessionStatus{State: wire.SessionWorking}, false},
		"blocked":               {wire.SessionStatus{State: wire.SessionBlocked}, false},
		"starting":              {wire.SessionStatus{State: wire.SessionStarting}, false},
	} {
		if got := c.st.Promptable(); got != c.want {
			t.Errorf("%s: Promptable = %v, want %v", name, got, c.want)
		}
	}
}
