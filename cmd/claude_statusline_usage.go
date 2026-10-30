package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/nativerunner"
)

// recordStatuslineUsage writes the payload's rate_limits into dir as a
// nativerunner.UsageRecord. A payload without rate_limits writes nothing, so
// an older record stays.
func recordStatuslineUsage(dir string, payload []byte, now time.Time) error {
	var p struct {
		RateLimits *struct {
			FiveHour *nativerunner.UsageWindow `json:"five_hour"`
			SevenDay *nativerunner.UsageWindow `json:"seven_day"`
		} `json:"rate_limits"`
	}
	if err := json.Unmarshal(payload, &p); err != nil || p.RateLimits == nil {
		return err
	}
	data, err := json.Marshal(nativerunner.UsageRecord{SeenAt: now, FiveHour: p.RateLimits.FiveHour, SevenDay: p.RateLimits.SevenDay})
	if err != nil {
		return err
	}
	// Rename keeps a reader from seeing a half-written record.
	tmp, err := os.CreateTemp(dir, nativerunner.UsageFile+".*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		if werr != nil {
			return werr
		}
		return cerr
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, nativerunner.UsageFile)); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}
