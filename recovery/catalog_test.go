package recovery

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

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

func TestDefaultR5MatchesAStalledParkAfterAStalledLaunch(t *testing.T) {
	stalledLaunch := Event{Type: events.LaunchFailed, Kind: events.AgentPromptStalled}
	stalledPark := Event{Type: events.NeedsRepair, Kind: events.AgentPromptStalled}
	tests := []struct {
		name string
		seq  []Event
		want bool
	}{
		{"stalled park after stalled launch", []Event{{Type: events.IterationStarted}, stalledLaunch, stalledLaunch, stalledPark}, true},
		{"no prior launch-failed", []Event{{Type: events.IterationStarted}, stalledPark}, false},
		{"prior launch-failed of another kind", []Event{{Type: events.LaunchFailed, Kind: events.AgentPaneBusy}, stalledPark}, false},
		{"other park kind", []Event{stalledLaunch, {Type: events.NeedsRepair, Kind: events.AgentPaneBusy}}, false},
		{"stalled kind on another type", []Event{stalledLaunch, {Type: events.NeedsAnswer, Kind: events.AgentPromptStalled}}, false},
		{"the launch-failed itself", []Event{stalledLaunch}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := Default().Match(tt.seq)
			if ok != tt.want || (ok && e.ID != "R5") {
				t.Fatalf("got (%q, %v), want match=%v", e.ID, ok, tt.want)
			}
		})
	}
}

func TestDefaultR5IsAnEnabledLowRuleThatNudgesOrClosesThePane(t *testing.T) {
	e, _ := Default().Match([]Event{
		{Type: events.LaunchFailed, Kind: events.AgentPromptStalled},
		{Type: events.NeedsRepair, Kind: events.AgentPromptStalled},
	})
	f := Failure{Address: "p:e/01", Type: events.NeedsRepair, Kind: events.AgentPromptStalled}
	if e.ID != "R5" || !e.Enabled || !e.Runnable(f) || e.Authority != AuthorityLow || !slices.Equal(e.Verbs, []string{"nudge", "close-pane"}) {
		t.Fatalf("R5 = %+v, want an enabled runnable low rule with nudge and close-pane", e)
	}

	delivered := &stubVerbs{prompt: "/gx-implement p:e/01"}
	if err := e.Remedy(f, delivered); err != nil || !slices.Equal(delivered.calls, []string{"nudge p:e/01 /gx-implement p:e/01"}) {
		t.Errorf("delivered: err %v, calls %q; want one nudge with the full prompt", err, delivered.calls)
	}
	refused := &stubVerbs{prompt: "/gx-implement p:e/01", refuse: true}
	if err := e.Remedy(f, refused); err == nil || !slices.Equal(refused.calls, []string{"nudge p:e/01 /gx-implement p:e/01", "close-pane p:e/01"}) {
		t.Errorf("refused: err %v, calls %q; want a failed nudge then close-pane", err, refused.calls)
	}
}

func TestDefaultR7MatchesAnIterationErrorAfterATimedOutCompaction(t *testing.T) {
	timedOut := Event{Type: events.SmartZoneRecoveryFailed, Reason: `compacting p after smart-zone breach: herdr agent wait p --until idle --until done --until blocked --timeout 300000: {"code":"timeout"}`}
	reprompt := Event{Type: events.SmartZoneRecoveryFailed, Reason: "re-prompting p after smart-zone compact: herdr agent prompt --until working: timed out waiting for agent status"}
	refused := Event{Type: events.SmartZoneRecoveryFailed, Reason: "confirming /compact submitted for p: pane not found"}
	started := Event{Type: events.IterationStarted}
	park := Event{Type: events.NeedsRepair, Kind: events.IterationError, Reason: "compact recovery exhausted"}
	tests := []struct {
		name string
		seq  []Event
		want bool
	}{
		{"wait timeout", []Event{started, timedOut, park}, true},
		{"re-prompt timeout", []Event{started, reprompt, park}, true},
		{"timeout then other events", []Event{started, timedOut, {Type: events.SmartZoneWaitExpired}, park}, true},
		{"failure that is not a timeout", []Event{started, refused, park}, false},
		{"compaction that expired but confirmed", []Event{started, {Type: events.SmartZoneWaitExpired, Reason: "timed out"}, park}, false},
		{"timeout in an earlier iteration", []Event{timedOut, started, park}, false},
		{"no smart-zone event", []Event{started, park}, false},
		{"timeout before another park kind", []Event{started, timedOut, {Type: events.NeedsRepair, Kind: events.Spinning}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := Default().Match(tt.seq)
			if got := ok && e.ID == "R7"; got != tt.want {
				t.Fatalf("got (%q, %v), want R7 match=%v", e.ID, ok, tt.want)
			}
		})
	}
}

