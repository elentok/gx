package rowactions

import (
	"strings"
	"testing"

	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/server/sessions/fixagent"
	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/tickets/internal/tickettest"
	"github.com/elentok/gx/ui/tickets/tabapi"
)

func TestInvestigate_PostsTheAddressAndLandsOnTheNewSession(t *testing.T) {
	var calls []string
	api := tickettest.FakeServerAPI{Calls: &calls, FixAgentRes: wire.FixAgentResult{SessionResult: wire.SessionResult{Session: &wire.SessionInfo{ID: "fix-1"}}}}

	got := InvestigateCmd(api, api, "gx:map/01")()

	if got != (tabapi.GoToSessionMsg{ID: "fix-1"}) {
		t.Fatalf("msg = %#v, want tabapi.GoToSessionMsg{fix-1}", got)
	}
	if len(calls) != 1 || calls[0] != "fix-agent gx:map/01" {
		t.Errorf("calls = %v, want one fix-agent post", calls)
	}
}

func TestInvestigate_NameTakenSelectsTheExistingFixAgent(t *testing.T) {
	api := tickettest.FakeServerAPI{
		FixAgentRes: wire.FixAgentResult{SessionResult: wire.SessionResult{Refused: true, Reason: wire.ReasonSessionNameTaken}},
		Snap: wire.Snapshot{Sessions: []wire.SessionInfo{
			{ID: "other", Name: "alpha"},
			{ID: "fix-1", Name: fixagent.FixAgentName("gx:map/01")},
		}},
	}

	got := InvestigateCmd(api, api, "gx:map/01")()

	if got != (tabapi.GoToSessionMsg{ID: "fix-1"}) {
		t.Fatalf("msg = %#v, want tabapi.GoToSessionMsg{fix-1}", got)
	}
}

func TestInvestigate_InvestigatingRefusalIsAToast(t *testing.T) {
	msg := "gx:map/01 is already being investigated in gx:map/05"
	api := tickettest.FakeServerAPI{FixAgentRes: wire.FixAgentResult{SessionResult: wire.SessionResult{Refused: true, Reason: wire.ReasonFixInvestigating, Message: msg}}}

	got := InvestigateCmd(api, api, "gx:map/01")()

	toast, ok := got.(notify.NotifyMsg)
	if !ok || toast.Kind != notify.KindError || !strings.Contains(toast.Message, msg) {
		t.Fatalf("msg = %#v, want a toast containing %q", got, msg)
	}
}
