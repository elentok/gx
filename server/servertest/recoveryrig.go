package servertest

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/tickets"
)

// StartInvestigateEnv starts a server whose ticket 01 is paused with kind and
// whose ticket 02 is the investigate ticket forked off it.
func StartInvestigateEnv(t *testing.T, kind events.Kind) *Harness {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	WriteTicketWith(t, store, "proj", "epic-a", "02", "probe", TicketOpts{Type: "investigate", Parent: "01"})
	SetProjectRepo(t, store, "proj", repo)
	h := StartWithStore(t, store, func(*server.Config) {})
	RegisterTurn(h)
	if err := h.Server.PauseAs("proj:epic-a/01", kind, "boom"); err != nil {
		t.Fatal(err)
	}
	return h
}

// GitIn runs git in repo and returns its trimmed output.
func GitIn(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// EpicTickets loads the one epic of project proj in h's store.
func EpicTickets(t *testing.T, h *Harness) tickets.Epic {
	t.Helper()
	epics, err := tickets.Load(filepath.Join(h.TicketStore, "proj"))
	if err != nil || len(epics) != 1 {
		t.Fatalf("load = %v, %v", epics, err)
	}
	return epics[0]
}
