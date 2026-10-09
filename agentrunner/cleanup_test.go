package agentrunner_test

import (
	"testing"

	"github.com/elentok/gx/agentrunner"
)

type cleaningRunner struct {
	agentrunner.Runner
	cleaned []string
}

func (c *cleaningRunner) Cleanup(label string) error {
	c.cleaned = append(c.cleaned, label)
	return nil
}

func TestCleanup_CallsRunnersThatCleanAndSkipsTheRest(t *testing.T) {
	c := &cleaningRunner{}
	if err := agentrunner.Cleanup(c, "e-01"); err != nil || len(c.cleaned) != 1 || c.cleaned[0] != "e-01" {
		t.Fatalf("Cleanup = %v, cleaned %v; want e-01", err, c.cleaned)
	}
	if err := agentrunner.Cleanup(struct{ agentrunner.Runner }{}, "e-01"); err != nil {
		t.Fatalf("Cleanup on a runner without leftovers = %v, want nil", err)
	}
}
