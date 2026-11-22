package evolve

import (
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/check"
	"github.com/elentok/gx/ralphloop/worktree"
	"github.com/elentok/gx/tickets/schema"
)

// A kind is slower when it is at least this far above the baseline, both
// relatively and absolutely.
const (
	slowerPercent   = 20
	slowerMinMS     = 30_000
	baselineRunsMax = 5
)

// SlowerRow is a check kind whose latest full verify run got clearly slower.
type SlowerRow struct {
	Kind        string `json:"kind"`
	Run         string `json:"run"`
	Branch      string `json:"branch"`
	DurationMS  int64  `json:"duration_ms"`
	BaselineMS  int64  `json:"baseline_ms"`
	BaselineRun int    `json:"baseline_runs"`
}

// FlakeRow is a test that failed and then passed on the same tree.
type FlakeRow struct {
	Test       string `json:"test"`
	FailingRun string `json:"failing_run"`
	// PassingRun is the passing `gx check` run id, or the passing brief
	// run, named by its log's base name without the extension.
	PassingRun string `json:"passing_run"`
	TreeID     string `json:"tree_id"`
}

// epicChecks holds what `evolve inputs` derives from the check record and the
// brief index.
type epicChecks struct {
	slower []SlowerRow
	// baselineRuns is how many earlier full runs the baseline had; under
	// baselineRunsMax no slower row is reported.
	baselineRuns int
	flakes       []FlakeRow
	// flakesNoTree counts failing records flake detection skipped for lacking a tree id.
	flakesNoTree int
}

// buildEpicChecks reads the check record and brief index through their
// packages. Nothing recorded means no rows.
func buildEpicChecks(env evolveEnv, t evolveTarget) (epicChecks, error) {
	if env.dataDir == "" {
		return epicChecks{}, nil
	}
	recs, err := check.ReadRecords(env.dataDir, t.project)
	if err != nil {
		return epicChecks{}, err
	}
	index, err := brief.ReadIndex(env.dataDir)
	if err != nil {
		return epicChecks{}, err
	}

	branches := map[string]bool{t.name: true}
	verifyBranches := map[string]bool{}
	var firstClaim time.Time
	for _, tk := range t.epic.Tickets {
		b := worktree.IterationBranch(t.name, tk.Identifier)
		branches[b] = true
		if tk.Type == string(schema.TypeVerify) {
			verifyBranches[b] = true
		}
		if !tk.ClaimedAt.IsZero() && (firstClaim.IsZero() || tk.ClaimedAt.Before(firstClaim)) {
			firstClaim = tk.ClaimedAt
		}
	}

	var out epicChecks
	out.slower, out.baselineRuns = slowerRows(recs, verifyBranches, firstClaim)
	out.flakes, out.flakesNoTree = flakeRows(recs, index, branches)
	return out, nil
}

func isFullRun(r check.Record) bool { return r.Scope == "full" && r.KindFilter == "" }

func recordTime(r check.Record) time.Time {
	ts, _ := time.Parse(time.RFC3339, r.Time)
	return ts
}

func slowerRows(recs []check.Record, verifyBranches map[string]bool, firstClaim time.Time) ([]SlowerRow, int) {
	var latest *check.Record
	var baseline []check.Record
	for i, r := range recs {
		if !isFullRun(r) {
			continue
		}
		if verifyBranches[r.Branch] {
			latest = &recs[i]
		}
		if !firstClaim.IsZero() && recordTime(r).Before(firstClaim) {
			baseline = append(baseline, r)
		}
	}
	if len(baseline) > baselineRunsMax {
		baseline = baseline[len(baseline)-baselineRunsMax:]
	}
	if latest == nil || len(baseline) < baselineRunsMax {
		return nil, len(baseline)
	}

	var rows []SlowerRow
	for _, k := range latest.Kinds {
		var ds []int
		for _, r := range baseline {
			for _, bk := range r.Kinds {
				if bk.Kind == k.Kind {
					ds = append(ds, int(bk.DurationMS))
				}
			}
		}
		if len(ds) == 0 {
			continue
		}
		base := int64(medianInt(ds))
		extra := k.DurationMS - base
		if extra >= slowerMinMS && extra*100 >= base*slowerPercent {
			rows = append(rows, SlowerRow{Kind: k.Kind, Run: latest.Run, Branch: latest.Branch,
				DurationMS: k.DurationMS, BaselineMS: base, BaselineRun: len(baseline)})
		}
	}
	return rows, len(baseline)
}

