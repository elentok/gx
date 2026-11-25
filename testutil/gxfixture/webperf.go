package main

import (
	"fmt"
	"time"

	"github.com/elentok/gx/testutil/gxfixture/perfdata"
	"github.com/elentok/gx/webperf"
)

// seedWebPerf writes the samples through the store, each day's batch received at noon of that past day.
func seedWebPerf(stateDir string, days []perfdata.WebPerfDay) error {
	now := time.Now()
	store, err := webperf.Open(stateDir, time.Now)
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
