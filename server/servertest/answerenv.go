package servertest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/runs"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// AnswerLabel is the session label of the iteration AnswerEnv starts.
const AnswerLabel = "epic-a-iter-01"

// AnswerEnv is a server with ticket 01 of epic-a and one live fake session.
type AnswerEnv struct {
	H      *Harness
	Fake   *runnerfake.Runner
	Ticket string
}

// StartAnswerEnv starts an AnswerEnv; wrap, when set, replaces the runner the
// server sees (the fake stays reachable as Fake).
func StartAnswerEnv(t *testing.T, wrap func(*runnerfake.Runner) agentrunner.Runner, opts ...func(*server.Config)) AnswerEnv {
	t.Helper()
	store := t.TempDir()
	WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	SetProjectRepo(t, store, "proj", t.TempDir())
	fake := runnerfake.NewRunner()
	var runner agentrunner.Runner = fake
	if wrap != nil {
		runner = wrap(fake)
	}
	h := StartWithStore(t, store, append([]func(*server.Config){WithRunner(runner)}, opts...)...)
	if _, err := fake.Start(agentrunner.StartOptions{Label: AnswerLabel, Epic: "epic-a", Cwd: t.TempDir(), Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	return AnswerEnv{h, fake, filepath.Join(store, "proj", "epic-a", "issues", "01-first.md")}
}

// PermissionEvents returns the pause.permission events logged on the ticket.
func (e AnswerEnv) PermissionEvents(t *testing.T) []tickets.LogEvent {
	t.Helper()
	return logEventsOfType(t, e.Ticket, tickets.EventPausePermission)
}

// Answer sends req for ticket 01 through the client.
func (e AnswerEnv) Answer(t *testing.T, req wire.QueueRequest) wire.QueueResult {
	t.Helper()
	req.Address = "proj:epic-a/01"
	res, err := e.H.Client.Tickets.AgentAnswerRequest(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// MustFind returns the session labelled AnswerLabel.
func MustFind(t *testing.T, r *runnerfake.Runner) agentrunner.Session {
	t.Helper()
	s, ok, err := r.Find(AnswerLabel)
	if err != nil || !ok {
		t.Fatalf("find = %v, %v", ok, err)
	}
	return s
}

// PauseEnvZeroCommit pauses ticket 01 of the answer env as zero-commit and
// returns the state token of its question.
func PauseEnvZeroCommit(t *testing.T, e AnswerEnv) string {
	t.Helper()
	if err := e.H.Server.PauseAs(AnswerAddr, events.ZeroCommit, "no commits"); err != nil {
		t.Fatal(err)
	}
	return StateOf(t, e.Ticket)
}

// StateOf is the state token ("seq:class") of the ticket's open question.
func StateOf(t *testing.T, ticketPath string) string {
	t.Helper()
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	open := tickets.OpenQuestion(evs)
	if open == nil {
		t.Fatal("no open question")
	}
	tk, err := schema.ParseTicket(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%d:%s", open.Seq, tk.PausedClass())
}

// AnswerEvents returns the pause.answer events logged on the ticket.
func AnswerEvents(t *testing.T, ticketPath string) []tickets.LogEvent {
	t.Helper()
	return logEventsOfType(t, ticketPath, tickets.EventPauseAnswer)
}

func logEventsOfType(t *testing.T, ticketPath string, typ string) []tickets.LogEvent {
	t.Helper()
	evs, err := tickets.ReadLog(ticketPath)
	if err != nil {
		t.Fatal(err)
	}
	var out []tickets.LogEvent
	for _, ev := range evs {
		if ev.Type == typ {
			out = append(out, ev)
		}
	}
	return out
}

// RegisterLiveAgent makes the fake herdr report agent p1 as idle.
func RegisterLiveAgent(h *Harness) {
	idle := map[string]any{"agent": map[string]any{"pane_id": "p1", "tab_id": "t1", "agent_status": "idle", "state_change_seq": 1}}
	for _, verb := range []string{"get", "wait"} {
		h.Herdr.Register("agent", verb, func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
			return idle, herdrfake.Identities{}, nil
		})
	}
	h.Herdr.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{}, herdrfake.Identities{}, nil
	})
}

// RestartUnder leaves an iteration whose handle names savedRunner ("" keeps
// the herdr handle), then restarts a server configured with configured, whose
// RunnerFor builds only the runners in others. It returns the ticket path.
func RestartUnder(t *testing.T, savedRunner string, configured agentrunner.Runner, others map[string]agentrunner.Runner, opts ...func(*server.Config)) (*Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	SetProjectRepo(t, store, "proj", repo)
	h := StartWithStore(t, store, append([]func(*server.Config){WithRunnerFor(configured, func(name, _ string) agentrunner.Runner {
		if name == configured.Name() {
			return configured
		}
		return others[name]
	})}, opts...)...)
	if h.Herdr != nil {
		RegisterLiveAgent(h)
	}
	h.DropAgentsAtCleanup(configured)
	for _, r := range others {
		h.DropAgentsAtCleanup(r)
	}
	_, ticketPath := LeaveIteration(t, h, store, repo, "herdr")
	if savedRunner != "" {
		AsHeadlessRun(t, h)
		setSavedRunner(t, h, savedRunner)
	}
	h.Restart(t)
	return h, ticketPath
}

func setSavedRunner(t *testing.T, h *Harness, name string) {
	t.Helper()
	path := filepath.Join(h.StateDir, "runs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"runner":"fake"`, `"runner":"`+name+`"`, 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// LiveUnderOther restarts a server configured with runner "b" over a live,
// working run that runner "a" started. It returns the harness, a's fake and
// the ticket path.
func LiveUnderOther(t *testing.T, opts ...func(*server.Config)) (*Harness, RenamedFake, string) {
	t.Helper()
	return LiveUnder(t, NewRenamedFake("a", nativerunner.VerdictLive), opts...)
}

// LiveUnder is LiveUnderOther with the run's runner given as own.
func LiveUnder(t *testing.T, own RenamedFake, opts ...func(*server.Config)) (*Harness, RenamedFake, string) {
	t.Helper()
	own.IDs = func(string) (string, string) { return "p1", "sess" }
	own.PromptState = agentrunner.StateWorking
	if _, err := own.Start(agentrunner.StartOptions{Label: AnswerLabel, Epic: "epic-a", Cwd: t.TempDir(), Kind: "claude"}); err != nil {
		t.Fatal(err)
	}
	own.SetState(AnswerLabel, agentrunner.StateWorking, "")
	h, ticketPath := RestartUnder(t, own.Name(), NewRenamedFake("b", nativerunner.VerdictLive), map[string]agentrunner.Runner{own.Name(): own}, opts...)
	if !Poll(10*time.Second, func() bool {
		return slices.ContainsFunc(h.Server.Runs(), func(r runs.Run) bool { return r.Runner == own.Name() })
	}) {
		t.Fatalf("run not reclaimed under %s: %+v", own.Name(), h.Server.Runs())
	}
	return h, own, ticketPath
}
