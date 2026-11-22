package recovery

import (
	"testing"

	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

func TestLaunchSkill_PerTicketType(t *testing.T) {
	cases := []struct {
		typ  schema.TicketType
		want string
	}{
		{schema.TypeCodeReview, "gx-code-review"},
		{schema.TypeVerify, "gx-verify-epic"},
		{schema.TypePrompt, "gx-one-off"},
		{schema.TypeTrunkConflict, "gx-one-off"},
		{schema.TypeEvolve, "gx-evolve"},
		{schema.TypeInvestigate, "gx-investigate"},
		{schema.TypeDiagnose, "gx-investigate"},
		{schema.TypeMerge, "gx-merge --ticket"},
		{schema.TypePRText, "gx-pr-text"},
		{schema.TypeCIFix, "gx-ci-fix"},
		{schema.TypeImplement, "gx-implement"},
	}
	for _, c := range cases {
		t.Run(string(c.typ), func(t *testing.T) {
			if got := LaunchSkill(tickets.Ticket{Type: string(c.typ)}); got != c.want {
				t.Errorf("LaunchSkill(%s) = %q, want %q", c.typ, got, c.want)
			}
		})
	}
}