func TestDefaultR7IsAnEnabledLowRuleThatWaitsOnceThenFinishesUp(t *testing.T) {
	e, _ := Default().Match([]Event{
		{Type: events.SmartZoneRecoveryFailed, Reason: `{"code":"timeout"}`},
		{Type: events.NeedsRepair, Kind: events.IterationError},
	})
	f := Failure{Address: "p:e/01", Type: events.NeedsRepair, Kind: events.IterationError}
	if e.ID != "R7" || !e.Enabled || !e.Runnable(f) || e.Authority != AuthorityLow || !slices.Equal(e.Verbs, []string{"wait", "nudge"}) {
		t.Fatalf("R7 = %+v, want an enabled runnable low rule with wait and nudge", e)
	}

	settled := &stubVerbs{}
	if err := e.Remedy(f, settled); err != nil || !slices.Equal(settled.calls, []string{"wait p:e/01", "nudge p:e/01 " + FinishUpPrompt}) {
		t.Errorf("settled: err %v, calls %q; want one wait then the finish-up", err, settled.calls)
	}
	timedOut := &stubVerbs{waitErr: errors.New("timed out")}
	if err := e.Remedy(f, timedOut); err == nil || !slices.Equal(timedOut.calls, []string{"wait p:e/01"}) {
		t.Errorf("timed out: err %v, calls %q; want a failed wait and nothing typed", err, timedOut.calls)
	}
	gone := &stubVerbs{refuse: true}
	if err := e.Remedy(f, gone); err == nil || !slices.Equal(gone.calls, []string{"wait p:e/01"}) {
		t.Errorf("no pane: err %v, calls %q; want a refused wait and nothing typed", err, gone.calls)
	}
}

// stubVerbs records calls and refuses every nudge and wait when refuse is set.
type stubVerbs struct {
	recorder
	prompt  string
	refuse  bool
	waitErr error
	calls   []string
}

func (s *stubVerbs) Wait(address string) (Result, error) {
	s.calls = append(s.calls, "wait "+address)
	return Result{Refused: s.refuse, Reason: "iteration-not-live"}, s.waitErr
}

func (s *stubVerbs) Nudge(address, text string) (Result, error) {
	s.calls = append(s.calls, "nudge "+address+" "+text)
	return Result{Refused: s.refuse, Reason: "iteration-not-live"}, nil
}

func (s *stubVerbs) ClosePane(address string) (Result, error) {
	s.calls = append(s.calls, "close-pane "+address)
	return Result{}, nil
}

func (s *stubVerbs) ReleaseGate(address string) (Result, error) {
	s.calls = append(s.calls, "release-gate "+address)
	return Result{Refused: s.refuse, Reason: "agent-busy"}, nil
}

func (s *stubVerbs) Finish(address string) (Result, error) {
	s.calls = append(s.calls, "finish "+address)
	return Result{}, nil
}

func (s *stubVerbs) SetParent(address, parent string) (Result, error) {
	s.calls = append(s.calls, "set-parent "+address+" "+parent)
	return Result{Refused: s.refuse, Reason: "parent-changed"}, nil
}

