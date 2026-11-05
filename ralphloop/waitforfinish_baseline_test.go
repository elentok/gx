package ralphloop

import (
	"errors"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/deps"
)

// stickyBaselineDeps is the premature-idle pane the sticky-baseline tests
// share: with r a breachRunner that never times out, "/compact" and every
// subsequent poll report idle immediately, so the only thing that can ever end
// the recovery is the transcript.
func stickyBaselineDeps(
	r *blipRunner, sleeps *int,
	readCompactions func(string, string) (int, bool, error),
	readCompactionsAfter func(string, string, time.Time) (int, bool, error),
) deps.Deps {
	return deps.Deps{
		Runner:               r,
		ReadCompactions:      readCompactions,
		ReadCompactionsAfter: readCompactionsAfter,
		Sleep:                func(time.Duration) { *sleeps++ },
	}
}

// noBoundarySinceSubmission is the after-submission read for a recovery whose
// compaction never merges.
func noBoundarySinceSubmission(string, string, time.Time) (int, bool, error) { return 0, true, nil }

// TestRecoverSmartZoneBreach_UnavailableBaselineConfirmsOnABoundaryAfterSubmission
// verifies that a transcript read failing at "/compact" submission time doesn't
// disable the gate for the whole recovery: with no baseline count to compare
// against, the gate switches to "was a boundary written after submission" and
// still refuses the pane's premature idle report until one is.
func TestRecoverSmartZoneBreach_UnavailableBaselineConfirmsOnABoundaryAfterSubmission(t *testing.T) {
	t.Parallel()
	var sleeps int
	merged := 0
	r := breachRunner(func(wait int) bool {
		if wait == 3 {
			merged = 1
		}
		return false
	})
	d := stickyBaselineDeps(r, &sleeps,
		func(string, string) (int, bool, error) {
			return 0, false, errors.New("transcript read failed")
		},
		func(string, string, time.Time) (int, bool, error) {
			return merged, true, nil
		},
	)

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(t.TempDir()), "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true: the boundary does merge after submission")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact, finish-up] once a boundary merges", prompts)
	}
	if sleeps == 0 {
		t.Error("gated sleeps = 0, want the gate held closed until a boundary merged after submission")
	}
}

// TestRecoverSmartZoneBreach_FastCompactionUnderAnUnavailableBaselineIsConfirmed
// covers the race the after-submission predicate exists for: the pre-submission
// read blips, the compaction then completes inside the first tick, and every
// count read afterwards already includes the new boundary. Comparing counts
// could only ever report "not advanced" here, turning a successful compaction
// into ten minutes of waiting and a gated give-up.
func TestRecoverSmartZoneBreach_FastCompactionUnderAnUnavailableBaselineIsConfirmed(t *testing.T) {
	t.Parallel()
	var sleeps int
	r := breachRunner(nil)
	d := stickyBaselineDeps(r, &sleeps,
		func(string, string) (int, bool, error) {
			if len(r.Prompts("iter-19")) == 0 {
				return 0, false, errors.New("transcript read failed")
			}
			return 6, true, nil // already includes the boundary this recovery caused
		},
		func(string, string, time.Time) (int, bool, error) { return 1, true, nil },
	)

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(t.TempDir()), "sess-19", "smart-zone breach", 100)
	if err != nil {
		t.Fatalf("recoverSmartZoneBreach: %v", err)
	}
	if !recovered {
		t.Fatal("recoverSmartZoneBreach returned recovered=false, want true: the compaction genuinely completed")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 2 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact, finish-up]", prompts)
	}
	if r.waits != 1 || sleeps != 0 {
		t.Errorf("Runner.Wait calls = %d, gated sleeps = %d; want 1 and 0: an already-merged boundary confirms on the first poll tick", r.waits, sleeps)
	}
}

