package recovery

import (
	"slices"
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

func TestDefaultR2LaunchesDisabledAsAnAgentEntry(t *testing.T) {
	c := Default()
	for _, e := range c.Entries {
		if e.ID != "R2" {
			continue
		}
		if e.Enabled || e.Executor != ExecutorAgent || e.Authority != AuthorityMedium {
			t.Errorf("R2 = %+v, want disabled medium agent entry", e)
		}
		if _, ok := c.Match([]Event{{Type: events.NeedsAnswer, Kind: events.ZeroCommit, Text: "Bash({"}}); ok {
			t.Error("disabled R2 must not match")
		}
		return
	}
	t.Fatal("no R2 in the default catalog")
}

func TestR2MatchesOnlyAZeroCommitEndingInABareCallLiteral(t *testing.T) {
	c := Default()
	for i := range c.Entries {
		c.Entries[i].Enabled = true
	}
	tests := []struct {
		name string
		seq  []Event
		want bool
	}{
		{"bare call literal", []Event{{Type: events.NeedsAnswer, Kind: events.ZeroCommit, Text: "Bash({\n  command: \"ls\"\n})"}}, true},
		{"surrounding whitespace", []Event{{Type: events.NeedsAnswer, Kind: events.ZeroCommit, Text: "\n  Agent({ prompt: \"x\" })\n"}}, true},
		{"fenced literal", []Event{{Type: events.NeedsAnswer, Kind: events.ZeroCommit, Text: "```\nBash({})\n```"}}, false},
		{"prose mentioning a call", []Event{{Type: events.NeedsAnswer, Kind: events.ZeroCommit, Text: "I would run Bash({ command }) next."}}, false},
		{"plain answer", []Event{{Type: events.NeedsAnswer, Kind: events.ZeroCommit, Text: "Done."}}, false},
		{"no text", []Event{{Type: events.NeedsAnswer, Kind: events.ZeroCommit}}, false},
		{"call literal on another kind", []Event{{Type: events.NeedsAnswer, Kind: events.BlockedPane, Text: "Bash({})"}}, false},
		{"call literal on another type", []Event{{Type: events.NeedsRepair, Kind: events.ZeroCommit, Text: "Bash({})"}}, false},
		{"literal only on an earlier event", []Event{{Type: events.IterationStarted, Text: "Bash({})"}, {Type: events.NeedsAnswer, Kind: events.ZeroCommit, Text: "Done."}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := c.Match(tt.seq)
			if ok != tt.want || (ok && e.ID != "R2") {
				t.Fatalf("got (%q, %v), want match=%v", e.ID, ok, tt.want)
			}
		})
	}
}

func TestDefaultR3LaunchesDisabledAsMediumAgentEntriesWithVerifyAndLand(t *testing.T) {
	c := Default()
	var n int
	for _, e := range c.Entries {
		if e.ID != "R3" {
			continue
		}
		n++
		if e.Enabled || e.Executor != ExecutorAgent || e.Authority != AuthorityMedium || !slices.Equal(e.Verbs, []string{"verify", "land"}) {
			t.Errorf("R3 = %+v, want disabled medium agent entry with verify and land", e)
		}
	}
	if n != 2 {
		t.Fatalf("R3 entries = %d, want 2 (one per signature)", n)
	}
	if _, ok := c.Match([]Event{{Type: events.NeedsRepair, Kind: events.AmbiguousLand}}); ok {
		t.Error("disabled R3 must not match")
	}
}

func TestR3MatchesAnUnrecoverableDoneTicketOrAZeroCommitClaimingPresence(t *testing.T) {
	c := Default()
	for i := range c.Entries {
		c.Entries[i].Enabled = true
	}
	zero := func(text string) []Event { return []Event{{Type: events.NeedsAnswer, Kind: events.ZeroCommit, Text: text}} }
	tests := []struct {
		name string
		seq  []Event
		want bool
	}{
		{"done ticket with commits missing", []Event{{Type: events.NeedsRepair, Kind: events.AmbiguousLand}}, true},
		{"zero-commit claiming the work is present", zero("The feature is already implemented on the branch."), true},
		{"claim in mixed case", zero("Already merged via the sibling ticket."), true},
		{"zero-commit with no claim", zero("Done."), false},
		{"zero-commit with no text", zero(""), false},
		{"claim on another kind", []Event{{Type: events.NeedsAnswer, Kind: events.BlockedPane, Text: "already implemented"}}, false},
		{"claim on another type", []Event{{Type: events.NeedsRepair, Kind: events.ZeroCommit, Text: "already implemented"}}, false},
		{"unrelated needs-repair kind", []Event{{Type: events.NeedsRepair, Kind: events.BudgetKilled}}, false},
		{"claim only on an earlier event", []Event{{Type: events.IterationFinished, Text: "already implemented"}, {Type: events.NeedsAnswer, Kind: events.ZeroCommit, Text: "Done."}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := c.Match(tt.seq)
			if ok != tt.want || (ok && e.ID != "R3") {
				t.Fatalf("got (%q, %v), want match=%v", e.ID, ok, tt.want)
			}
		})
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