func (s *stubVerbs) LaunchPrompt(string) (string, error) { return s.prompt, nil }

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
		{"call literal on another kind", []Event{{Type: events.NeedsAnswer, Kind: events.SelfReported, Text: "Bash({})"}}, false},
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
		if e.Enabled {
			t.Errorf("R3 = %+v, want disabled", e)
		}
		if e.Executor != ExecutorAgent {
			continue
		}
		n++
		if e.Authority != AuthorityMedium || !slices.Equal(e.Verbs, []string{"verify", "land"}) {
			t.Errorf("R3 = %+v, want medium agent entry with verify and land", e)
		}
	}
	if n != 2 {
		t.Fatalf("R3 agent entries = %d, want 2 (one per signature)", n)
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
	zero := func(text string) []Event {
		return []Event{{Type: events.NeedsAnswer, Kind: events.ZeroCommit, Text: text}}
	}
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
		{"claim on another kind", []Event{{Type: events.NeedsAnswer, Kind: events.SelfReported, Text: "already implemented"}}, false},
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

func enabledDefault() Catalog {
	c := Default()
	for i := range c.Entries {
		c.Entries[i].Enabled = true
	}
	return c
}

func TestR3ProposesCommitlessDoneWhenTheWorkLandedInAnotherTicket(t *testing.T) {
	f := Failure{Address: "p:e/01", Type: events.NeedsAnswer, Kind: events.ZeroCommit}
	e, ok := enabledDefault().Match([]Event{{Type: f.Type, Kind: f.Kind, Text: "Already implemented by the sibling ticket 04."}})
	if !ok || e.ID != "R3" {
		t.Fatalf("got (%q, %v), want R3", e.ID, ok)
	}
	if e.Runnable(f) || !e.Proposable(f) {
		t.Fatalf("R3 commitless-done = %+v, want proposed only", e)
	}
	calls, err := e.Propose(f)
	if err != nil || len(calls) != 1 || calls[0].String() != "commitless-done p:e/01" {
		t.Fatalf("proposal = %v, %v, want one commitless-done call", calls, err)
	}
}

func TestR3LostCommitsOnlyEscalate(t *testing.T) {
	f := Failure{Type: events.NeedsRepair, Kind: events.AmbiguousLand}
	for _, reason := range []string{
		"done but commits missing from epic-a and iteration branch ralph-loop/epic-a-item-01 no longer exists to recover them",
		"land interrupted by a crash (lock owner pid 1) and verify cannot tell whether it landed: unrecoverable",
	} {
		e, ok := enabledDefault().Match([]Event{{Type: f.Type, Kind: f.Kind, Reason: reason}})
		if !ok || e.ID != "R3" || e.Executor != ExecutorPerson || e.Runnable(f) || e.Proposable(f) {
			t.Errorf("reason %q matched (%+v, %v), want an escalate-only R3", reason, e, ok)
		}
	}
	if e, _ := enabledDefault().Match([]Event{{Type: f.Type, Kind: f.Kind, Reason: "verify cannot tell whether it landed: unknown"}}); e.Executor != ExecutorAgent {
		t.Errorf("an ambiguous land with its branch = %+v, want the agent entry", e)
	}
}

func TestDefaultR4LaunchesDisabledAsAMediumAgentEntryThatAnswers(t *testing.T) {
	c := Default()
	var n int
	for _, e := range c.Entries {
		if e.ID != "R4" {
			continue
		}
		n++
		if e.Enabled || e.Executor != ExecutorAgent || e.Authority != AuthorityMedium || !slices.Equal(e.Verbs, []string{"answer"}) {
			t.Errorf("R4 = %+v, want disabled medium agent entry with answer", e)
		}
	}
	if n != 1 {
		t.Fatalf("R4 entries = %d, want 1", n)
	}
	if _, ok := c.Match([]Event{{Type: events.NeedsAnswer, Kind: events.BlockedPane}}); ok {
		t.Error("disabled R4 must not match")
	}
}

func TestR4MatchesOnlyANeedsAnswerBlockedPane(t *testing.T) {
	c := Default()
	for i := range c.Entries {
		c.Entries[i].Enabled = true
	}
	tests := []struct {
		name string
		seq  []Event
		want bool
	}{
		{"blocked pane", []Event{{Type: events.NeedsAnswer, Kind: events.BlockedPane}}, true},
		{"blocked pane with any last text", []Event{{Type: events.NeedsAnswer, Kind: events.BlockedPane, Text: "Do you trust this folder?"}}, true},
		{"blocked pane after other events", []Event{{Type: events.IterationFinished}, {Type: events.NeedsAnswer, Kind: events.BlockedPane}}, true},
		{"blocked pane as needs-repair", []Event{{Type: events.NeedsRepair, Kind: events.BlockedPane}}, false},
		{"other needs-answer kind", []Event{{Type: events.NeedsAnswer, Kind: events.SelfReported}}, false},
		{"blocked pane only on an earlier event", []Event{{Type: events.NeedsAnswer, Kind: events.BlockedPane}, {Type: events.IterationFinished}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := c.Match(tt.seq)
			if ok != tt.want || (ok && e.ID != "R4") {
				t.Fatalf("got (%q, %v), want match=%v", e.ID, ok, tt.want)
			}
		})
	}
}

func TestDefaultR6LaunchesDisabledAsABusyRetryAndATakenNameAgent(t *testing.T) {
	c := Default()
	got := map[events.Kind]Entry{}
	for _, e := range c.Entries {
		if e.ID == "R6" {
			got[e.Kind] = e
		}
	}
	if len(got) != 2 {
		t.Fatalf("R6 entries = %v, want one per launch-failed kind", got)
	}
	busy, taken := got[events.AgentPaneBusy], got[events.AgentNameTaken]
	if busy.Enabled || busy.Executor != ExecutorRule || busy.Authority != AuthorityLow || busy.Remedy == nil || !slices.Equal(busy.Verbs, []string{"relaunch"}) {
		t.Errorf("R6 busy = %+v, want disabled low rule that relaunches", busy)
	}
	if taken.Enabled || taken.Executor != ExecutorAgent || taken.Authority != AuthorityMedium || !slices.Equal(taken.Verbs, []string{"close-pane", "relaunch"}) {
		t.Errorf("R6 taken = %+v, want disabled medium agent that closes a pane and relaunches", taken)
	}
	seq := []Event{{Type: events.LaunchFailed, Kind: events.AgentPaneBusy}, {Type: events.NeedsRepair, Kind: events.AgentPaneBusy}}
	if _, ok := c.Match(seq); ok {
		t.Error("disabled R6 must not match")
	}
}

func TestR6MatchesAParkedLaunchCollisionOfTheSameKind(t *testing.T) {
	c := Default()
	for i := range c.Entries {
		c.Entries[i].Enabled = true
	}
	failed := func(k events.Kind) Event { return Event{Type: events.LaunchFailed, Kind: k} }
	park := func(k events.Kind) Event { return Event{Type: events.NeedsRepair, Kind: k} }
	tests := []struct {
		name     string
		seq      []Event
		want     bool
		executor Executor
	}{
		{"pane busy", []Event{failed(events.AgentPaneBusy), park(events.AgentPaneBusy)}, true, ExecutorRule},
		{"name taken", []Event{failed(events.AgentNameTaken), park(events.AgentNameTaken)}, true, ExecutorAgent},
		{"retries before the park", []Event{failed(events.AgentNameTaken), failed(events.AgentNameTaken), {Type: events.IterationStarted}, park(events.AgentNameTaken)}, true, ExecutorAgent},
		{"park with no launch-failed", []Event{park(events.AgentPaneBusy)}, false, ""},
		{"launch-failed of another kind", []Event{failed(events.AgentPromptStalled), park(events.AgentNameTaken)}, false, ""},
		{"busy failure, taken park", []Event{failed(events.AgentPaneBusy), park(events.AgentNameTaken)}, false, ""},
		{"retry storm", []Event{failed(events.AgentPaneBusy), park(events.RetryExhausted)}, false, ""},
		{"launch-failed with no park", []Event{failed(events.AgentPaneBusy)}, false, ""},
		{"collision as needs-answer", []Event{failed(events.AgentNameTaken), {Type: events.NeedsAnswer, Kind: events.AgentNameTaken}}, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := c.Match(tt.seq)
			if ok != tt.want || (ok && (e.ID != "R6" || e.Executor != tt.executor)) {
				t.Fatalf("got (%q, %q, %v), want match=%v executor %q", e.ID, e.Executor, ok, tt.want, tt.executor)
			}
		})
	}
}

