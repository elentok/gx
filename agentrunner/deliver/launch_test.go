package deliver_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/agentrunner/codexui"
	"github.com/elentok/gx/agentrunner/deliver"
)

// launchCaps are the default shape with every wait one tick, so the window is a
// count of polls.
func launchCaps(marker string) *deliver.LaunchCaps {
	return &deliver.LaunchCaps{Marker: marker, Window: 5 * time.Millisecond, RetypeEvery: time.Millisecond, Poll: time.Millisecond}
}

// clocked is a slashFake on a fake clock: a wait costs its duration and a type
// costs typeCost, so the window is counted on the clock, not in polls.
type clocked struct {
	*slashFake
	at       time.Time
	typeCost time.Duration
}

func newClocked(f *slashFake, typeCost time.Duration) *clocked {
	return &clocked{slashFake: f, at: time.Unix(1000, 0), typeCost: typeCost}
}

func (c *clocked) now() time.Time { return c.at }

func (c *clocked) WaitStart(d time.Duration) (bool, error) {
	c.at = c.at.Add(d)
	return c.slashFake.WaitStart(d)
}

func (c *clocked) Type(text string) error {
	c.at = c.at.Add(c.typeCost)
	return c.slashFake.Type(text)
}

func launchPolicy(c *clocked, text, marker string, slash bool, report *[]claudeui.NudgeDecision) deliver.Policy {
	p := deliver.PolicyFor(text)
	if slash {
		p.Slash.Settle = 0
	}
	p.Launch = launchCaps(marker)
	p.Now = c.now
	p.Report = func(d claudeui.NudgeDecision) { *report = append(*report, d) }
	return p
}

func typedCount(f *slashFake) int {
	return strings.Count(" "+strings.Join(f.calls, " "), " type") + strings.Count(" "+strings.Join(f.calls, " "), " submit:")
}

func TestDeliver_LaunchRetypesWhileTheMarkerShowsThenLands(t *testing.T) {
	for _, tt := range []struct {
		name, marker string
		slash        bool
	}{
		{"claude slash", claudeui.UnknownCommandMarker, true},
		{"codex slash", codexui.UnknownCommandMarker, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			text := "do it"
			if tt.slash {
				text = "/gx-skill"
			}
			bad, good := ok([]string{tt.marker + " " + text}), ok([]string{""})
			// One read after each send: the marker shows after the first two.
			f := &slashFake{reads: []read{bad, bad, good}}
			c := newClocked(f, 0)
			var report []claudeui.NudgeDecision
			if err := deliver.Deliver(c, text, launchPolicy(c, text, tt.marker, tt.slash, &report)); err != nil {
				t.Fatalf("Deliver() error = %v, want landed", err)
			}
			if len(report) != 2 || report[0] != claudeui.NudgePrompt || report[1] != claudeui.NudgePrompt {
				t.Fatalf("reports = %v, want two prompt-nudged", report)
			}
			if got := typedCount(f); got != 3 {
				t.Fatalf("typed %d times, want 3 (calls %v)", got, f.calls)
			}
		})
	}
}

func TestDeliver_LaunchThatNeverLandsFailsWithUnknownSkill(t *testing.T) {
	bad := ok([]string{claudeui.UnknownCommandMarker + " /gx-skill"})
	f := &slashFake{reads: []read{bad}}
	c := newClocked(f, 0)
	var report []claudeui.NudgeDecision
	err := deliver.Deliver(c, "/gx-skill", launchPolicy(c, "/gx-skill", claudeui.UnknownCommandMarker, true, &report))
	if !errors.Is(err, agentrunner.ErrNotDelivered) || !errors.Is(err, agentrunner.ErrUnknownSkill) {
		t.Fatalf("Deliver() error = %v, want ErrNotDelivered and ErrUnknownSkill", err)
	}
	if len(report) == 0 || typedCount(f) != len(report)+1 {
		t.Fatalf("reports %v, typed %d: want one report per retype", report, typedCount(f))
	}
}

func TestDeliver_LaunchWindowEndsAtTheDeadlineWhenSendTakesTime(t *testing.T) {
	bad := ok([]string{claudeui.UnknownCommandMarker + " /gx-skill"})
	f := &slashFake{reads: []read{bad}}
	// A retype costs 3ms; the window is 5ms from the first sighting.
	c := newClocked(f, 3*time.Millisecond)
	var report []claudeui.NudgeDecision
	start := c.at
	err := deliver.Deliver(c, "/gx-skill", launchPolicy(c, "/gx-skill", claudeui.UnknownCommandMarker, true, &report))
	if !errors.Is(err, agentrunner.ErrUnknownSkill) {
		t.Fatalf("Deliver() error = %v, want ErrUnknownSkill", err)
	}
	// First send (3ms) + window (5ms) + at most one retype (3ms). Counting only
	// polls would run to about 23ms.
	if got := c.at.Sub(start); got > 11*time.Millisecond {
		t.Fatalf("ended after %s, want the window deadline, not a poll count", got)
	}
}

func TestDeliver_LaunchWithoutTheMarkerRetypesNothing(t *testing.T) {
	f := &slashFake{reads: []read{ok([]string{"all fine"})}}
	c := newClocked(f, 0)
	var report []claudeui.NudgeDecision
	if err := deliver.Deliver(c, "/gx-skill", launchPolicy(c, "/gx-skill", claudeui.UnknownCommandMarker, true, &report)); err != nil {
		t.Fatal(err)
	}
	if len(report) != 0 || typedCount(f) != 1 {
		t.Fatalf("reports %v, typed %d, want none and once", report, typedCount(f))
	}
}

func TestDeliver_SlashPolicyFailsAtOnceOnUnknownCommand(t *testing.T) {
	f := &slashFake{reads: []read{ok(box("/xyz")), ok(append(box("/xyz"), "Unknown command: /xyz"))}}
	err := f.deliver("/xyz")
	if !errors.Is(err, agentrunner.ErrNotDelivered) || !errors.Is(err, agentrunner.ErrUnknownSkill) {
		t.Fatalf("Deliver() error = %v, want ErrNotDelivered and ErrUnknownSkill", err)
	}
	if typedCount(f) != 1 {
		t.Fatalf("typed %d times, want once", typedCount(f))
	}
}
