package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/config"
	"github.com/elentok/gx/server/broker"
	"github.com/elentok/gx/server/rest"
	"github.com/elentok/gx/testutil/gxfixture/spec"
)

// seedRest writes the scenario's Rest state and records by driving the
// server's own runner over the seed's heartbeat times on a stepped clock, so
// the server reads what its own writes would have made. The runner is never
// started, so no timer is armed.
func seedRest(f *fixture, sc scenario) error {
	if sc.Rest == nil {
		return nil
	}
	var now time.Time
	cfg := func() config.MentalHealthConfig { return sc.Rest.Config }
	r, err := rest.Open(f.stateD, broker.New(0), cfg, func() time.Time { return now })
	if err != nil {
		return err
	}
	for _, t := range sc.Rest.Active {
		now = t
		if _, err := r.Active(); err != nil {
			return err
		}
	}
	return setBreakText(filepath.Join(f.stateD, "rest.json"))
}

// setBreakText replaces the stored Break text with the mock's. The engine
// chooses its own text, which no setting can make the mock's two lines, so the
// persisted period is the one thing patched after the runner wrote it.
func setBreakText(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var state map[string]any
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	period, ok := state["period"].(map[string]any)
	if !ok {
		return nil
	}
	period["message"], period["sub_message"] = spec.RestBreakMessage, spec.RestBreakSubMessage
	data, err = json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
