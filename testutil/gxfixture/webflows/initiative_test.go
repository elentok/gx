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
	for scenario, slug := range map[string]string{
		"initiative-fresh":           spec.InitiativeSlug,
		"initiative-m1-implementing": spec.InitiativeSlug,
		"initiative-m1-gated":        spec.InitiativeSlug,
		"initiative-broken-links":    spec.InitiativeBrokenSlug,
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
			if got := strings.Fields(string(out)); !slices.Equal(got, []string{slug}) {
				t.Errorf("Initiatives = %v, want [%s]", got, slug)
			}
		})
	}
}
