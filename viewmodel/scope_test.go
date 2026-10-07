package viewmodel_test

import (
	"testing"

	"github.com/elentok/gx/server"
	"github.com/elentok/gx/viewmodel"
)

func scopeSnapshot() server.Snapshot {
	return server.Snapshot{Tickets: []server.TicketInfo{
		{Address: "alpha:e/01"},
		{Address: "beta:e/01"},
	}}
}

func addresses(ts []server.TicketInfo) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Address)
	}
	return out
}

func TestScopedTickets_DefaultsToCwdProjectAndTogglesToAll(t *testing.T) {
	vm := viewmodel.State{CwdProject: "alpha"}.ApplySnapshot(scopeSnapshot())

	if got := addresses(vm.ScopedTickets()); len(got) != 1 || got[0] != "alpha:e/01" {
		t.Fatalf("scoped = %v, want only alpha", got)
	}
	if got := addresses(vm.ToggleAllProjects().ScopedTickets()); len(got) != 2 {
		t.Fatalf("all = %v, want both projects", got)
	}
	if vm.UnregisteredHint() != "" {
		t.Fatal("registered project must not show the hint")
	}
}

func TestScopedTickets_OutsideProjectShowsAllWithHint(t *testing.T) {
	vm := viewmodel.State{}.ApplySnapshot(scopeSnapshot())

	if got := addresses(vm.ScopedTickets()); len(got) != 2 {
		t.Fatalf("scoped = %v, want all", got)
	}
	if vm.UnregisteredHint() == "" {
		t.Fatal("expected the unregistered hint")
	}
}

func TestApplySnapshot_KeepsScope(t *testing.T) {
	vm := viewmodel.State{CwdProject: "alpha"}.ToggleAllProjects().ApplySnapshot(scopeSnapshot())
	if vm.CwdProject != "alpha" || !vm.AllProjects {
		t.Fatalf("scope lost across snapshot: %+v", vm)
	}
}
