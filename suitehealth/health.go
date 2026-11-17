// Package suitehealth turns the gx check records and the brief index into the
// numbers of the test-suite stability work: the first-attempt pass rate of
// full trunk runs, the tests that flip on one tree, and per-test durations.
package suitehealth

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/check"
)

const (
	// ExitPass is the exit code whatever the numbers are.
	ExitPass = 0
	// ExitCantRun is the exit code when the data cannot be read.
	ExitCantRun = 2

	minSamples   = 5
	defaultLimit = 20
	defaultOrig  = "agent"
)

// Options selects the data and the output size.
type Options struct {
	DataDir string
	Project string
	// Limit is the number of lines of the flip list and the duration list;
	// zero means 20.
	Limit int
	// AllRuns reads every index line, not only the ones of full trunk runs.
	AllRuns bool
}

// Health prints the three blocks and returns the exit code.
func Health(o Options, w io.Writer) int {
	if o.Limit <= 0 {
		o.Limit = defaultLimit
	}
	recs, err := check.ReadRecords(o.DataDir, o.Project)
	if err != nil {
		return cantRun(w, err)
	}
	runs := fullTrunkRuns(recs)
	data, err := readIndex(o, runs)
	if err != nil {
		return cantRun(w, err)
	}
	printHeadline(w, runs, data.origins)
	printFlips(w, data.flips, o.Limit)
	printDurations(w, data.durations, o.Limit)
	return ExitPass
}

func cantRun(w io.Writer, err error) int {
	fmt.Fprintf(w, "brief health: %v\n", err)
	return ExitCantRun
}

// run is one full trunk run.
type run struct {
	id, tree string
	failed   bool
	slow     map[string]bool
}

// fullTrunkRuns keeps the records that ran every kind of the full scope on
// trunk, oldest first.
func fullTrunkRuns(recs []check.Record) []run {
	var runs []run
	for _, r := range recs {
		if r.Scope != "full" || r.KindFilter != "" || r.Link != check.LinkTrunk || r.TreeID == "" || r.Run == "" {
			continue
		}
		runs = append(runs, run{id: r.Run, tree: r.TreeID, failed: recordFailed(r), slow: slowNames(r)})
	}
	return runs
}

func slowNames(r check.Record) map[string]bool {
	slow := map[string]bool{}
	for _, k := range r.Kinds {
		for _, n := range k.Slow {
			slow[n] = true
		}
	}
	return slow
}

// recordFailed: a kind failed for a reason that is not a guard and not only
// slow tests. A failing kind with no names (a build error, a killed run)
// counts.
func recordFailed(r check.Record) bool {
	for _, k := range r.Kinds {
		if k.Status != check.StatusFail || Excluded(k.Kind, "") {
			continue
		}
		if len(k.Failed) == 0 {
			return true
		}
		for _, n := range k.Failed {
			if !slices.Contains(k.Slow, n) && !Excluded(k.Kind, n) {
				return true
			}
		}
	}
	return false
}

func printHeadline(w io.Writer, runs []run, origins map[string]string) {
	seen := map[string]bool{}
	passed, total := 0, 0
	for _, r := range runs {
		if seen[r.tree] {
			continue
		}
		seen[r.tree] = true
		total++
		if !r.failed {
			passed++
		}
	}
	streak := 0
	for _, r := range slices.Backward(runs) {
		if r.failed {
			break
		}
		streak++
	}
	pct := 0.0
	if total > 0 {
		pct = 100 * float64(passed) / float64(total)
	}
	fmt.Fprintf(w, "%d of %d first attempts passed (%.0f%%)", passed, total, pct)
	if counts := originCounts(runs, origins); counts != "" {
		fmt.Fprintf(w, " [%s]", counts)
	}
	fmt.Fprintf(w, "\nstreak: %d\n", streak)
}

// originCounts is "agent=3 person=1", empty when no run names an origin.
func originCounts(runs []run, origins map[string]string) string {
	if len(origins) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, r := range runs {
		o := origins[r.id]
		if o == "" {
			o = defaultOrig
		}
		counts[o]++
	}
	parts := make([]string, 0, len(counts))
	for _, o := range slices.Sorted(maps.Keys(counts)) {
		parts = append(parts, fmt.Sprintf("%s=%d", o, counts[o]))
	}
	return strings.Join(parts, " ")
}

// indexData is everything one pass over the index collects.
type indexData struct {
	flips     map[string]*flip
	durations map[string][]int64
	origins   map[string]string // check run -> origin, only non-empty
}

type flip struct {
	trees  map[string]bool
	newest string
}