func TestDefaultR8LaunchesEnabledAsANoOpRule(t *testing.T) {
	var r8 []Entry
	for _, e := range Default().Entries {
		if e.ID == "R8" {
			r8 = append(r8, e)
		}
	}
	if len(r8) != 1 {
		t.Fatalf("R8 entries = %v, want one", r8)
	}
	e := r8[0]
	if !e.Enabled || e.Executor != ExecutorRule || e.Authority != AuthorityLow || len(e.Verbs) != 0 || e.Remedy == nil {
		t.Fatalf("R8 = %+v, want enabled low rule with no verbs", e)
	}
	if err := e.Remedy(Failure{}, nil); err != nil {
		t.Errorf("R8 remedy = %v, want a no-op", err)
	}
}

func TestR8MatchesOnlyARateLimitPauseThatResets(t *testing.T) {
	c := Default()
	for i := range c.Entries {
		c.Entries[i].Enabled = true
	}
	pause := func(reason string) Event { return Event{Type: events.PausedRateLimit, Reason: reason} }
	tests := []struct {
		name string
		seq  []Event
		want bool
	}{
		{"claude rate limit", []Event{pause("rate limit detected")}, true},
		{"claude rate limit with reset", []Event{pause("rate limit detected, resets 3pm")}, true},
		{"codex quota with reset", []Event{pause("Codex 5h quota exhausted, resets 2026-10-08T15:00:00Z")}, true},
		{"after an earlier pause", []Event{pause("rate limit detected"), {Type: events.Resumed}, pause("rate limit detected")}, true},
		{"codex quota without reset", []Event{pause("Codex weekly quota exhausted")}, false},
		{"unknown pause reason", []Event{pause("budget exhausted")}, false},
		{"smart-zone pause", []Event{{Type: events.PausedSmartZone, Reason: "rate limit detected"}}, false},
		{"resumed after the pause", []Event{pause("rate limit detected"), {Type: events.Resumed}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := c.Match(tt.seq)
			if ok != tt.want || (ok && e.ID != "R8") {
				t.Fatalf("got (%q, %v), want match=%v", e.ID, ok, tt.want)
			}
		})
	}
}

