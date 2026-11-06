package migrations

import (
	"slices"
	"testing"
)

func TestLaunchStampStep_RegisteredAndBackfillsNothing(t *testing.T) {
	if !slices.Contains(Default.Names(), "launch-stamp") {
		t.Fatalf("launch-stamp not registered in %v", Default.Names())
	}
	r := &Registry{}
	for _, s := range Default.steps {
		if s.Name == "launch-stamp" {
			r.Register(s)
		}
	}
	changes, _, err := r.Run(Env{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none", changes)
	}
}
