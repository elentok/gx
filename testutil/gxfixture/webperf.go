package main

import (
	"fmt"
	"time"

	"github.com/elentok/gx/testutil/gxfixture/spec"
	"github.com/elentok/gx/webperf"
)

// seedWebPerf writes the scenario's samples through the store, each day's
// batch received at noon of that past local day. The server loads them at start.
func (f *fixture) seedWebPerf(days []spec.WebPerfDay) error {
	now := time.Now()
	store, err := webperf.Open(f.stateD, time.Now)
	if err != nil {
		return err
	}
	defer store.Close()
	for _, d := range days {
		received := time.Date(now.Year(), now.Month(), now.Day()-d.DaysAgo, 12, 0, 0, 0, time.Local)
		for i := range d.Samples {
			d.Samples[i].Timestamp = received.UnixMilli()
		}
		_, dropped, err := store.Append(received, d.Samples)
		if err != nil {
			return err
		}
		if dropped > 0 {
			return fmt.Errorf("day %d: the store dropped %d samples", d.DaysAgo, dropped)
		}
	}
	return nil
}
