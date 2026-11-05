package recovery_test

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/server/wire"
)

const investigateAddr = servertest.InvestigateAddr

var retryAs = servertest.RetryAs

func TestInvestigateRetry_LogsRecoveryAppliedNotHumanRelaunch(t *testing.T) {
	t.Parallel()
	for _, kind := range []events.Kind{events.IterationError, events.ZeroCommit} {
		t.Run(string(kind), func(t *testing.T) {
			h := startInvestigateEnv(t, kind)
			if res := retryAs(t, h, investigateAddr); res.Refused {
				t.Fatalf("retry refused: %+v", res)
			}
			log := runLog(t, h)
			got := ofType(log, events.RecoveryApplied)
			if len(got) != 1 || got[0].Ticket != "01" || got[0].Kind != string(kind) || got[0].Outcome != "ok" || got[0].Text != investigateAddr {
				t.Errorf("recovery-applied = %+v, want one ok for 01 %s naming %s", got, kind, investigateAddr)
			}
			if hr := ofType(log, events.HumanRelaunch); len(hr) != 0 {
				t.Errorf("human-relaunch = %+v, want none", hr)
			}
		})
	}
}

func TestInvestigateRetry_BlockedPaneIsRefused(t *testing.T) {
	t.Parallel()
	h := startInvestigateEnv(t, events.BlockedPane)
	if res := retryAs(t, h, investigateAddr); !res.Refused || res.Reason != wire.ReasonInvestigateStop {
		t.Errorf("retry = %+v, want %s refusal", res, wire.ReasonInvestigateStop)
	}
}

func TestInvestigateRetry_NotAnInvestigateTicketIsRefused(t *testing.T) {
	t.Parallel()
	h := startInvestigateEnv(t, events.IterationError)
	if res := retryAs(t, h, "proj:epic-a/01"); !res.Refused || res.Reason != wire.ReasonInvestigateStop {
		t.Errorf("retry = %+v, want %s refusal", res, wire.ReasonInvestigateStop)
	}
}

func TestInvestigateRetry_PastTheGuardRailIsRefused(t *testing.T) {
	t.Parallel()
	h := startInvestigateEnv(t, events.IterationError)
	prior := runlog.Event{Type: string(events.RecoveryApplied), Ticket: "01", Kind: string(events.IterationError), Reason: "relaunch", Outcome: "ok"}
	if err := runlog.AppendEvent(filepath.Join(h.TicketStore, "proj"), "epic-a", prior); err != nil {
		t.Fatal(err)
	}
	if res := retryAs(t, h, investigateAddr); !res.Refused || res.Reason != wire.ReasonInvestigateStop {
		t.Errorf("retry = %+v, want %s refusal", res, wire.ReasonInvestigateStop)
	}
	if got := ofType(runLog(t, h), events.RecoveryApplied); len(got) != 1 {
		t.Errorf("recovery-applied = %+v, want only the earlier one", got)
	}
}

func TestInvestigateRetry_SecondRetryOfTheKindIsRefused(t *testing.T) {
	t.Parallel()
	h := startInvestigateEnv(t, events.IterationError)
	if res := retryAs(t, h, investigateAddr); res.Refused {
		t.Fatalf("first retry refused: %+v", res)
	}
	h.Server.Registry().Delete("proj:epic-a/01")
	if err := h.Server.PauseAs("proj:epic-a/01", events.IterationError, "boom again"); err != nil {
		t.Fatal(err)
	}
	if res := retryAs(t, h, investigateAddr); !res.Refused {
		t.Errorf("second retry = %+v, want refusal", res)
	}
}

func TestInvestigateRetry_TheForksOwnEventDoesNotSpendTheKindsCap(t *testing.T) {
	t.Parallel()
	h := startInvestigateEnv(t, events.IterationError)
	fork := runlog.Event{Type: string(events.RecoveryApplied), Ticket: "01", Kind: string(events.IterationError), Reason: "investigate", Outcome: investigateAddr}
	if err := runlog.AppendEvent(filepath.Join(h.TicketStore, "proj"), "epic-a", fork); err != nil {
		t.Fatal(err)
	}
	if res := retryAs(t, h, investigateAddr); res.Refused {
		t.Errorf("retry after the fork = %+v, want allowed", res)
	}
}

func TestRetry_APersonStillLogsHumanRelaunch(t *testing.T) {
	t.Parallel()
	h := startInvestigateEnv(t, events.IterationError)
	if res := retryAs(t, h, ""); res.Refused {
		t.Fatalf("retry refused: %+v", res)
	}
	log := runLog(t, h)
	if hr := ofType(log, events.HumanRelaunch); len(hr) != 1 {
		t.Errorf("human-relaunch = %+v, want one", hr)
	}
	if got := ofType(log, events.RecoveryApplied); len(got) != 0 {
		t.Errorf("recovery-applied = %+v, want none", got)
	}
}
