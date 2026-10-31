package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/elentok/gx/nativerunner"
)

// statuslineRateLimits is the rate_limits object of a statusline payload.
type statuslineRateLimits struct {
	FiveHour *nativerunner.UsageWindow `json:"five_hour"`
	SevenDay *nativerunner.UsageWindow `json:"seven_day"`
}

// parseStatuslineRateLimits is the one parse of a payload's rate_limits. It
// returns nil when the payload has none.
func parseStatuslineRateLimits(payload []byte) (*statuslineRateLimits, error) {
	var p struct {
		RateLimits *statuslineRateLimits `json:"rate_limits"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, err
	}
	return p.RateLimits, nil
}

// recordStatuslineUsage writes the payload's rate_limits into dir as a
// nativerunner.UsageRecord. A payload without rate_limits writes nothing, so
// an older record stays.
func recordStatuslineUsage(dir string, payload []byte, now time.Time) error {
	rl, err := parseStatuslineRateLimits(payload)
	if err != nil || rl == nil {
		return err
	}
	data, err := json.Marshal(nativerunner.UsageRecord{SeenAt: now, FiveHour: rl.FiveHour, SevenDay: rl.SevenDay})
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
