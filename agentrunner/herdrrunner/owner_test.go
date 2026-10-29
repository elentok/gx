package herdrrunner_test

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/herdrrunner"
	"github.com/elentok/gx/testutil/herdrfake"
)

// ownerHarness fakes herdr's pane process-info and counts the calls. The fake
// herdr handler runs on the fake server's goroutine, so mu guards the fields
// it shares with the test: execs, argv and herdrFails.
type ownerHarness struct {
	mu          sync.Mutex
	execs       int
	argv        []string
	herdrFails  bool
	startReads  int
	startFails  bool
	runner      *herdrrunner.Runner
	paneSession agentrunner.Session
}

func newOwnerHarness(t *testing.T) *ownerHarness {
	h := &ownerHarness{paneSession: agentrunner.Session{Label: "x", ID: "w1:p3"}}
	_, fakeEnv := herdrfake.StartEnv(t, func(argv []string) ([]byte, int) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.execs++
		h.argv = argv
		if h.herdrFails {
			return herdrfake.CommandError("boom")
		}
		return herdrfake.Result(map[string]any{"process_info": map[string]any{"shell_pid": 555}})
	})
	h.runner = newTestRunner(fakeEnv)
	h.runner.PIDStart = func(pid int) (int64, error) {
		h.startReads++
		if h.startFails {
			return 0, errors.New("no start")
		}
		return 1700000000 + int64(pid), nil
	}
	return h
}

func (h *ownerHarness) setHerdrFails(fail bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.herdrFails = fail
}

// calls returns the herdr exec count and the last argv.
func (h *ownerHarness) calls() (int, []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.execs, h.argv
}

func TestOwnerPIDs_CachesPerPane(t *testing.T) {
	h := newOwnerHarness(t)
	want := []agentrunner.OwnerPID{{PID: 555, Start: 1700000555}}
	for range 2 {
		got, err := agentrunner.OwnerPIDs(h.runner, h.paneSession)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("OwnerPIDs = %v, %v; want %v", got, err, want)
		}
	}
	execs, argv := h.calls()
	if execs != 1 || h.startReads != 1 {
		t.Errorf("herdr execs = %d, start reads = %d; want 1 and 1", execs, h.startReads)
	}
	if want := []string{"pane", "process-info", "--pane", "w1:p3"}; !slices.Equal(argv, want) {
		t.Errorf("argv = %v, want %v", argv, want)
	}
}

func TestOwnerPIDs_FailuresReturnNoPidsAndAreRetried(t *testing.T) {
	h := newOwnerHarness(t)

	h.setHerdrFails(true)
	if got, err := agentrunner.OwnerPIDs(h.runner, h.paneSession); err == nil || len(got) != 0 {
		t.Fatalf("herdr failure: %v, %v; want no pids and an error", got, err)
	}
	h.setHerdrFails(false)
	h.startFails = true
	if got, err := agentrunner.OwnerPIDs(h.runner, h.paneSession); err == nil || len(got) != 0 {
		t.Fatalf("start failure: %v, %v; want no pids and an error", got, err)
	}
	h.startFails = false
	got, err := agentrunner.OwnerPIDs(h.runner, h.paneSession)
	if err != nil || len(got) != 1 {
		t.Fatalf("retry: %v, %v; want one pid", got, err)
	}
	if execs, _ := h.calls(); execs != 3 {
		t.Errorf("herdr execs = %d, want 3 (failures are not cached)", execs)
	}
}
