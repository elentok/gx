package main

import "testing"

func TestRecoveryScenarios_EachStartsAndPrintsItsJSON(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"hung-pane-twice", "hung-pane-once", "hung-pane-twice-codex", "codex-drops-prompt", "verify-notes", "summary-notes", "map-evolve", "live-task-gate", "live-task-gate-codex", "live-task-gate-dirty", "slow-holder", "classifier-outage"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := start(t, name)
			if r.WebURL == "" {
				t.Errorf("%s printed no web_url: %+v", name, r)
			}
			r.stop(t)
		})
	}
}
