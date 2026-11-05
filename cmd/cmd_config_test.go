package cmd

import (
	"testing"
	"time"

	"github.com/elentok/gx/config"
)

// TestSettingsFromConfigIncludesExecutionQueueLimits stays in root until `tui`
// takes settingsFromConfig (cmd/ui.go).
func TestSettingsFromConfigIncludesExecutionQueueLimits(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	cfg.ExecutionQueue.MaxConcurrentTicketsPerEpic = 3
	cfg.ExecutionQueue.MaxConcurrentEpics = 4

	settings := settingsFromConfig(config.Dirs{}, cfg)
	if got := settings.MaxConcurrentTicketsPerEpic(); got != 3 {
		t.Fatalf("MaxConcurrentTicketsPerEpic() = %d, want 3", got)
	}
	if got := settings.MaxConcurrentEpics(); got != 4 {
		t.Fatalf("MaxConcurrentEpics() = %d, want 4", got)
	}
}

// The relativeDate tests stay in root until `gitops` takes the push code.
func TestRelativeDate_Zero(t *testing.T) {
	t.Parallel()
	got := relativeDate(time.Time{})
	if got != "unknown time" {
		t.Fatalf("relativeDate(zero) = %q, want %q", got, "unknown time")
	}
}

func TestRelativeDate_NonZero(t *testing.T) {
	t.Parallel()
	got := relativeDate(time.Now().Add(-time.Hour))
	if got == "" || got == "unknown time" {
		t.Fatalf("relativeDate(now-1h) = %q, expected a non-empty relative string", got)
	}
}
