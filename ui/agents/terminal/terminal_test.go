package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/ui/agents/agentstyle"
)

func TestTerminal_MaybeSentRowIsOneYellowLineWithTheHint(t *testing.T) {
	s := wire.SessionInfo{Actionable: wire.SessionActionable{
		Kind: wire.DeliveryMaybeSent, Reason: "ignored reason text", CopyText: "/model\nopus",
	}}
	row := InfoRow(s, time.Now())
	if got, want := ansi.Strip(row), "May not have been sent: /model opus  see the screen · <leader> y: copy · no resend"; got != want {
		t.Fatalf("info row = %q, want %q", got, want)
	}
	want := agentstyle.Detail.Render("May not have been sent: /model opus") + agentstyle.Action.Render("  see the screen · <leader> y: copy · no resend")
	if row != want {
		t.Fatalf("styled row = %q, want %q", row, want)
	}
}

func TestTerminal_SendFailedRowKeepsTheRedReasonAndActionHint(t *testing.T) {
	s := wire.SessionInfo{Actionable: wire.SessionActionable{
		Kind: wire.DeliverySendFailed, Reason: "Send failed", Action: []string{"/model opus"}, CopyText: "/model opus",
	}}
	row := InfoRow(s, time.Now())
	if got := ansi.Strip(row); got != "Send failed  <leader> a: /model opus" {
		t.Fatalf("info row = %q", got)
	}
	want := agentstyle.Reason.Render("Send failed") + agentstyle.Action.Render("  <leader> a: /model opus")
	if row != want {
		t.Fatalf("styled row = %q, want %q", row, want)
	}
}

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
