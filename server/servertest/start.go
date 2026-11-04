package servertest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/migrations"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/claudeenv"
	"github.com/elentok/gx/testutil/herdrfake"
)

// SetHerdrDown makes the fake herdr fail (or answer again) from now on.
func (h *Harness) SetHerdrDown(down bool) { h.herdrDown.Store(down) }

// StartHerdrDown is Start with herdr failing from before the server starts.
func StartHerdrDown(t *testing.T, opts ...func(*server.Config)) *Harness {
	t.Helper()
	return startWith(t, t.TempDir(), "", true, opts...)
}

// StartHerdrDownWithStore is StartHerdrDown over a caller-seeded ticket store.
func StartHerdrDownWithStore(t *testing.T, store string, opts ...func(*server.Config)) *Harness {
	t.Helper()
	return startWith(t, store, "", true, opts...)
}

// Start runs a server until the test ends. Tickets already written to
// TicketStore are indexed at startup; since the harness creates TicketStore
// itself, use StartWithStore to seed it first.
func Start(t *testing.T, opts ...func(*server.Config)) *Harness {
	t.Helper()
	return StartWithStore(t, t.TempDir(), opts...)
}

// StartWithStore is Start over a caller-seeded ticket store.
// Options tweak the server config before it starts.
func StartWithStore(t *testing.T, store string, opts ...func(*server.Config)) *Harness {
	t.Helper()
	return startWith(t, store, "", false, opts...)
}

// StartWithTCP is Start with the loopback TCP listener on a free port;
// Harness.TCPAddr is where it bound.
func StartWithTCP(t *testing.T, opts ...func(*server.Config)) *Harness {
	t.Helper()
	return startWith(t, t.TempDir(), "127.0.0.1:0", false, opts...)
}

// shortTempRoot is the temp root of a harness's state dir, short enough for the
// holder sockets below it.
const shortTempRoot = "/tmp"