// flakeRows also counts the records with failures it had to skip for lacking a
// tree id, old records included.
func flakeRows(recs []check.Record, index []brief.IndexEntry, branches map[string]bool) (rows []FlakeRow, noTree int) {
	seen := map[string]bool{}
	for i, rec := range recs {
		if !branches[rec.Branch] {
			continue
		}
		if rec.TreeID == "" {
			if slices.ContainsFunc(rec.Kinds, func(k check.KindRecord) bool { return len(k.Failed) > 0 }) {
				noTree++
			}
			continue
		}
		for _, k := range rec.Kinds {
			for _, test := range k.Failed {
				key := rec.Run + "\x00" + test
				if seen[key] {
					continue
				}
				if pass := laterPass(recs[i+1:], index, rec, k.Kind, test); pass != "" {
					seen[key] = true
					rows = append(rows, FlakeRow{Test: test, FailingRun: rec.Run, PassingRun: pass, TreeID: rec.TreeID})
				}
			}
		}
	}
	return rows, noTree
}

// laterPass names the first later passing run of test on rec's tree: a `gx
// check` run whose same kind passed, else a brief index line that ran test
// without failing it.
func laterPass(later []check.Record, index []brief.IndexEntry, rec check.Record, kind, test string) string {
	for _, r := range later {
		if r.TreeID != rec.TreeID {
			continue
		}
		for _, k := range r.Kinds {
			if k.Kind == kind && k.Status == check.StatusPass && !slices.Contains(k.Failed, test) {
				return r.Run
			}
		}
	}
	failedAt := recordTime(rec)
	for _, e := range index {
		if e.TreeID != rec.TreeID || e.CheckRun == rec.Run {
			continue
		}
		if ts, err := time.Parse(time.RFC3339, e.Time); err != nil || !ts.After(failedAt) {
			continue
		}
		if slices.Contains(e.Tests, test) && !slices.Contains(e.Failed, test) {
			return strings.TrimSuffix(filepath.Base(e.Log), ".log")
		}
	}
	return ""
}

// runIncidentMinTests: a failing run with more failed tests than this, all
// passing in one later run, prints as one run-level incident, not per-test flakes.
// The JSON keeps one FlakeRow per test.
const runIncidentMinTests = 5

// groupFlakes groups rows sharing a failing and passing run, in first-seen order.
func groupFlakes(rows []FlakeRow) [][]FlakeRow {
	var groups [][]FlakeRow
	idx := map[string]int{}
	for _, f := range rows {
		key := f.FailingRun + "\x00" + f.PassingRun
		i, ok := idx[key]
		if !ok {
			i = len(groups)
			idx[key] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], f)
	}
	return groups
}

// flakePackages lists the distinct packages of a group's tests, sorted. A test
// name is `pkg.TestName[/sub]`.
func flakePackages(g []FlakeRow) []string {
	set := map[string]bool{}
	for _, f := range g {
		// Package paths contain slashes, so cut at the first ".Test", not at "/".
		name, _, _ := strings.Cut(f.Test, ".Test")
		set[name] = true
	}
	return slices.Sorted(maps.Keys(set))
}

func printEpicChecks(w io.Writer, table InputsTable) error {
	for _, s := range table.Slower {
		if _, err := fmt.Fprintf(w, "\nslower: %s %ds vs baseline %ds (median of %d full runs, run %s)",
			s.Kind, s.DurationMS/1000, s.BaselineMS/1000, s.BaselineRun, s.Run); err != nil {
			return err
		}
	}
	for _, g := range groupFlakes(table.Flakes) {
		if len(g) > runIncidentMinTests {
			if _, err := fmt.Fprintf(w, "\nrun-level incident: %d tests failed in %s, passed in %s (tree %s), packages %s, first %s",
				len(g), g[0].FailingRun, g[0].PassingRun, g[0].TreeID, strings.Join(flakePackages(g), ", "), g[0].Test); err != nil {
				return err
			}
			continue
		}
		for _, f := range g {
			if _, err := fmt.Fprintf(w, "\nflake: %s failed in %s, passed in %s (tree %s)", f.Test, f.FailingRun, f.PassingRun, f.TreeID); err != nil {
				return err
			}
		}
	}
	if table.FlakesNoTree > 0 {
		if _, err := fmt.Fprintf(w, "\nskipped %d records with no tree", table.FlakesNoTree); err != nil {
			return err
		}
	}
	if len(table.Slower)+len(table.Flakes)+table.FlakesNoTree > 0 {
		_, err := fmt.Fprintln(w)
		return err
	}
	return nil
}
