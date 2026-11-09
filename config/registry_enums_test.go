package config

import (
	"slices"
	"testing"

	"github.com/elentok/gx/config/registry"
)

// The registry cannot import config, so its enum lists are pinned here to
// what the loader accepts.
func TestRegistryEnumsMatchLoader(t *testing.T) {
	for _, v := range registry.HarnessChoices {
		if err := validateDefaultHarness(v); err != nil {
			t.Errorf("harness %q: %v", v, err)
		}
	}
	if !slices.Equal(registry.HarnessChoices, []string{HarnessClaude, HarnessCodex}) {
		t.Errorf("harness choices = %v", registry.HarnessChoices)
	}
	if !slices.Equal(registry.EvolveChoices, []string{EvolveOff, EvolveMaintainer, EvolveContributor}) {
		t.Errorf("evolve choices = %v", registry.EvolveChoices)
	}
	for _, v := range registry.EvolveChoices {
		if !validEvolveMode(v) {
			t.Errorf("evolve %q rejected by loader", v)
		}
	}
}