// TestRecoverSmartZoneBreach_UnavailableBaselineNeverRebasesOnALaterCount
// pins the invariant that keeps the unavailable case out of a deadlock: a count
// that only becomes readable mid-recovery is never adopted as the baseline. Its
// value is already past the true submission-time one, so "count is greater than
// baseline" would be unsatisfiable forever and a successful compaction would be
// reported as a give-up. With no boundary merging after submission here, the
// recovery must give up on the extended bound — never on that stale comparison.
func TestRecoverSmartZoneBreach_UnavailableBaselineNeverRebasesOnALaterCount(t *testing.T) {
	t.Parallel()
	var sleeps, reads int
	r := breachRunner(nil)
	d := stickyBaselineDeps(r, &sleeps,
		func(string, string) (int, bool, error) {
			reads++
			if reads == 1 { // the submission-time read
				return 0, false, errors.New("transcript read failed")
			}
			return 5, true, nil
		},
		noBoundarySinceSubmission,
	)

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(t.TempDir()), "sess-19", "smart-zone breach", 100)
	if !errors.Is(err, errCompactNeverConfirmed) {
		t.Fatalf("recoverSmartZoneBreach error = %v, want one wrapping errCompactNeverConfirmed", err)
	}
	if recovered {
		t.Error("recoverSmartZoneBreach returned recovered=true, want false: a late baseline is stale and can only deadlock the gate")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 1 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact] only: no finish-up prompt on a never-confirmed compaction", prompts)
	}
}

// TestRecoverSmartZoneBreach_UnavailableReadsInLoopHoldGateClosed verifies
// that transcript reads failing intermittently mid-recovery are treated as
// "not yet", not as "no baseline, trust the pane" — that second reading is the
// same silent-disable failure in a different disguise. A persistent read
// problem must surface as an ordinary gated give-up, paced by the extended
// bound.
func TestRecoverSmartZoneBreach_UnavailableReadsInLoopHoldGateClosed(t *testing.T) {
	t.Parallel()
	var sleeps, reads int
	r := breachRunner(nil)
	d := stickyBaselineDeps(r, &sleeps,
		func(string, string) (int, bool, error) {
			reads++
			if reads == 1 {
				return 0, true, nil // a readable baseline, then the transcript goes flaky
			}
			if reads%2 == 0 {
				return 0, false, errors.New("transcript read failed")
			}
			return 0, false, nil // exists-but-unreadable and not-yet-existing both hold
		},
		noBoundarySinceSubmission,
	)

	recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(t.TempDir()), "sess-19", "smart-zone breach", 100)
	if !errors.Is(err, errCompactNeverConfirmed) {
		t.Fatalf("recoverSmartZoneBreach error = %v, want one wrapping errCompactNeverConfirmed", err)
	}
	if recovered {
		t.Error("recoverSmartZoneBreach returned recovered=true, want false: an unreadable transcript confirms nothing")
	}
	if prompts := r.Prompts("iter-19"); len(prompts) != 1 || prompts[0] != "/compact" {
		t.Errorf("prompts = %v, want [/compact] only", prompts)
	}
	wantSleeps := smartZoneCompactExtendedTimeoutMs / smartZonePollMs
	if sleeps != wantSleeps {
		t.Errorf("gated sleeps = %d, want %d: failed reads must consume the extended bound, not short-circuit it", sleeps, wantSleeps)
	}
}

