package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server/wire"
)

func TestTerminal_ActionableReasonWinsOverStatusDetail(t *testing.T) {
	s := wire.SessionInfo{Status: wire.SessionStatus{State: wire.SessionBlocked, Kind: "permission", Message: "Bash"}}
	s.Actionable = wire.SessionActionable{Reason: "needs approval", Action: []string{"yes"}}
	got := ansi.Strip(InfoRow(s, time.Now()))
	if got != "needs approval  <leader> a: yes" {
		t.Fatalf("info row = %q", got)
	}
}

func TestTerminal_InfoRowShowsTheCountdownAndCancel(t *testing.T) {
	now := time.Now()
	at := now.Add(7 * time.Second)
	s := wire.SessionInfo{Status: wire.SessionStatus{State: wire.SessionIdle}}
	s.Actionable = wire.SessionActionable{Reason: "waiting", Action: []string{"Continue"}}
	s.AutoAdvanceAt = &at
	got := ansi.Strip(InfoRow(s, now))
	for _, want := range []string{"Continuing in 7s", "Cancel"} {
		if !strings.Contains(got, want) {
			t.Errorf("info row %q lacks %q", got, want)
		}
	}
}
