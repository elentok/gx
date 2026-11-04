package servertest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
)

// WithUsageSample seeds the persisted Claude usage sample the server loads at start.
func WithUsageSample(t *testing.T, rec nativerunner.UsageRecord) func(*server.Config) {
	t.Helper()
	return withSampleFile(t, "usage-sample.json", rec)
}

// WithCodexUsageSample seeds the persisted Codex usage sample.
func WithCodexUsageSample(t *testing.T, rec nativerunner.UsageRecord) func(*server.Config) {
	t.Helper()
	return withSampleFile(t, "usage-sample-codex.json", rec)
}

func withSampleFile(t *testing.T, name string, rec nativerunner.UsageRecord) func(*server.Config) {
	return func(c *server.Config) {
		data, err := json.Marshal(rec)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(c.StateDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(c.StateDir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
