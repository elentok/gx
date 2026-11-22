package main

import (
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
	r, err := rest.Open(f.stateD, broker.New(0), cfg, func() time.Time { return now },
		rest.WithBreakText(spec.RestBreakMessage, spec.RestBreakSubMessage))
	if err != nil {
		return err
	}
	for _, t := range sc.Rest.Active {
		now = t
		if _, err := r.Active(); err != nil {
			return err
		}
	}
	return nil
}