// readIndex streams the index line by line: it can be hundreds of MB.
func readIndex(o Options, runs []run) (*indexData, error) {
	d := &indexData{flips: map[string]*flip{}, durations: map[string][]int64{}, origins: map[string]string{}}
	f, err := os.Open(filepath.Join(o.DataDir, "brief", "index.jsonl"))
	if errors.Is(err, os.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	slowByRun := map[string]map[string]bool{}
	for _, r := range runs {
		slowByRun[r.id] = r.slow
	}
	failedTrees := map[string]map[string]bool{} // test -> trees where it failed
	rd := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := rd.ReadBytes('\n')
		if len(line) > 0 {
			readLine(o, line, slowByRun, failedTrees, d)
		}
		if errors.Is(err, io.EOF) {
			return d, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

var checkRunKey = []byte(`"check_run"`)

func readLine(o Options, line []byte, slowByRun map[string]map[string]bool, failedTrees map[string]map[string]bool, d *indexData) {
	// Cheap filter before the JSON decode: a line of a check run names it.
	if !o.AllRuns && !bytes.Contains(line, checkRunKey) {
		return
	}
	var e brief.IndexEntry
	if json.Unmarshal(line, &e) != nil {
		return
	}
	slow, isRun := slowByRun[e.CheckRun]
	if e.CheckRun != "" && e.Origin != "" {
		d.origins[e.CheckRun] = e.Origin
	}
	if !o.AllRuns && !isRun {
		return
	}
	name := func(n string) string { return brief.StableName(e.Tool, e.Dir, n) }
	failed := map[string]bool{}
	for _, n := range e.Failed {
		n = name(n)
		if !slow[n] && !Excluded("", n) {
			failed[n] = true
		}
	}
	var passing []string
	if e.Tool == "playwright" {
		passing = slices.Collect(maps.Keys(e.TestDurations))
		for i, n := range passing {
			passing[i] = name(n)
		}
	} else {
		passing = e.Tests
	}
	if e.TreeID != "" {
		for n := range failed {
			if failedTrees[n] == nil {
				failedTrees[n] = map[string]bool{}
			}
			failedTrees[n][e.TreeID] = true
		}
		for _, n := range passing {
			n = name(n)
			if failed[n] || !failedTrees[n][e.TreeID] {
				continue
			}
			fl := d.flips[n]
			if fl == nil {
				fl = &flip{trees: map[string]bool{}}
				d.flips[n] = fl
			}
			fl.trees[e.TreeID] = true
			fl.newest = max(fl.newest, e.Time)
		}
	}
	for n, ms := range e.TestDurations {
		n = name(n)
		d.durations[n] = append(d.durations[n], ms)
	}
}

func printFlips(w io.Writer, flips map[string]*flip, limit int) {
	fmt.Fprintln(w, "\nfailed then passed on the same tree:")
	names := slices.Collect(maps.Keys(flips))
	sort.Slice(names, func(i, j int) bool {
		a, b := flips[names[i]], flips[names[j]]
		if c := cmp.Compare(len(b.trees), len(a.trees)); c != 0 {
			return c < 0
		}
		return names[i] < names[j]
	})
	if len(names) == 0 {
		fmt.Fprintln(w, "  none")
	}
	for _, n := range names[:min(limit, len(names))] {
		fmt.Fprintf(w, "  %d trees  %s  %s\n", len(flips[n].trees), flips[n].newest, n)
	}
}

// Percentile is the nearest-rank percentile (p in 1..100) of a sorted list.
func Percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := (p*len(sorted) + 99) / 100
	return sorted[max(rank, 1)-1]
}

func printDurations(w io.Writer, durations map[string][]int64, limit int) {
	type row struct {
		name     string
		p50, p95 int64
		n        int
	}
	var rows []row
	for n, ms := range durations {
		if len(ms) < minSamples {
			continue
		}
		slices.Sort(ms)
		rows = append(rows, row{n, Percentile(ms, 50), Percentile(ms, 95), len(ms)})
	}
	slices.SortFunc(rows, func(a, b row) int {
		return cmp.Or(cmp.Compare(b.p95, a.p95), cmp.Compare(a.name, b.name))
	})
	fmt.Fprintf(w, "\nslowest tests (at least %d samples):\n", minSamples)
	if len(rows) == 0 {
		fmt.Fprintln(w, "  none")
	}
	for _, r := range rows[:min(limit, len(rows))] {
		fmt.Fprintf(w, "  p50 %dms  p95 %dms  (%d samples)  %s\n", r.p50, r.p95, r.n, r.name)
	}
}
