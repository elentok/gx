package main

import (
	"os"
	"path/filepath"
)

const (
	// calmHealth is well below every threshold, so a scenario that does not steer
	// the meters never shows a warning.
	calmHealth = "cpu=20 mem=50 pressure=none"
	// healthInterval is short so the 12-reading average settles in about a second.
	healthInterval = "100ms"
)

// healthDir sits outside the project repo, so the trunk stays clean.
func (f *fixture) healthDir() string { return filepath.Join(f.root, "health") }

// setupHealth gives every scenario fake CPU and memory values: the server reads
// them from <health dir>/system, which starts at calmHealth.
func (f *fixture) setupHealth() error {
	if err := os.MkdirAll(f.healthDir(), 0o755); err != nil {
		return err
	}
	system := filepath.Join(f.healthDir(), "system")
	if err := os.WriteFile(system, []byte(calmHealth+"\n"), 0o644); err != nil {
		return err
	}
	f.env = append(f.env, "GX_HEALTH_FILE="+system, "GX_HEALTH_INTERVAL="+healthInterval)
	return nil
}