func startWith(t *testing.T, store, tcpAddr string, herdrDown bool, opts ...func(*server.Config)) *Harness {
	t.Helper()
	// Unix socket paths are capped near 100 bytes; t.TempDir() can exceed that,
	// and so can macOS's default temp root once a holder socket sits under it.
	stateDir, err := os.MkdirTemp(shortTempRoot, "gxs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })
	// Runs after the server stops (cleanups run last-in first-out).
	t.Cleanup(func() {
		if t.Failed() {
			log, _ := os.ReadFile(server.LogPath(filepath.Join(stateDir, "state")))
			t.Logf("server log:\n%s", log)
		}
	})

	// The fake herdr is first on PATH in every test; h.Herdr is only set, and the
	// fake only answers, when the test asked for WithHerdr.
	herdr := herdrfake.NewState(t)
	h := &Harness{
		tb:          t,
		StateDir:    filepath.Join(stateDir, "state"),
		SessionsDir: filepath.Join(stateDir, "sessions"),
		TicketStore: store,

		ConversationsDir: filepath.Join(stateDir, "conversations"),
	}
	// Runs before the temp dir goes, so no session holder outlives the test.
	t.Cleanup(func() { stopSessionHolders(h.SessionsDir) })
	h.herdrDown.Store(herdrDown)
	// The server's herdr probe is `workspace list`; answer it unless told to fail.
	herdr.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		if h.herdrDown.Load() {
			return nil, herdrfake.Identities{}, errors.New("herdr unavailable")
		}
		return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
	})
	_, herdrEnv := herdrfake.StartStateEnv(t, herdr)

	h.cfg = server.Config{StateDir: h.StateDir, SessionsDir: h.SessionsDir, ConversationsDir: h.ConversationsDir, Build: "test-build", TicketStore: h.TicketStore, TCPAddr: tcpAddr}
	// The production defaults: an implement launch with no model is refused.
	h.cfg.AgentsClaude, h.cfg.AgentsCodex = config.DefaultAgentsConfig().Claude, config.DefaultAgentsConfig().Codex
	// An empty registry, so a harness server never runs the real steps over a
	// seeded store. A test that wants migrations sets Config.Migrations.
	h.cfg.Migrations = &migrations.Registry{}
	h.cfg.RecentlyFinished = config.DefaultRecentlyFinished
	// The herdr runner's wait N defaults to 5s, which every launch's slash
	// command prompt would sit out: the fake agent starts no turn.
	h.cfg.RunnerWait = 100 * time.Millisecond
	// The server's env is the process env plus the fake herdr, a poisoned home
	// and a clean config dir of its own, so code that reads HOME where it should
	// read CLAUDE_CONFIG_DIR trips the canary. The merge locks go under the
	// stateDir, not the real home's state dir.
	h.cfg.Env = append(os.Environ(), herdrEnv...)
	// The ticket store is the one repo product code creates in a server test;
	// without this its commits start detached auto maintenance.
	h.cfg.Env = git.WithMaintenanceOff(h.cfg.Env)
	// The holders end with this test process, unless a test passes another pid.
	h.cfg.Env = append(h.cfg.Env, TestPidEnv+"="+strconv.Itoa(os.Getpid()))
	h.cfg.Env = append(h.cfg.Env, "HOME="+filepath.Join(stateDir, "home"))
	h.cfg.Env = append(h.cfg.Env, claudeenv.In(filepath.Join(stateDir, "claude-home"))...)
	// Last, so claudeenv.In's empty XDG_STATE_HOME doesn't win.
	xdgState := filepath.Join(stateDir, "xdg-state")
	if err := os.MkdirAll(xdgState, 0o755); err != nil {
		t.Fatal(err)
	}
	h.cfg.Env = append(h.cfg.Env, "XDG_STATE_HOME="+xdgState)
	// The fake program's side channels, per test rather than shared on the machine.
	h.cfg.Env = append(h.cfg.Env, agentfake.ProgramDirEnv+"="+agentfake.NewProgramDir(t))
	// Every merge pauses on ralphloop's 3s finish debounce; the fake agents
	// settle at once, so a hundredth keeps the order without the wait. The floor
	// keeps the finish debounce longer than a loaded machine's lag between the
	// run starting and its prompt turning the fake busy: a shorter one reads the
	// not-yet-busy fake as finished and reaps a stalled run.
	h.cfg.Sleep = func(d time.Duration) {
		scaled := d / 100
		if d >= time.Second {
			scaled = max(scaled, 300*time.Millisecond)
		}
		time.Sleep(scaled)
	}
	// Fake agents idle without a report and never leave idle after a nudge, so
	// the stall nudge is off unless a test sets its delay.
	h.cfg.StalledNudgeDelay = -1
	// Fake agents idle on run-end lines in unrelated tests; auto-advance is off
	// unless a test sets its countdown.
	h.cfg.AutoAdvanceDelay = -1
	// Tests write a transcript right after the server first looked for it, so
	// a remembered miss would hide it. Negative turns the miss cache off.
	h.cfg.TranscriptMissTTL = -1
	// Short polls so a test that waits for a tick gets it in milliseconds. Setting
	// PollInterval also turns off keepFresh's 15s backstop, so tests no longer
	// depend on store watch events arriving.
	h.cfg.PollInterval = 200 * time.Millisecond
	h.cfg.BudgetPollInterval = 100 * time.Millisecond
	h.cfg.ChatPollInterval = 100 * time.Millisecond
	h.cfg.GuardPoll = 200 * time.Millisecond
	choice := &runnerChoice{herdr: herdrDown}
	choices.Store(&h.cfg, choice)
	t.Cleanup(func() { choices.Delete(&h.cfg) })
	for _, o := range opts {
		o(&h.cfg)
	}
	h.pty = resolveRunner(&h.cfg, choice)
	if choice.herdr {
		h.Herdr = herdr
	} else {
		herdr.ForbidExcept("workspace", "list")
	}
	if h.cfg.Notifier == nil {
		h.Banners = &Banners{}
		h.cfg.Notifier = h.Banners
	}
	if h.cfg.Player == nil {
		h.Sounds = &Sounds{}
		h.cfg.Player = h.Sounds
	}
	if h.cfg.SessionBannerDelay == 0 {
		h.cfg.SessionBannerDelay = SessionBannerDelay
	}
	// The real check reads ~/.claude.json, which would leak an extra-usage
	// notice into the chat on a machine that has it on.
	if h.cfg.ExtraUsageCheck == nil {
		h.cfg.ExtraUsageCheck = func() subscription.State { return subscription.StateDisabled }
	}
	// The real source reads the test machine's disk, and a default-threshold
	// reload could then send a warn notice into the chat.
	h.Disks = &FakeDisks{}
	if h.cfg.FreeSpace == nil {
		h.cfg.FreeSpace = h.Disks.Read
	}
	// Every program is on PATH unless a test marks it missing: the suites run
	// a fake in place of claude, and the machine may have no claude at all.
	h.Programs = &Programs{}
	if h.cfg.LookPath == nil {
		h.cfg.LookPath = h.Programs.LookPath
	}
	// The real source reads the test machine's CPU and memory.
	h.System = NewFakeSystem()
	if h.cfg.SystemHealth == nil {
		h.cfg.SystemHealth = h.System.Read
	}
	// The real source reads the test machine's process table.
	h.Processes = NewFakeProcesses()
	if h.cfg.ProcessSource == nil {
		h.cfg.ProcessSource = h.Processes
	}
	// The real runner would empty the developer's Go cache on a low-disk machine.
	if h.cfg.CleanupRunner == nil {
		h.cfg.CleanupRunner = func(context.Context, []string, string) ([]byte, error) { return nil, nil }
	}
	// The real sweep would delete the developer's stale temp dirs.
	if h.cfg.TempSweepDir == "" {
		h.cfg.TempSweepDir = t.TempDir()
	}
	// A real process scan would see the `go test` running this very test and wait.
	if h.cfg.CacheGateScan == nil {
		h.cfg.CacheGateScan = func() (bool, error) { return false, nil }
	}
	if h.cfg.GoCacheDir == nil {
		goCache := t.TempDir()
		h.cfg.GoCacheDir = func() (string, error) { return goCache, nil }
	}
	h.serve(t)
	return h
}

