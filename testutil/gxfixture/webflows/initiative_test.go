package webflows

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/elentok/gx/testutil/gxfixture/fixturetest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

func TestInitiativeScenario_SeedsAndListsTheInitiative(t *testing.T) {
	t.Parallel()
	for scenario, want := range map[string][]string{
		"initiative-fresh":           {spec.InitiativeSlug},
		"initiative-m1-implementing": {spec.InitiativeSlug},
		"initiative-m1-gated":        {spec.InitiativeSlug},
		"initiative-broken-links":    {spec.InitiativeBrokenSlug},
		"initiative-stopped":         {spec.InitiativeSlug},
		"initiative-finished":        {spec.InitiativeSlug},
		"initiative-list":            {spec.InitiativeSlug, spec.InitiativeStoppedSlug},
		"initiative-epics-cards":     {spec.InitiativeSlug},
		"initiative-gate-ready":      {spec.InitiativeSlug},
		"initiative-resume":          {spec.InitiativeSlug},
	} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			r := start(t, scenario)
			defer r.Stop(t)
			cmd := exec.Command(fixturetest.GxBin, "tickets", "epics", "--initiatives")
			cmd.Env, cmd.Dir = r.Env, r.Repo()
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("gx tickets epics --initiatives: %v\n%s", err, out)
			}
			got := strings.Fields(string(out))
			slices.Sort(got)
			if !slices.Equal(got, want) {
				t.Errorf("Initiatives = %v, want %v", got, want)
			}
		})
	}
}
