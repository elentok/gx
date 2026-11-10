package migrations

import (
	"time"

	"github.com/elentok/gx/ralphloop/metrics"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketmigrate"
)

func init() {
	// The pause plan runs first and its apply last (see tickets/migrate_pause.go).
	// The question-section step still looks for "## Needs Answer", so it runs
	// before the apply.
	Default.Register(Step{Name: "pause-and-question-sections", Append: true, Run: pauseUnit})
	Default.Register(Step{Name: "backfill-merged-at", Append: true, Run: func(e Env) ([]tickets.MigrationChange, error) {
		return ticketmigrate.BackfillMergedAt(e.Root, epicLandedUnder(e.Dirs, e.LookupRoot))
	}})
	Default.Register(Step{Name: "backfill-epic-times", Append: true, Run: func(e Env) ([]tickets.MigrationChange, error) {
		return ticketmigrate.BackfillEpicTimes(e.Root)
	}})
	Default.Register(Step{Name: "backfill-ticket-totals", Run: func(e Env) ([]tickets.MigrationChange, error) {
		home, err := e.Dirs.Home()
		if err != nil {
			return nil, err
		}
		totals := func(ids []string) (int64, float64, error) { return metrics.SessionIDsTotals(home, ids) }
		return ticketmigrate.BackfillTicketTotals(e.Root, totals, conflictSessionsUnder(e.LookupRoot))
	}})
	Default.Register(Step{Name: "backfill-friction", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return ticketmigrate.BackfillFriction(e.Root, ticketFrictionUnder(e.LookupRoot))
	}})
	Default.Register(Step{Name: "backfill-epic-totals", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return ticketmigrate.BackfillEpicTotals(e.Root, epicTrailerLinesUnder(e.Dirs, e.LookupRoot))
	}})
	Default.Register(Step{Name: "destination-unconfirmed", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return ticketmigrate.MigrateDestinationUnconfirmed(e.Root)
	}})
	Default.Register(Step{Name: "design-step", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return ticketmigrate.MigrateDesignStep(e.Root, designProbeUnder(e.LookupRoot))
	}})
	Default.Register(Step{Name: "map-done", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return ticketmigrate.MigrateMapDone(e.Root, false)
	}})
	Default.Register(Step{Name: "ticket-creation", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return backfillTicketCreation(e.Root, false)
	}})
	Default.Register(Step{Name: "grilling", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return backfillGrilling(e.Dirs, e.LookupRoot, e.DryRun)
	}})
	// model-fields: ModelRan, ModelExpected, TierOverride, LongTicketTurns and the
	// model-mismatch pause are all optional. An older ticket reads as the zero
	// value, so there is nothing to rewrite.
	Default.Register(Step{Name: "model-fields", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return nil, nil
	}})
	Default.Register(Step{Name: ledgerStepName, Append: true, Run: rebuildLedger})
	// prefix-tokens: PrefixTokens is optional and an older ticket reads as zero,
	// so there is nothing to rewrite.
	Default.Register(Step{Name: "prefix-tokens", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return nil, nil
	}})
	// launch-stamp: Launch is optional and an older ticket reads as none, so
	// there is nothing to rewrite; old launches are not reported.
	Default.Register(Step{Name: "launch-stamp", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return nil, nil
	}})
	// classifier-outage: the pause reason is a new enum value; no older ticket
	// carries it, so there is nothing to rewrite.
	Default.Register(Step{Name: "classifier-outage", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return nil, nil
	}})
	// process-storm: a new pause reason enum value; no older ticket carries it.
	Default.Register(Step{Name: "process-storm", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return nil, nil
	}})
	// ci-fix-pr-text-types: new ticket type values; no older ticket carries them.
	Default.Register(Step{Name: "ci-fix-pr-text-types", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return nil, nil
	}})
	// pr-epic-fields: pr_number and pr_url are optional and read as zero when absent.
	Default.Register(Step{Name: "pr-epic-fields", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return nil, nil
	}})
	// pr-pause-kinds: new pause kind enum values; no older ticket or epic carries them.
	Default.Register(Step{Name: "pr-pause-kinds", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return nil, nil
	}})
	// epic-archive-fields: archived_at, archived_workspace and branch_name are optional; absent is the default.
	Default.Register(Step{Name: "epic-archive-fields", Run: func(e Env) ([]tickets.MigrationChange, error) {
		return nil, nil
	}})
}

// pauseUnit plans the pause migration, runs the frontmatter migration and the
// question-section step, then applies the pause plan. A failure of the plan or
// the frontmatter migration is early: nothing was changed.
func pauseUnit(e Env) ([]tickets.MigrationChange, error) {
	pause, err := ticketmigrate.PlanPauseMigration(e.Root)
	if err != nil {
		return nil, earlyError{err}
	}
	result, err := ticketmigrate.Migrate(e.Root)
	if err != nil {
		return nil, earlyError{err}
	}
	changes := result.Changes
	moved, err := ticketmigrate.MigrateQuestionSections(e.Root, time.Now())
	changes = append(changes, moved...)
	if err != nil {
		return changes, err
	}
	paused, err := ticketmigrate.ApplyPauseMigration(pause)
	return mergeChanges(changes, paused), err
}
