package cmd

import (
	"testing"

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
