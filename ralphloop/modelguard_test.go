package ralphloop

import (
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/internal/fixture"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

func TestModelMatches(t *testing.T) {
	cases := []struct {
		name          string
		ran, expected string
		want          bool
	}{
		{"alias in full id", "claude-sonnet-5-5", "sonnet", true},
		{"full id equal", "claude-sonnet-5-5", "claude-sonnet-5-5", true},
		{"other version of the family", "claude-sonnet-4-6", "claude-sonnet-5-5", true},
		{"other family", "claude-opus-5-5", "sonnet", false},
		{"other family, full ids", "claude-opus-5-5", "claude-sonnet-5-5", false},
		{"case is ignored", "Claude-Sonnet-5-5", "SONNET", true},
		{"codex equal", "gpt-5-codex", "gpt-5-codex", true},
		{"codex expected is a prefix", "gpt-5-codex", "gpt-5", true},
		{"codex other model", "gpt-4.1", "gpt-5-codex", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ModelMatches(c.ran, c.expected); got != c.want {
				t.Errorf("ModelMatches(%q, %q) = %v, want %v", c.ran, c.expected, got, c.want)
			}
		})
	}
}

type modelGuardFixture struct {
	err        error
	reads      int
	scratchDir string
}

// runModelGuardWait runs waitForFinish through three timed-out polls on an
// implement ticket expecting expected, reading turns from read.
func runModelGuardWait(t *testing.T, ticketType, expected string, read func(since time.Time) (transcript.Turns, bool), tweak ...func(*Deps)) modelGuardFixture {
	t.Helper()
	var f modelGuardFixture
	r := boundGiveUpRunner(func(wait int) bool { return wait <= 3 })
	f.scratchDir = fixture.EpicScratchDir(t, "epic")
	d := Deps{Runner: r, Sleep: func(time.Duration) {}}
	if read != nil {
		d.ReadTurns = func(_, _ string, since time.Time) (transcript.Turns, bool, error) {
			f.reads++
			turns, ok := read(since)
			return turns, ok, nil
		}
	}
	for _, tw := range tweak {
		tw(&d)
	}
	p := boundGiveUpParams()
	p.Agent = AgentClaude
	p.Model = expected
	p.TicketData = tickets.Ticket{Identifier: "19", Type: ticketType}
	p.ScratchDir = f.scratchDir
	p.EpicName = "epic"
	f.err = waitForFinish(d, p, "sess-19")
	return f
}

func modelEvents(t *testing.T, scratchDir string) []Event {
	t.Helper()
	evs, _, err := ReadEvents(scratchDir, "epic")
	if err != nil {
		t.Fatal(err)
	}
	var out []Event
	for _, ev := range evs {
		if ev.Type == string(events.ModelMismatch) {
			out = append(out, ev)
		}
	}
	return out
}

func firstTurnOn(model string) func(time.Time) (transcript.Turns, bool) {
	return func(time.Time) (transcript.Turns, bool) { return transcript.Turns{Model: model, Count: 1}, true }
}

func TestWaitForFinish_ModelMismatch_WrongModelReturnsTypedError(t *testing.T) {
	t.Parallel()
	f := runModelGuardWait(t, "implement", "sonnet", firstTurnOn("claude-opus-5-5"))
	var mm *ModelMismatchError
	if !errors.As(f.err, &mm) {
		t.Fatalf("err = %v, want *ModelMismatchError", f.err)
	}
	if mm.Ran != "claude-opus-5-5" || mm.Expected != "sonnet" {
		t.Errorf("error = %+v, want ran claude-opus-5-5 expected sonnet", mm)
	}
	if f.reads != 1 {
		t.Errorf("reads = %d, want 1", f.reads)
	}
}

func TestWaitForFinish_ModelMismatch_MatchingModelChecksOnce(t *testing.T) {
	t.Parallel()
	f := runModelGuardWait(t, "implement", "sonnet", firstTurnOn("claude-sonnet-5-5"))
	if f.err != nil {
		t.Fatalf("err = %v, want none", f.err)
	}
	if f.reads != 1 {
		t.Errorf("reads = %d, want 1: the check runs once per launch", f.reads)
	}
}

func TestWaitForFinish_ModelMismatch_NoTurnYetKeepsReading(t *testing.T) {
	t.Parallel()
	calls := 0
	f := runModelGuardWait(t, "implement", "sonnet", func(time.Time) (transcript.Turns, bool) {
		calls++
		if calls < 3 {
			return transcript.Turns{}, false
		}
		return transcript.Turns{Model: "claude-opus-5-5", Count: 1}, true
	})
	var mm *ModelMismatchError
	if !errors.As(f.err, &mm) {
		t.Fatalf("err = %v, want *ModelMismatchError once the first turn lands", f.err)
	}
}

func TestWaitForFinish_ModelMismatch_NilReadTurnsChangesNothing(t *testing.T) {
	t.Parallel()
	if f := runModelGuardWait(t, "implement", "sonnet", nil); f.err != nil {
		t.Fatalf("err = %v, want none", f.err)
	}
}

func TestWaitForFinish_ModelMismatch_EmptyExpectedSkipsAndLogsOnce(t *testing.T) {
	t.Parallel()
	f := runModelGuardWait(t, "implement", "", firstTurnOn("claude-opus-5-5"))
	if f.err != nil {
		t.Fatalf("err = %v, want none", f.err)
	}
	if f.reads != 0 {
		t.Errorf("reads = %d, want 0", f.reads)
	}
	if got := modelEvents(t, f.scratchDir); len(got) != 1 {
		t.Errorf("logged %d model events, want 1", len(got))
	}
}

func TestWaitForFinish_ModelMismatch_OtherTicketTypesAreUntouched(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{"verify", "prompt", "research"} {
		f := runModelGuardWait(t, typ, "sonnet", firstTurnOn("claude-opus-5-5"))
		if f.err != nil || f.reads != 0 {
			t.Errorf("%s: err = %v, reads = %d, want none and 0", typ, f.err, f.reads)
		}
	}
}

func TestWaitForFinish_ModelMismatchCodex(t *testing.T) {
	t.Parallel()
	f := runModelGuardWait(t, "implement", "gpt-5-codex", firstTurnOn("gpt-4.1"))
	var mm *ModelMismatchError
	if !errors.As(f.err, &mm) {
		t.Fatalf("err = %v, want *ModelMismatchError", f.err)
	}
}

func TestWaitForFinish_ResumedLaunch_OldTurnsOnAnotherModelDoNotPause(t *testing.T) {
	t.Parallel()
	launch := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	lines := []transcript.Line{{Type: "assistant", Timestamp: launch.Add(-time.Hour), Usage: transcript.Usage{Model: "claude-opus-5-5"}}}
	f := runModelGuardWait(t, "implement", "sonnet", func(since time.Time) (transcript.Turns, bool) {
		turns := transcript.CountTurns(lines, since)
		return turns, turns.Count > 0
	}, func(d *Deps) { d.Now = func() time.Time { return launch } })
	if f.err != nil {
		t.Fatalf("err = %v, want none: the old Opus turns predate this launch", f.err)
	}
	if f.reads == 0 {
		t.Error("the guard never read the transcript")
	}
}