func TestDefaultR10LaunchesDisabledAsAMediumRule(t *testing.T) {
	var r10 []Entry
	for _, e := range Default().Entries {
		if e.ID == "R10" {
			r10 = append(r10, e)
		}
	}
	if len(r10) != 1 {
		t.Fatalf("R10 entries = %v, want one", r10)
	}
	e := r10[0]
	if e.Enabled || e.Executor != ExecutorRule || e.Authority != AuthorityMedium || !slices.Equal(e.Verbs, []string{"release-gate", "finish"}) || e.Remedy == nil {
		t.Fatalf("R10 = %+v, want disabled medium rule with release-gate, finish", e)
	}
	held := []Event{{Type: events.BackgroundTaskGateHeld, Kind: events.BackgroundTaskGate, Reason: "background task t1"}}
	if _, ok := Default().Match(held); ok {
		t.Error("R10 must not match while disabled")
	}
}

func TestR10RemedyReleasesThenFinishesAndNeverParks(t *testing.T) {
	e := Entry{}
	for _, c := range Default().Entries {
		if c.ID == "R10" {
			e = c
		}
	}
	f := Failure{Address: "p:e/01", Type: events.BackgroundTaskGateHeld, Kind: events.BackgroundTaskGate}
	ok := &stubVerbs{}
	if err := e.Remedy(f, ok); err != nil || !slices.Equal(ok.calls, []string{"release-gate p:e/01", "finish p:e/01"}) {
		t.Errorf("released: err %v, calls %q; want release-gate then finish", err, ok.calls)
	}
	refused := &stubVerbs{refuse: true}
	if err := e.Remedy(f, refused); err == nil || !strings.Contains(err.Error(), "agent-busy") || !slices.Equal(refused.calls, []string{"release-gate p:e/01"}) {
		t.Errorf("refused: err %v, calls %q; want the refusal and no finish", err, refused.calls)
	}
	if len(refused.recorder.calls) != 0 {
		t.Errorf("refused remedy called %v, want no park", refused.recorder.calls)
	}
}

