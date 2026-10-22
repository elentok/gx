package server_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/runnerfake"
	"github.com/elentok/gx/tickets/schema"
)

// renamedFake is a headlessFake answering to name, so two of them are two runners.
type renamedFake struct {
	*headlessFake
	name string
}

func (r renamedFake) Name() string { return r.name }

func newRenamedFake(name string, verdict nativerunner.Verdict) renamedFake {
	return renamedFake{&headlessFake{Runner: runnerfake.NewRunner(), verdict: verdict}, name}
}

// restartUnder leaves an iteration whose handle names savedRunner ("" keeps
// the herdr handle), then restarts a server configured with configured, whose
// RunnerFor builds only the runners in others. It returns the ticket path.
func restartUnder(t *testing.T, savedRunner string, configured agentrunner.Runner, others map[string]agentrunner.Runner, opts ...func(*server.Config)) (*servertest.Harness, string) {
	t.Helper()
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store, append([]func(*server.Config){func(c *server.Config) {
		c.Runner = configured
		c.RunnerFor = func(name, _ string) agentrunner.Runner {
			if name == configured.Name() {
				return configured
			}
			return others[name]
		}
	}}, opts...)...)
	registerLiveAgent(h)
	h.DropAgentsAtCleanup(configured)
	for _, r := range others {
		h.DropAgentsAtCleanup(r)
	}
	_, ticketPath := leaveIteration(t, h, store, repo)
	if savedRunner != "" {
		asHeadlessRun(t, h)
		setSavedRunner(t, h, savedRunner)
	}
	h.Restart(t)
	return h, ticketPath
}

func setSavedRunner(t *testing.T, h *servertest.Harness, name string) {
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

func waitStatus(t *testing.T, ticketPath string, want schema.Status) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		tk, err := schema.ParseTicket(ticketPath)
		if err != nil {
			t.Fatal(err)
		}
		// A legacy needs-answer / needs-repair want means paused in that class.
		if class := legacyClass(want); class != "" {
			if tk.Status == schema.StatusPaused && tk.PausedClass() == class {
				return
			}
		} else if tk.Status == want {
			return
		}
		if time.Now().After(deadline) {
			body, _ := os.ReadFile(ticketPath)
			t.Fatalf("status = %q, want %q:\n%s", tk.Status, want, body)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func cleanedBy(f renamedFake) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.cleaned)
}

func TestReclaim_FinishesThroughTheRunsOwnRunner(t *testing.T) {
	own, configured := newRenamedFake("a", nativerunner.VerdictFinished), newRenamedFake("b", nativerunner.VerdictFinished)
	_, ticketPath := restartUnder(t, "a", configured, map[string]agentrunner.Runner{"a": own})
	waitStatus(t, ticketPath, schema.StatusDone)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for ctx.Err() == nil && len(cleanedBy(own)) == 0 {
		time.Sleep(50 * time.Millisecond)
	}
	if got := cleanedBy(own); !slices.Equal(got, []string{"epic-a-iter-01"}) {
		t.Errorf("run's runner cleaned %v, want epic-a-iter-01", got)
	}
	if got := cleanedBy(configured); len(got) != 0 {
		t.Errorf("configured runner cleaned %v, want nothing", got)
	}
}

// Regression: a herdr run reclaimed under a headless server was waited on and
// finished through headless, which had no such session, and was left unwatched.
func TestReclaim_HerdrRunUnderAHeadlessServerFinishesThroughHerdr(t *testing.T) {
	headless := newRenamedFake("headless", nativerunner.VerdictLive)
	_, ticketPath := restartUnder(t, "", headless, map[string]agentrunner.Runner{"herdr": herdrrunner.New()})
	waitStatus(t, ticketPath, schema.StatusDone)
}

func TestReclaim_NonMismatchErrorParksNeedsRepairWithItsReason(t *testing.T) {
	store, repo := t.TempDir(), testutil.TempRepo(t)
	servertest.WriteTicket(t, store, "proj", "epic-a", "01", "first", "")
	servertest.SetProjectRepo(t, store, "proj", repo)
	h := servertest.StartWithStore(t, store)
	registerLiveAgent(h)
	_, ticketPath := leaveIteration(t, h, store, repo)
	path := filepath.Join(h.StateDir, "runs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"root":"proj:epic-a"`, `"root":"corrupt"`, 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	h.Restart(t)
	waitStatus(t, ticketPath, schema.StatusNeedsRepair)
	body, _ := os.ReadFile(ticketPath)
	if !strings.Contains(string(body), `invalid root "corrupt"`) {
		t.Errorf("ticket lacks the reclaim error in its reason:\n%s", body)
	}
}

func TestReclaim_UnavailableRunnerParksNeedsRepairNamingIt(t *testing.T) {
	_, ticketPath := restartUnder(t, "pty", newRenamedFake("headless", nativerunner.VerdictLive), nil)
	waitStatus(t, ticketPath, schema.StatusNeedsRepair)
	body, _ := os.ReadFile(ticketPath)
	if !strings.Contains(string(body), "started by pty; pty unavailable") {
		t.Errorf("ticket lacks the runner in its reason:\n%s", body)
	}
}
