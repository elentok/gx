package stats

import (
	"testing"

	"github.com/elentok/gx/tokenledger"
)

func costRow(day, project, group, harness string, tokens int64, dollars float64) tokenledger.Row {
	return tokenledger.Row{
		Key:     tokenledger.Key{Day: day, Project: project, Group: group, Harness: harness},
		Tokens:  tokens,
		Dollars: dollars,
	}
}

func stampSeries(days ...costDay) costSeries { return costSeries(days) }

func TestStatsCostSource(t *testing.T) {
	days := []string{"d1", "d2", "d3", "d4", "d5"}
	stamped := costDay{finished: true, stamped: true, tokens: 100, spend: 1}
	unstamped := costDay{finished: true}

	tests := []struct {
		name   string
		in     costSourceInput
		source []string
		// expected Queue tokens per day; -1 means no data
		queue  []float64
		design []float64
	}{
		{
			name: "stamp only",
			in: costSourceInput{
				Days:   days,
				Design: stampSeries(stamped, costDay{}, unstamped, costDay{}, costDay{}),
				Impl:   stampSeries(costDay{}, stamped, costDay{}, unstamped, costDay{}),
			},
			source: []string{"stamp", "stamp", "stamp", "stamp", "stamp"},
			queue:  []float64{0, 100, 0, -1, 0},
			design: []float64{100, 0, -1, 0, 0},
		},
		{
			name: "rebuilt then live, empty days are zero",
			in: costSourceInput{
				Days:         days,
				FirstRebuilt: "d2",
				FirstLive:    "d4",
				Rows: []tokenledger.Row{
					costRow("d2", "p", "Queue", "claude", 10, 0.1),
					costRow("d4", "p", "Queue", "claude", 40, 0.4),
				},
				Impl: stampSeries(stamped, stamped, stamped, stamped, stamped),
			},
			// d1 stamp (before rebuilt), d2-d3 rebuilt (d3 empty = zero), d4-d5 live
			source: []string{"stamp", "rebuilt", "rebuilt", "live", "live"},
			queue:  []float64{100, 10, 0, 40, 0},
			design: []float64{0, 0, 0, 0, 0},
		},
		{
			name: "live only: days before it fall back to stamps",
			in: costSourceInput{
				Days:      days,
				FirstLive: "d3",
				Rows:      []tokenledger.Row{costRow("d3", "p", "Design", "claude", 7, 0.07)},
				Design:    stampSeries(unstamped, costDay{}, costDay{}, costDay{}, costDay{}),
			},
			source: []string{"stamp", "stamp", "live", "live", "live"},
			queue:  []float64{0, 0, 0, 0, 0},
			design: []float64{-1, 0, 7, 0, 0},
		},
		{
			name: "rebuilt with no live day runs to the end",
			in: costSourceInput{
				Days:         days,
				FirstRebuilt: "d4",
			},
			source: []string{"stamp", "stamp", "stamp", "rebuilt", "rebuilt"},
			queue:  []float64{0, 0, 0, 0, 0},
			design: []float64{0, 0, 0, 0, 0},
		},
		{
			name: "live wins when first rebuilt day is after first live day",
			in: costSourceInput{
				Days:         days,
				FirstRebuilt: "d5",
				FirstLive:    "d2",
			},
			source: []string{"stamp", "live", "live", "live", "live"},
			queue:  []float64{0, 0, 0, 0, 0},
			design: []float64{0, 0, 0, 0, 0},
		},
		{
			name: "stamp rows are ignored on stamp days",
			in: costSourceInput{
				Days:      days,
				FirstLive: "d5",
				Rows:      []tokenledger.Row{costRow("d1", "p", "Queue", "claude", 999, 9)},
				Impl:      stampSeries(stamped),
			},
			source: []string{"stamp", "stamp", "stamp", "stamp", "live"},
			queue:  []float64{100, 0, 0, 0, 0},
			design: []float64{0, 0, 0, 0, 0},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Pad series so every case has one entry per day.
			for len(tc.in.Design) < len(days) {
				tc.in.Design = append(tc.in.Design, costDay{})
			}
			for len(tc.in.Impl) < len(days) {
				tc.in.Impl = append(tc.in.Impl, costDay{})
			}
			got := pickCostSource(tc.in)
			if len(got) != len(days) {
				t.Fatalf("got %d days, want %d", len(got), len(days))
			}
			for i, d := range got {
				if d.Source != tc.source[i] {
					t.Errorf("day %d source = %q, want %q", i, d.Source, tc.source[i])
				}
				for name, want := range map[string]float64{"Queue": tc.queue[i], "Design": tc.design[i]} {
					a := d.Groups[name].Tokens
					if want < 0 {
						if !a.NoData {
							t.Errorf("day %d %s: want no-data, got %v", i, name, a)
						}
						continue
					}
					if a.NoData || a.Value != want {
						t.Errorf("day %d %s tokens = %+v, want %v", i, name, a, want)
					}
				}
			}
		})
	}
}

func TestStatsCostSourceHarnessSplitAndTotals(t *testing.T) {
	got := pickCostSource(costSourceInput{
		Days:      []string{"d1"},
		FirstLive: "d1",
		Rows: []tokenledger.Row{
			costRow("d1", "p", "Queue", "claude", 10, 1.5),
			costRow("d1", "p", "Queue", "codex", 5, 0),
			costRow("d1", "q", "Queue", "claude", 20, 2),
		},
		Design: costSeries{{}},
		Impl:   costSeries{{}},
	})
	q := got[0].Groups["Queue"]
	if q.Tokens.Value != 35 || q.Dollars.Value != 3.5 {
		t.Errorf("queue totals = %v / %v, want 35 / 3.5", q.Tokens.Value, q.Dollars.Value)
	}
	if h := q.Harness["codex"]; h.Tokens != 5 || h.Dollars != 0 {
		t.Errorf("codex = %+v, want 5 tokens $0", h)
	}
	if h := q.Harness["claude"]; h.Tokens != 30 || h.Dollars != 3.5 {
		t.Errorf("claude = %+v, want 30 tokens $3.5", h)
	}
	if q.Tokens.NoData {
		t.Error("a Codex-bearing day must not be no-data")
	}
}

func TestStatsCostSourceProjectFilter(t *testing.T) {
	rows := []tokenledger.Row{
		costRow("d1", "p", "Queue", "claude", 10, 1),
		costRow("d1", "q", "Queue", "claude", 20, 2),
		costRow("d1", "", "Queue", "claude", 40, 4),
	}
	tests := []struct {
		project string
		want    float64
	}{
		{"", 70},  // All projects includes "no project"
		{"p", 10}, // a project excludes "no project" and other projects
		{"q", 20},
		{"nope", 0}, // inside the ledger range an empty day is a zero
	}
	for _, tc := range tests {
		got := pickCostSource(costSourceInput{
			Days: []string{"d1"}, FirstLive: "d1", Rows: rows, Project: tc.project,
			Design: costSeries{{}}, Impl: costSeries{{}},
		})
		a := got[0].Groups["Queue"].Tokens
		if a.NoData || a.Value != tc.want {
			t.Errorf("project %q: tokens = %+v, want %v", tc.project, a, tc.want)
		}
	}
}