func TestR10MatchesOnlyAGateStillHeld(t *testing.T) {
	c := Default()
	for i := range c.Entries {
		c.Entries[i].Enabled = true
	}
	held := Event{Type: events.BackgroundTaskGateHeld, Kind: events.BackgroundTaskGate, Reason: "background task t1"}
	released := Event{Type: events.BackgroundTaskGateReleased, Reason: "background task t1"}
	expired := Event{Type: events.BackgroundTaskGateExpired, Reason: "background task t1"}
	tests := []struct {
		name string
		seq  []Event
		want bool
	}{
		{"gate held", []Event{{Type: events.IterationStarted}, held}, true},
		{"held again after an earlier release", []Event{held, released, held}, true},
		{"gate released", []Event{held, released}, false},
		{"gate expired", []Event{held, expired}, false},
		{"held then finished", []Event{held, released, {Type: events.IterationFinished}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := c.Match(tt.seq)
			if ok != tt.want || (ok && e.ID != "R10") {
				t.Fatalf("got (%q, %v), want match=%v", e.ID, ok, tt.want)
			}
		})
	}
}

func TestDefaultR11LaunchesDisabledAsAMediumReclaimAgent(t *testing.T) {
	c := Default()
	i := slices.IndexFunc(c.Entries, func(e Entry) bool { return e.ID == "R11" })
	if i < 0 {
		t.Fatal("R11 not in the default catalog")
	}
	e := c.Entries[i]
	if e.Enabled || e.Executor != ExecutorAgent || e.Authority != AuthorityMedium || e.Remedy != nil || !slices.Equal(e.Verbs, []string{"reclaim"}) {
		t.Errorf("R11 = %+v, want disabled medium agent that reclaims", e)
	}
	seq := []Event{{Type: events.IterationStarted}, {Type: events.LaunchFailed, Kind: events.AgentNameTaken}, {Type: events.NeedsRepair, Kind: events.AgentNameTaken}}
	if got, ok := c.Match(seq); ok && got.ID == "R11" {
		t.Error("disabled R11 must not match")
	}
}

func TestR11MatchesANameTakenRelaunchOfAStillLiveIteration(t *testing.T) {
	c := Default()
	for i := range c.Entries {
		c.Entries[i].Enabled = true
	}
	started := Event{Type: events.IterationStarted}
	taken := Event{Type: events.LaunchFailed, Kind: events.AgentNameTaken}
	park := Event{Type: events.NeedsRepair, Kind: events.AgentNameTaken}
	tests := []struct {
		name string
		seq  []Event
		want string
	}{
		{"live iteration, then a taken name", []Event{started, taken, park}, "R11"},
		{"in-iteration retries", []Event{started, taken, taken, park}, "R11"},
		{"rate-limit pause while live", []Event{started, {Type: events.PausedRateLimit}, taken, park}, "R11"},
		{"no iteration ever started", []Event{taken, park}, "R6"},
		{"start only after the failure", []Event{taken, started, park}, "R6"},
		{"iteration finished first", []Event{started, {Type: events.IterationFinished}, taken, park}, "R6"},
		{"iteration parked first", []Event{started, {Type: events.NeedsRepair, Kind: events.IterationError}, taken, park}, "R6"},
		{"a person reset it", []Event{started, {Type: events.TicketReset}, taken, park}, "R6"},
		{"landed first", []Event{started, {Type: events.CherryPicked}, taken, park}, "R6"},
		{"busy pane, not a taken name", []Event{started, {Type: events.LaunchFailed, Kind: events.AgentPaneBusy}, {Type: events.NeedsRepair, Kind: events.AgentPaneBusy}}, "R6"},
		{"taken-name failure, other park", []Event{started, taken, {Type: events.NeedsRepair, Kind: events.RetryExhausted}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := c.Match(tt.seq)
			if (tt.want == "") == ok || e.ID != tt.want {
				t.Fatalf("got (%q, %v), want %q", e.ID, ok, tt.want)
			}
		})
	}
}

