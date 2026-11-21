package catalog

import (
	"slices"
	"testing"
)

func TestInitiativeScenario_RegisteredByName(t *testing.T) {
	t.Parallel()
	names := Names()
	for _, name := range []string{"initiative-fresh", "initiative-m1-implementing", "initiative-m1-gated", "initiative-broken-links"} {
		if _, ok := Scenarios[name]; !ok {
			t.Errorf("scenario %q is not registered", name)
		}
		if !slices.Contains(names, name) {
			t.Errorf("Names() lacks %q", name)
		}
	}
}
