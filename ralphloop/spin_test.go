package ralphloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/events"
)

// seedParks writes n park events for ticket 01 at the given age, as a prior
// run (or process) would have left in the run log.
func seedParks(t *testing.T, scratchDir, epic string, n int, age time.Duration) {
	t.Helper()
	for range n {
		err := logEvent(scratchDir, epic, Event{
			Time: time.Now().Add(-age), Type: string(events.NeedsAnswer), Ticket: "01",
			Kind: string(events.SelfReported), Reason: "question",
		})
		if err != nil {
			t.Fatalf("logEvent: %v", err)
		}
	}
}

func runSpinScenario(t *testing.T, parks int, age time.Duration, opts RunOptions) (scratchDir string, prompts *[]string) {
	t.Helper()
	scratchDir = writeEpic(t, "epic", map[string]string{
		"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
	})
	seedParks(t, scratchDir, "epic", parks, age)
	d, prompts, _ := fakeDeps()
	opts.EpicName, opts.Skill, opts.ScratchDir, opts.RepoDir = "epic", "implement", scratchDir, "/fake/repo"
	runUntilParked(t, opts, d, noopEventSink{})
	return scratchDir, prompts
}

func spinningEvents(t *testing.T, scratchDir string) []Event {
	t.Helper()
	all, _, err := ReadEvents(scratchDir, "epic")
	if err != nil {
		t.Fatalf("ReadEvents: %v", err)
	}
	var out []Event
	for _, ev := range all {
		if ev.Kind == string(events.Spinning) {
			out = append(out, ev)
		}
	}
	return out
}

func TestRun_SpinCyclesInsideWindow_ParkSpinningAndNeverReclaim(t *testing.T) {
	t.Parallel()
	scratchDir, prompts := runSpinScenario(t, 3, time.Minute, RunOptions{})

	if len(*prompts) != 0 {
		t.Errorf("prompts = %v, want the quarantined ticket never launched", *prompts)
	}
	raw, err := os.ReadFile(filepath.Join(scratchDir, "epic", "issues", "01-a.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"status: needs-repair", "park_kind: spinning"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("ticket missing %q:\n%s", want, raw)
		}
	}
	spins := spinningEvents(t, scratchDir)
	if len(spins) != 1 || spins[0].Type != string(events.NeedsRepair) {
		t.Errorf("spinning events = %+v, want exactly one needs-repair", spins)
	}
}

func TestRun_SpinCycles_BelowThresholdOrOutsideWindow_StillClaims(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		parks int
		age   time.Duration
		opts  RunOptions
	}{
		"below default M":      {parks: 2, age: time.Minute},
		"outside default 5min": {parks: 3, age: 10 * time.Minute},
		"custom cycles":        {parks: 3, age: time.Minute, opts: RunOptions{SpinCycles: 4}},
		"custom window":        {parks: 3, age: time.Minute, opts: RunOptions{SpinWindow: 30 * time.Second}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			scratchDir := writeEpic(t, "epic", map[string]string{
				"01-a.md": "---\nid: \"01\"\nstatus: open\ntype: task\n---\n# A\n",
			})
			seedParks(t, scratchDir, "epic", tc.parks, tc.age)
			d, prompts, _ := fakeDeps()
			opts := tc.opts
			opts.EpicName, opts.Skill, opts.ScratchDir, opts.RepoDir = "epic", "implement", scratchDir, "/fake/repo"
			if err := Run(opts, d, noopEventSink{}); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if len(*prompts) != 1 {
				t.Errorf("prompts = %v, want the ticket claimed once", *prompts)
			}
			if spins := spinningEvents(t, scratchDir); len(spins) != 0 {
				t.Errorf("spinning events = %+v, want none", spins)
			}
		})
	}
}

func TestRun_SpinCycles_CustomThresholdQuarantines(t *testing.T) {
	t.Parallel()
	scratchDir, prompts := runSpinScenario(t, 2, time.Minute, RunOptions{SpinCycles: 2})
	if len(*prompts) != 0 || len(spinningEvents(t, scratchDir)) != 1 {
		t.Errorf("prompts = %v, spinning = %+v, want quarantined with SpinCycles=2", *prompts, spinningEvents(t, scratchDir))
	}
}
