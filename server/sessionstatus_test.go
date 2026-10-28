package server_test

import (
	"testing"

	"github.com/elentok/gx/server"
)

func TestSessionStatus_Promptable(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		st   server.SessionStatus
		want bool
	}{
		"idle":                  {server.SessionStatus{State: server.SessionIdle}, true},
		"turn over, shell left": {server.SessionStatus{State: server.SessionWorking, Background: true}, true},
		"working":               {server.SessionStatus{State: server.SessionWorking}, false},
		"blocked":               {server.SessionStatus{State: server.SessionBlocked}, false},
		"starting":              {server.SessionStatus{State: server.SessionStarting}, false},
	} {
		if got := c.st.Promptable(); got != c.want {
			t.Errorf("%s: Promptable = %v, want %v", name, got, c.want)
		}
	}
}