func TestDefaultR12LaunchesDisabledAsAMediumAgentThatClosesAndRelaunches(t *testing.T) {
	c := Default()
	i := slices.IndexFunc(c.Entries, func(e Entry) bool { return e.ID == "R12" })
	if i < 0 {
		t.Fatal("R12 not in the default catalog")
	}
	e := c.Entries[i]
	if e.Enabled || e.Executor != ExecutorAgent || e.Authority != AuthorityMedium || e.Remedy != nil || !slices.Equal(e.Verbs, []string{"close-pane", "relaunch"}) {
		t.Errorf("R12 = %+v, want disabled medium agent that closes the pane and relaunches", e)
	}
	t0 := time.Now()
	seq := []Event{
		{Type: events.LaunchFailed, Kind: events.AgentPromptStalled, Time: t0},
		{Type: events.Reclaimed, Time: t0.Add(time.Minute)},
		{Type: events.NeedsAnswer, Kind: events.ZeroCommit, Time: t0.Add(time.Minute + time.Second)},
	}
	if got, ok := c.Match(seq); ok && got.ID == "R12" {
		t.Error("disabled R12 must not match")
	}
}

func TestR12MatchesAQuickZeroCommitFinishOfAReclaimAfterAStall(t *testing.T) {
	c := Default()
	for i := range c.Entries {
		c.Entries[i].Enabled = true
	}
	t0 := time.Now()
	at := func(typ events.Type, kind events.Kind, d time.Duration) Event {
		return Event{Type: typ, Kind: kind, Time: t0.Add(d)}
	}
	stall := at(events.LaunchFailed, events.AgentPromptStalled, 0)
	reclaimed := at(events.Reclaimed, "", time.Minute)
	park := at(events.NeedsAnswer, events.ZeroCommit, time.Minute+time.Second)
	tests := []struct {
		name string
		seq  []Event
		want string
	}{
		{"stall, reclaim, quick zero-commit park", []Event{stall, reclaimed, park}, "R12"},
		{"stall parked, then reclaimed", []Event{stall, at(events.NeedsRepair, events.AgentPromptStalled, time.Second), reclaimed, park}, "R12"},
		{"finished logged before the park", []Event{stall, reclaimed, at(events.IterationFinished, "", time.Minute+time.Second), park}, "R12"},
		{"reclaim finished slowly", []Event{stall, reclaimed, at(events.NeedsAnswer, events.ZeroCommit, 2*time.Minute)}, ""},
		{"no earlier stall", []Event{at(events.IterationStarted, "", 0), reclaimed, park}, ""},
		{"fresh launch, not a reclaim", []Event{stall, at(events.IterationStarted, "", time.Minute), park}, ""},
		{"reclaim of an older iteration", []Event{stall, reclaimed, at(events.IterationStarted, "", time.Minute), park}, ""},
		{"a person reset after the stall", []Event{stall, at(events.TicketReset, "", time.Second), reclaimed, park}, ""},
		{"landed after the stall", []Event{stall, at(events.CherryPicked, "", time.Second), reclaimed, park}, ""},
		{"reclaim with no time", []Event{stall, {Type: events.Reclaimed}, park}, ""},
		{"stall of another kind", []Event{at(events.LaunchFailed, events.AgentPaneBusy, 0), reclaimed, park}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, ok := c.Match(tt.seq)
			if tt.want == "" {
				if ok && e.ID == "R12" {
					t.Fatal("R12 matched a near-miss")
				}
				return
			}
			if !ok || e.ID != tt.want {
				t.Fatalf("got (%q, %v), want %q", e.ID, ok, tt.want)
			}
		})
	}
}

