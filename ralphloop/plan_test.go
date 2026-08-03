package ralphloop

import (
	"strings"
	"testing"

	"github.com/elentok/gx/tickets"
)

func displayNumbers(tickets []tickets.Ticket) []string {
	ids := make([]string, len(tickets))
	for i, t := range tickets {
		ids[i] = t.DisplayNumber()
	}
	return ids
}

func waveIDs(waves [][]tickets.Ticket) [][]string {
	out := make([][]string, len(waves))
	for i, w := range waves {
		out[i] = displayNumbers(w)
	}
	return out
}

func TestPlanWaves_CapsEachWaveAtMaxParallel(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "open"},
		{Number: 2, Identifier: "02", Status: "open", BlockedBy: []string{"01"}},
		{Number: 3, Identifier: "03", Status: "open"},
		{Number: 4, Identifier: "04", Status: "open"},
	}}
	scope, err := ResolveRunScope(epic, []string{"01", "02", "03", "04"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	waves, err := PlanWaves(epic, scope, 2)
	if err != nil {
		t.Fatalf("PlanWaves() error = %v", err)
	}
	got := waveIDs(waves)
	want := [][]string{{"01", "03"}, {"02", "04"}}
	if len(got) != len(want) {
		t.Fatalf("PlanWaves() = %v, want %v", got, want)
	}
	for i := range want {
		if strings.Join(got[i], ",") != strings.Join(want[i], ",") {
			t.Fatalf("PlanWaves() = %v, want %v", got, want)
		}
	}
}

func TestPlanWaves_MatchesScopeFrontierForFirstWave(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "open"},
		{Number: 2, Identifier: "02", Status: "open", BlockedBy: []string{"01"}},
	}}
	scope, err := ResolveRunScope(epic, []string{"01", "02"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	waves, err := PlanWaves(epic, scope, 2)
	if err != nil {
		t.Fatalf("PlanWaves() error = %v", err)
	}
	if len(waves) == 0 || strings.Join(displayNumbers(waves[0]), ",") != strings.Join(displayNumbers(scope.Frontier(epic)), ",") {
		t.Fatalf("PlanWaves() first wave = %v, want to match scope.Frontier() = %v", waveIDs(waves), displayNumbers(scope.Frontier(epic)))
	}
}

func TestPlanWaves_BlockerOutsideScopeThatIsNotDoneNeverRuns(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "open"},
		{Number: 2, Identifier: "02", Status: "open", BlockedBy: []string{"01"}},
	}}
	scope, err := ResolveRunScope(epic, []string{"02"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	_, err = PlanWaves(epic, scope, 2)
	if err == nil {
		t.Fatal("PlanWaves() error = nil, want an actionable error for the unresolved out-of-scope dependency")
	}
	for _, want := range []string{"delivery", "02"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("PlanWaves() error = %q, want it to contain %q", err, want)
		}
	}
}

func TestPlanWaves_BlockerOutsideScopeThatIsDoneRunsImmediately(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "open", BlockedBy: []string{"01"}},
	}}
	scope, err := ResolveRunScope(epic, []string{"02"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	waves, err := PlanWaves(epic, scope, 2)
	if err != nil {
		t.Fatalf("PlanWaves() error = %v", err)
	}
	if len(waves) != 1 || strings.Join(displayNumbers(waves[0]), ",") != "02" {
		t.Fatalf("PlanWaves() = %v, want [[02]]", waveIDs(waves))
	}
}

func TestPlanWaves_CycleAmongScopeTicketsReportsActionableError(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "open", BlockedBy: []string{"02"}},
		{Number: 2, Identifier: "02", Status: "open", BlockedBy: []string{"01"}},
	}}
	scope, err := ResolveRunScope(epic, []string{"01", "02"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	_, err = PlanWaves(epic, scope, 2)
	if err == nil {
		t.Fatal("PlanWaves() error = nil, want an actionable cycle error")
	}
	for _, want := range []string{"delivery", "01", "02"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("PlanWaves() error = %q, want it to contain %q", err, want)
		}
	}
}

func TestPlanWaves_AlreadyDoneScopeTicketNeverBlocks(t *testing.T) {
	epic := tickets.Epic{Name: "delivery", Tickets: []tickets.Ticket{
		{Number: 1, Identifier: "01", Status: "done"},
		{Number: 2, Identifier: "02", Status: "open", BlockedBy: []string{"01"}},
	}}
	scope, err := ResolveRunScope(epic, []string{"01", "02"})
	if err != nil {
		t.Fatalf("ResolveRunScope() error = %v", err)
	}

	waves, err := PlanWaves(epic, scope, 2)
	if err != nil {
		t.Fatalf("PlanWaves() error = %v", err)
	}
	if len(waves) != 1 {
		t.Fatalf("PlanWaves() = %v, want a single wave since 01 is already done", waveIDs(waves))
	}
	if strings.Join(displayNumbers(waves[0]), ",") != "01,02" {
		t.Fatalf("PlanWaves() first wave = %v, want [01 02]", displayNumbers(waves[0]))
	}
}
