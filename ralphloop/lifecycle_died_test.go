package ralphloop

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/ralphloop/internal/fakes"
	"github.com/elentok/gx/ralphloop/supervise"
	"github.com/elentok/gx/ralphloop/worktree"
)

func TestWaitIterationFinished_SessionDied_ReturnsDiedReason(t *testing.T) {
	t.Parallel()
	const epic = "my-epic"
	label := worktree.IterationLabel(epic, "01")
	d, _, _ := fakes.FakeDeps()
	r := fakes.FakeRunner(d)
	r.DieAfterPrompt(label, agentrunner.SessionDied("claude exited (code 1)"))
	session, err := supervise.StartAndPrompt(context.Background(), r, agentrunner.StartOptions{Label: label, Epic: epic}, "/gx-implement gx:my-epic/01", func(int, string) {}, nil)
	if err != nil {
		t.Fatalf("StartAndPrompt = %v", err)
	}

	err = WaitIterationFinished(d, OneIteration{Agent: agent.AgentClaude, Epic: epic}, IterationWorktree{Label: label}, session)

	if !errors.Is(err, agentrunner.ErrSessionDied) || !strings.Contains(err.Error(), "claude exited (code 1)") {
		t.Errorf("WaitIterationFinished = %v, want ErrSessionDied carrying the died reason", err)
	}
}