func TestDefaultR14LaunchesDisabledAsAMediumRuleThatSetsTheParent(t *testing.T) {
	c := Default()
	i := slices.IndexFunc(c.Entries, func(e Entry) bool { return e.ID == "R14" })
	if i < 0 {
		t.Fatal("R14 not in the default catalog")
	}
	e := c.Entries[i]
	if e.Enabled || e.Executor != ExecutorRule || e.Authority != AuthorityMedium || !slices.Equal(e.Verbs, []string{"set-parent"}) {
		t.Errorf("R14 = %+v, want disabled medium rule that sets the parent", e)
	}
	seq := []Event{{Type: events.TicketGraphDefect, Kind: events.ParentDefect}}
	if got, ok := c.Match(seq); ok && got.ID == "R14" {
		t.Error("disabled R14 must not match")
	}
	c.Entries[i].Enabled = true
	if got, ok := c.Match(seq); !ok || got.ID != "R14" {
		t.Errorf("enabled R14: got (%q, %v), want R14", got.ID, ok)
	}
}

func TestR14RemedySetsTheIDDerivedParentAndReturnsARefusal(t *testing.T) {
	e := Entry{}
	for _, c := range Default().Entries {
		if c.ID == "R14" {
			e = c
		}
	}
	f := Failure{Address: "p:e/12a", Type: events.TicketGraphDefect, Kind: events.ParentDefect, Reason: "12"}
	ok := &stubVerbs{}
	if err := e.Remedy(f, ok); err != nil || !slices.Equal(ok.calls, []string{"set-parent p:e/12a 12"}) {
		t.Errorf("set: err %v, calls %q; want one set-parent to 12", err, ok.calls)
	}
	refused := &stubVerbs{refuse: true}
	if err := e.Remedy(f, refused); err == nil || !strings.Contains(err.Error(), "parent-changed") {
		t.Errorf("refused: err %v, want the refusal", err)
	}
}

func TestR14ParentDefectFlagsALetteredTicketWhoseParentItsIDDoesNotAllow(t *testing.T) {
	p := func(s string) *string { return &s }
	tests := []struct {
		name   string
		id     string
		parent *string
		want   string
	}{
		{"lettered, parent missing", "02a", nil, "02"},
		{"numbered fork, parent missing", "06b1", nil, "06b"},
		{"lettered, parent empty", "02a", p(""), "02"},
		{"parent of another number", "02a", p("03"), "02"},
		{"parent is itself", "02a", p("02a"), "02"},
		{"parent skips a level", "06b1", p("06"), "06b"},
		{"parent under another letter", "06b1", p("06a"), "06b"},
		{"parent is a later fork", "27a1", p("27a3"), "27a"},
		{"parent is a sibling letter", "02b", p("02a"), "02"},
		{"lettered, ID parent", "02a", p("02"), ""},
		{"parent padded differently", "02a", p("2"), ""},
		{"numbered fork, ID parent", "06b1", p("06b"), ""},
		{"numbered fork of an earlier sibling", "27a3", p("27a1"), ""},
		{"upper-case ID", "02A", p("02"), ""},
		{"bare number, no parent", "02", nil, ""},
		{"bare number with a parent", "02", p("01"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want, defect := ParentDefect(tt.id, tt.parent)
			if defect != (tt.want != "") || want != tt.want {
				t.Errorf("ParentDefect(%q) = (%q, %v), want %q", tt.id, want, defect, tt.want)
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

func TestR9IsRecordedNotCatalogued(t *testing.T) {
	if _, ok := NotCatalogued["R9"]; !ok {
		t.Error("R9 not recorded")
	}
	c := Default()
	for i := range c.Entries {
		if c.Entries[i].ID == "R9" {
			t.Errorf("R9 entry %+v, want none", c.Entries[i])
		}
		c.Entries[i].Enabled = true
	}
	for _, reason := range []string{"context deadline exceeded", "dial tcp: lookup api.telegram.org: no such host", "send failed with status 400"} {
		if e, ok := c.Match([]Event{{Type: events.NotificationFailed, Reason: reason}}); ok {
			t.Errorf("notification-failed %q matched %s", reason, e.ID)
		}
	}
}
