package ralphloop

import (
	"errors"
	"slices"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/testutil/runnerfake"
)

type launchFail struct {
	attempt int
	kind    events.Kind
}

func startAndPromptFake(t *testing.T, r *runnerfake.Runner) (agentrunner.Session, error, []launchFail) {
	t.Helper()
	var fails []launchFail
	l, err := startAndPrompt(r, agentrunner.StartOptions{Label: "it-01", Epic: "epic", Cwd: "/wt"}, "/go", func(attempt int, kind events.Kind, _ error) {
		fails = append(fails, launchFail{attempt, kind})
	})
	return l.Session, err, fails
}

func TestStartAndPrompt_DeliversPrompt(t *testing.T) {
	r := runnerfake.NewRunner()
	s, err, fails := startAndPromptFake(t, r)
	if err != nil || len(fails) != 0 {
		t.Fatalf("err = %v, fails = %v", err, fails)
	}
	if s.SessionID == "" {
		t.Fatal("session id missing")
	}
	if got := r.Prompts("it-01"); !slices.Equal(got, []string{"/go"}) {
		t.Fatalf("prompts = %v", got)
	}
}

func TestStartAndPrompt_NotDeliveredRetriesOnFreshSession(t *testing.T) {
	r := runnerfake.NewRunner()
	r.FailNextPrompts("it-01", 1, agentrunner.ErrNotDelivered)
	s, err, fails := startAndPromptFake(t, r)
	if err != nil {
		t.Fatal(err)
	}
	if want := []launchFail{{1, events.AgentPromptStalled}}; !slices.Equal(fails, want) {
		t.Fatalf("fails = %v, want %v", fails, want)
	}
	if got := r.Prompts("it-01"); !slices.Equal(got, []string{"/go"}) {
		t.Fatalf("fresh session prompts = %v", got)
	}
	live, _ := r.List("epic")
	if len(live) != 1 || live[0].ID != s.ID {
		t.Fatalf("live sessions = %v, want only %v", live, s)
	}
}

func TestStartAndPrompt_NotDeliveredTwiceParksStalled(t *testing.T) {
	r := runnerfake.NewRunner()
	r.FailNextPrompts("it-01", 2, agentrunner.ErrNotDelivered)
	_, err, fails := startAndPromptFake(t, r)
	var lf *launchFailure
	if !errors.As(err, &lf) || lf.Kind != events.AgentPromptStalled {
		t.Fatalf("err = %v, want AgentPromptStalled launchFailure", err)
	}
	if len(fails) != 2 {
		t.Fatalf("fails = %v, want two attempts", fails)
	}
	if live, _ := r.List("epic"); len(live) != 0 {
		t.Fatalf("stalled session left running: %v", live)
	}
}

func TestStartAndPrompt_LabelTakenIsNameTaken(t *testing.T) {
	r := runnerfake.NewRunner()
	r.SetStartErr("it-01", agentrunner.ErrLabelTaken)
	_, err, _ := startAndPromptFake(t, r)
	var lf *launchFailure
	if !errors.As(err, &lf) || lf.Kind != events.AgentNameTaken {
		t.Fatalf("err = %v, want AgentNameTaken launchFailure", err)
	}
}

func TestStartAndPrompt_NotReadyKeepsSessionForBlockedPark(t *testing.T) {
	r := runnerfake.NewRunner()
	r.FailNextPrompts("it-01", 1, agentrunner.ErrNotReady)
	s, err, fails := startAndPromptFake(t, r)
	if !errors.Is(err, agentrunner.ErrNotReady) || len(fails) != 0 {
		t.Fatalf("err = %v, fails = %v", err, fails)
	}
	if live, _ := r.List("epic"); len(live) != 1 || live[0].ID != s.ID {
		t.Fatalf("blocked session not kept: %v", live)
	}
}
