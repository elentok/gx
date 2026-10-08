package recovery

import (
	"testing"

	"github.com/elentok/gx/events"
)

func testEntry() Entry {
	return Entry{
		ID: "T1", Type: events.LaunchFailed, Kind: events.AgentNameTaken,
		Predicate: func(seq []Event) bool { return len(seq) >= 2 },
		Executor:  ExecutorRule, Authority: AuthorityLow, Verbs: []string{"retry"}, Enabled: true,
	}
}

func TestMatch(t *testing.T) {
	hit := []Event{{Type: events.IterationStarted}, {Type: events.LaunchFailed, Kind: events.AgentNameTaken}}
	tests := []struct {
		name string
		cat  Catalog
		seq  []Event
		want string
	}{
		{"match", Catalog{Enabled: true, Entries: []Entry{testEntry()}}, hit, "T1"},
		{"wrong kind", Catalog{Enabled: true, Entries: []Entry{testEntry()}},
			[]Event{{}, {Type: events.LaunchFailed, Kind: events.AgentPaneBusy}}, ""},
		{"wrong type", Catalog{Enabled: true, Entries: []Entry{testEntry()}},
			[]Event{{}, {Type: events.Reclaimed, Kind: events.AgentNameTaken}}, ""},
		{"predicate rejects", Catalog{Enabled: true, Entries: []Entry{testEntry()}}, hit[1:], ""},
		{"empty", Catalog{Enabled: true, Entries: []Entry{testEntry()}}, nil, ""},
		{"kill switch", Catalog{Enabled: false, Entries: []Entry{testEntry()}}, hit, ""},
		{"disabled entry", Catalog{Enabled: true, Entries: []Entry{func() Entry { e := testEntry(); e.Enabled = false; return e }()}}, hit, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.cat.Match(tt.seq)
			if ok != (tt.want != "") || got.ID != tt.want {
				t.Fatalf("got (%q, %v), want %q", got.ID, ok, tt.want)
			}
		})
	}
}

func TestWithConfig(t *testing.T) {
	cat := Catalog{Enabled: true, Entries: []Entry{testEntry()}}
	seq := []Event{{}, {Type: events.LaunchFailed, Kind: events.AgentNameTaken}}

	if _, ok := cat.WithConfig(false, nil).Match(seq); ok {
		t.Error("kill switch off still matched")
	}
	if _, ok := cat.WithConfig(true, []string{"T1"}).Match(seq); ok {
		t.Error("disabled entry still matched")
	}
	if _, ok := cat.WithConfig(true, nil).Match(seq); !ok {
		t.Error("enabled config did not match")
	}
	if !cat.Entries[0].Enabled {
		t.Error("WithConfig mutated the receiver")
	}
}

func TestDefaultR1MatchesOnlyTheSpinningPark(t *testing.T) {
	tests := []struct {
		name string
		seq  []Event
		want bool
	}{
		{"spinning park", []Event{{Type: events.IterationStarted}, {Type: events.NeedsRepair, Kind: events.Spinning}}, true},
		{"other repair kind", []Event{{Type: events.NeedsRepair, Kind: events.RetryExhausted}}, false},
		{"spinning kind on another type", []Event{{Type: events.NeedsAnswer, Kind: events.Spinning}}, false},
		{"manual park", []Event{{Type: events.NeedsRepair, Kind: events.ManualPark}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := Default().Match(tt.seq)
			if ok != tt.want || (ok && e.ID != "R1") {
				t.Fatalf("got (%q, %v), want match=%v", e.ID, ok, tt.want)
			}
		})
	}
}

func TestDefaultR1IsAnEnabledLowRuleThatNeverNudges(t *testing.T) {
	e, _ := Default().Match([]Event{{Type: events.NeedsRepair, Kind: events.Spinning}})
	if !e.Enabled || e.Executor != ExecutorRule || e.Authority != AuthorityLow || len(e.Verbs) != 0 {
		t.Errorf("R1 = %+v, want enabled low rule with no verbs", e)
	}
}

func TestDefaultIsOnAndRecordsR13(t *testing.T) {
	if !Default().Enabled {
		t.Error("default catalog must be enabled")
	}
	if _, ok := NotCatalogued["R13"]; !ok {
		t.Error("R13 not recorded")
	}
}
