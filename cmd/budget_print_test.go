package cmd

import (
	"strings"
	"testing"

	"github.com/elentok/gx/server"
)

func TestPrintBudget_ShowsOverrideAndWhatIsLeft(t *testing.T) {
	t.Parallel()
	var out strings.Builder
	b := server.BudgetStatus{Day: "2026-10-08", Total: 304.19, SoftLimit: 300, HardLimit: 350, Override: 301.22,
		Projects: map[string]float64{"gx": 255.64}}
	if err := printBudget(&out, b); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"overridden at", "$301.22", "$2.97 counts toward the limits", "$297.03 left", "$347.03 left", "(untracked)", "$48.55"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestBudgetLine_NamesOverride(t *testing.T) {
	t.Parallel()
	got := budgetLine(server.BudgetStatus{Total: 304.19, SoftLimit: 300, Override: 301.22})
	if !strings.Contains(got, "overridden at $301.22") {
		t.Errorf("line = %q, want the override point", got)
	}
}
