package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/elentok/gx/transcript"
)

const (
	ledgerFile = "budget-ledger.json"
	// spendPollInterval is short so a usage line appended by a flow shows within a second or two.
	spendPollInterval = "1s"
)

// spendShapes are the --shape values of the spend scenario. Each returns the
// seeded ledger days (key 2006-01-02 → cost) from the real clock.
var spendShapes = map[string]func(now time.Time) map[string]float64{
	// normal: 40 days through today, so every window is fully covered.
	"normal": func(now time.Time) map[string]float64 {
		days := map[string]float64{}
		for n := 1; n <= 40; n++ {
			days[dayKey(now, n)] = 3.00
		}
		days[dayKey(now, 0)] = 4.82
		return days
	},
	// short: today is the first ledger day.
	"short": func(now time.Time) map[string]float64 {
		return map[string]float64{dayKey(now, 0): 4.82}
	},
	// empty: no days at all.
	"empty": func(time.Time) map[string]float64 { return map[string]float64{} },
}

func spendShapeNames() []string {
	names := make([]string, 0, len(spendShapes))
	for n := range spendShapes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func dayKey(now time.Time, daysAgo int) string {
	return now.AddDate(0, 0, -daysAgo).Format("2006-01-02")
}

// seedSpend writes the budget ledger the server loads at start.
func (f *fixture) seedSpend(shape string) error {
	data, err := json.Marshal(map[string]any{
		"days": spendShapes[shape](time.Now()),
		"seen": map[string]any{},
	})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(f.stateD, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(f.stateD, ledgerFile), data, 0o600)
}

// agentTranscript is where the held fake agent's transcript belongs; a flow
// appends usage lines there and the ledger poll prices them.
func (f *fixture) agentTranscript(ctx context.Context) (string, error) {
	m, err := f.waitAgentMeta(ctx)
	if err != nil {
		return "", err
	}
	return transcript.PathIn(filepath.Join(f.root, "home"), m.Cwd, m.SessionID), nil
}