// TestRecoverSmartZoneBreach_MissingTranscriptVersusUnsupportedAgent
// discriminates the two states that arrive as the same (0, false, nil) read.
// A Claude transcript that doesn't exist yet is temporarily unavailable and
// holds the gate closed; a Codex session has no boundary signal at all and
// still fails open, never routed through the closed-gate policy.
func TestRecoverSmartZoneBreach_MissingTranscriptVersusUnsupportedAgent(t *testing.T) {
	t.Parallel()
	missing := func(string, string) (int, bool, error) { return 0, false, nil }

	t.Run("Claude transcript not written yet holds the gate closed", func(t *testing.T) {
		t.Parallel()
		var sleeps int
		r := breachRunner(nil)
		d := stickyBaselineDeps(r, &sleeps, missing, noBoundarySinceSubmission)
		recovered, err := recoverSmartZoneBreach(d, gatedBreachParams(t.TempDir()), "sess-19", "smart-zone breach", 100)
		if !errors.Is(err, errCompactNeverConfirmed) {
			t.Fatalf("recoverSmartZoneBreach error = %v, want one wrapping errCompactNeverConfirmed", err)
		}
		if prompts := r.Prompts("iter-19"); recovered || len(prompts) != 1 {
			t.Errorf("recovered = %v, prompts = %v; want no finish-up prompt for an unavailable transcript", recovered, prompts)
		}
	})

	t.Run("an agent with no boundary read wired fails open", func(t *testing.T) {
		t.Parallel()
		r := breachRunner(nil)
		var sleeps int
		d := deps.Deps{Runner: r, Sleep: func(time.Duration) { sleeps++ }}
		p := gatedBreachParams(t.TempDir())
		p.Agent = agent.AgentCodex
		recovered, err := recoverSmartZoneBreach(d, p, "sess-19", "smart-zone breach", 100)
		if err != nil {
			t.Fatalf("recoverSmartZoneBreach: %v", err)
		}
		if prompts := r.Prompts("iter-19"); !recovered || len(prompts) != 2 {
			t.Errorf("recovered = %v, prompts = %v; want the pre-gate behavior for an agent with no boundary signal", recovered, prompts)
		}
		if r.waits != 1 || sleeps != 0 {
			t.Errorf("Runner.Wait calls = %d, gated sleeps = %d; want 1 and 0: an unsupported agent trusts the first idle tick", r.waits, sleeps)
		}
	})

	t.Run("Codex rollout not written yet holds the gate closed", func(t *testing.T) {
		t.Parallel()
		var sleeps int
		r := breachRunner(nil)
		d := deps.Deps{
			Runner:                    r,
			ReadCodexCompactions:      missing,
			ReadCodexCompactionsAfter: func(string, string, time.Time) (int, bool, error) { return 0, false, nil },
			Sleep:                     func(time.Duration) { sleeps++ },
		}
		p := gatedBreachParams(t.TempDir())
		p.Agent = agent.AgentCodex
		recovered, err := recoverSmartZoneBreach(d, p, "sess-19", "smart-zone breach", 100)
		if !errors.Is(err, errCompactNeverConfirmed) {
			t.Fatalf("recoverSmartZoneBreach error = %v, want one wrapping errCompactNeverConfirmed", err)
		}
		if recovered {
			t.Error("recovered = true, want false: an unreadable rollout confirms nothing")
		}
		if want := smartZoneCompactExtendedTimeoutMs / smartZonePollMs; sleeps != want {
			t.Errorf("gated sleeps = %d, want %d: same extended bound as Claude", sleeps, want)
		}
	})

	t.Run("Codex gate holds until the rollout shows a compacted item", func(t *testing.T) {
		t.Parallel()
		var sleeps, reads int
		r := breachRunner(nil)
		d := deps.Deps{
			Runner: r,
			ReadCodexCompactions: func(string, string) (int, bool, error) {
				reads++
				if reads == 1 {
					return 2, true, nil // baseline
				}
				if sleeps >= 3 {
					return 3, true, nil
				}
				return 2, true, nil
			},
			Sleep: func(time.Duration) { sleeps++ },
		}
		p := gatedBreachParams(t.TempDir())
		p.Agent = agent.AgentCodex
		recovered, err := recoverSmartZoneBreach(d, p, "sess-19", "smart-zone breach", 100)
		if err != nil || !recovered {
			t.Fatalf("recovered = %v, err = %v; want true, nil", recovered, err)
		}
		if sleeps < 3 {
			t.Errorf("gated sleeps = %d, want >= 3: an idle pane must not end the wait before the boundary", sleeps)
		}
	})
}
