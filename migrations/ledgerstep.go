package migrations

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/ralphloop/metrics"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tokenledger"
)

// ledgerStepName is a released name once shipped: never rename or reuse it.
const ledgerStepName = "token-ledger-backfill"

// Pass is what the project runs of one migration pass share: one server
// start-up run over all projects, or one `gx tickets migrate` invocation. A nil
// *Pass is a pass of one project run.
type Pass struct {
	noProjectDone atomic.Bool

	scanOnce sync.Once
	claude   []metrics.SessionDay
	codex    []codexsession.DayUsage
	scanErr  error
}

// claimNoProject reports whether this is the first run of the pass, which owns
// the transcripts that belong to no project.
func (p *Pass) claimNoProject() bool {
	if p == nil {
		return true
	}
	return p.noProjectDone.CompareAndSwap(false, true)
}

// scan reads every transcript once per pass, so a pass over many projects does
// not rescan them for each one.
func (p *Pass) scan(home string) ([]metrics.SessionDay, []codexsession.DayUsage, error) {
	read := func() (c []metrics.SessionDay, x []codexsession.DayUsage, err error) {
		if c, err = metrics.ClaudeSessionDays(home, time.Local); err != nil {
			return nil, nil, err
		}
		x, err = codexsession.SessionDaysIn(filepath.Join(home, ".codex"), time.Local)
		return c, x, err
	}
	if p == nil {
		return read()
	}
	p.scanOnce.Do(func() { p.claude, p.codex, p.scanErr = read() })
	return p.claude, p.codex, p.scanErr
}

// rebuildLedger rebuilds the project's daily tokens and dollars from the old
// transcripts (see tokenledger.ApplyRebuilt). A root that is not a store
// project, or a project with no transcripts, has nothing to rebuild.
func rebuildLedger(e Env) ([]tickets.MigrationChange, error) {
	root := e.LookupRoot
	if root == "" {
		root = e.Root
	}
	if _, err := config.ReadProjectFile(root); err != nil {
		return nil, nil
	}
	home, err := e.Dirs.Home()
	if err != nil {
		return nil, err
	}
	attr, err := Attribute(e.Dirs, root)
	if err != nil {
		return nil, err
	}
	claude, codex, err := e.Pass.scan(home)
	if err != nil {
		return nil, err
	}
	rows := ledgerRows(attr, tickets.ProjectName(root), e.Pass.claimNoProject(), claude, codex)
	if len(rows) == 0 {
		return nil, nil
	}

	ledger, err := tokenledger.Open(e.StateDir)
	if err != nil {
		return nil, err
	}
	cutoff, err := ledger.FirstLiveDay()
	if err != nil {
		return nil, err
	}
	if cutoff == "" {
		cutoff = time.Now().Format(tokenledger.DayLayout)
	}
	days, skipped, tokens, dollars := sumRebuilt(rows, cutoff)

	verb := "written"
	if e.DryRun {
		verb = "would be written"
	} else {
		written, _, err := ledger.ApplyRebuilt(rows)
		if err != nil {
			return nil, err
		}
		if written == 0 {
			return nil, nil
		}
	}
	note := fmt.Sprintf("%d days %s, %d days skipped (live rows exist), %d tokens, $%.2f", days, verb, skipped, tokens, dollars)
	return []tickets.MigrationChange{{Path: filepath.Join(e.StateDir, tokenledger.FileName), Notes: []string{note}}}, nil
}

// ledgerRows sums the per-day deltas of the project's sessions into ledger
// rows. The sessions that belong to no project are included when noProject.
func ledgerRows(attr Attributions, project string, noProject bool, claude []metrics.SessionDay, codex []codexsession.DayUsage) []tokenledger.Row {
	sums := map[tokenledger.Key]tokenledger.Row{}
	add := func(id, day, h string, tokens int64, dollars float64) {
		at := attr.Of(id)
		if at.Project != project && !(noProject && at.Project == "") {
			return
		}
		k := tokenledger.Key{Day: day, Project: at.Project, Group: at.Group, Harness: h}
		r := sums[k]
		r.Key = k
		r.Tokens += tokens
		r.Dollars += dollars
		r.Rebuilt = true
		sums[k] = r
	}
	for _, d := range claude {
		add(d.SessionID, d.Day, string(harness.Claude), int64(d.Tokens), d.Cost)
	}
	for _, d := range codex {
		add(d.SessionID, d.Day, string(harness.Codex), int64(d.Tokens), 0)
	}
	out := make([]tokenledger.Row, 0, len(sums))
	for _, r := range sums {
		out = append(out, r)
	}
	return out
}

// sumRebuilt totals the rows ApplyRebuilt would take (days before cutoff) and
// counts the days it would skip.
func sumRebuilt(rows []tokenledger.Row, cutoff string) (days, skipped int, tokens int64, dollars float64) {
	taken, skip := map[string]bool{}, map[string]bool{}
	for _, r := range rows {
		if r.Day >= cutoff {
			skip[r.Day] = true
			continue
		}
		taken[r.Day] = true
		tokens += r.Tokens
		dollars += r.Dollars
	}
	return len(taken), len(skip), tokens, dollars
}
