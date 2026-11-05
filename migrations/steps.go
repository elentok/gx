package migrations

import (
	"time"

	"github.com/elentok/gx/ralphloop"
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
		totals := func(ids []string) (int64, float64, error) { return ralphloop.SessionIDsTotals(home, ids) }
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
		return tickets.MigrateDesignStep(e.Root, designProbeUnder(e.LookupRoot))
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