// Env is key as the server sees it in its Config.Env.
func (h *Harness) Env(key string) string { return harness.LookupEnv(h.cfg.Env, key) }

// Restart stops the server and starts a new one on the same state dir, as a
// crash-and-relaunch would.
func (h *Harness) Restart(t *testing.T) {
	t.Helper()
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	h.serve(t)
}

// DropAgentsAtCleanup adds runners whose agents cleanup drops before it waits
// for runs to settle, for runners the config only reaches by name.
func (h *Harness) DropAgentsAtCleanup(rs ...agentrunner.Runner) {
	h.extraRunners = append(h.extraRunners, rs...)
}

// runners is every runner cleanup drops the agents of. RunnerFor is asked with
// empty names, which the test doubles that set it ignore.
func (h *Harness) runners() []agentrunner.Runner {
	rs := append([]agentrunner.Runner{h.cfg.Runner}, h.extraRunners...)
	if h.cfg.RunnerFor != nil {
		rs = append(rs, h.cfg.RunnerFor("", ""))
	}
	return rs
}

func (h *Harness) serve(t *testing.T) {
	t.Helper()
	if h.pty {
		h.buildPty()
	}
	// A hold sleeps until its reset (an hour for a rate limit), which even
	// scaled down outlasts WaitSettled below. Cleanup cuts the sleep short.
	stopping := make(chan struct{})
	if sleep := h.cfg.Sleep; sleep != nil {
		h.cfg.Sleep = func(d time.Duration) {
			woke := make(chan struct{})
			go func() { sleep(d); close(woke) }()
			select {
			case <-woke:
			case <-stopping:
			}
		}
	}
	srv, err := server.New(h.cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.Server = srv
	trustWebRoot(h.StateDir)
	h.TCPAddr = srv.TCPAddr()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx) }()
	var once sync.Once
	h.Stop = func() error {
		var err error
		once.Do(func() { cancel(); err = <-done })
		return err
	}
	t.Cleanup(func() {
		close(stopping)
		if err := h.Stop(); err != nil {
			t.Errorf("server: %v", err)
		}
		// Runs outlive Stop and still write to the ticket store; let them end
		// before t.TempDir's cleanup removes it. Their wait on an agent never
		// ends on its own, so drop the agents first.
		for _, r := range h.runners() {
			if sa, ok := r.(interface{ StopAll() }); ok {
				sa.StopAll()
			}
		}
		if h.pty {
			h.stopAgents()
		}
		if !srv.WaitSettled(10 * time.Second) {
			t.Logf("servertest: a run was still settling 10s after stop")
		}
	})

	h.Client = apiclient.New(server.SocketPath(h.StateDir))
}
