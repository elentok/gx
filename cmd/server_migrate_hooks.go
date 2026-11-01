package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/tickets"
)

// Test hooks for the start-up migration, set by gxfixture. Both are inert when
// unset: the server then runs migrations.Default as released.
const (
	// migrateHoldEnv names a file. While it exists, a step ahead of the real
	// ones waits, so the migration stays open until the file is removed.
	migrateHoldEnv = "GX_MIGRATE_HOLD_FILE"
	// migrateInjectEnv names a JSON control file, read once at start, with a
	// list of extra steps run after the real ones. A missing file adds none.
	migrateInjectEnv = "GX_MIGRATE_INJECT_FILE"
)

// injectedStep is one entry of the inject control file: a step that fails with
// Fail, or else returns Notes as skip notes on one change.
type injectedStep struct {
	Name  string   `json:"name"`
	Fail  string   `json:"fail,omitempty"`
	Notes []string `json:"notes,omitempty"`
}

// holdStep waits, for each project, until holdFile is gone.
func holdStep(holdFile string) migrations.Step {
	return migrations.Step{Name: "test-hold-open", Run: func(migrations.Env) ([]tickets.MigrationChange, error) {
		for {
			if _, err := os.Stat(holdFile); errors.Is(err, os.ErrNotExist) {
				return nil, nil
			}
			time.Sleep(50 * time.Millisecond)
		}
	}}
}

// migrationHooks returns the registry the env hooks ask for, or nil when none
// is set. getenv is os.Getenv in production.
func migrationHooks(getenv func(string) string) (*migrations.Registry, error) {
	holdFile, injectFile := getenv(migrateHoldEnv), getenv(migrateInjectEnv)
	if holdFile == "" && injectFile == "" {
		return nil, nil
	}
	var injected []injectedStep
	if injectFile != "" {
		raw, err := os.ReadFile(injectFile)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s: %w", migrateInjectEnv, err)
		}
		if err == nil {
			if err := json.Unmarshal(raw, &injected); err != nil {
				return nil, fmt.Errorf("%s: %w", migrateInjectEnv, err)
			}
		}
	}
	reg := &migrations.Registry{}
	if holdFile != "" {
		reg.Register(holdStep(holdFile))
	}
	reg.Extend(migrations.Default)
	for _, in := range injected {
		reg.Register(migrations.Step{Name: in.Name, Run: func(e migrations.Env) ([]tickets.MigrationChange, error) {
			if in.Fail != "" {
				return nil, errors.New(in.Fail)
			}
			if len(in.Notes) == 0 {
				return nil, nil
			}
			return []tickets.MigrationChange{{Path: e.Root, Notes: in.Notes}}, nil
		}})
	}
	return reg, nil
}
