// Package servertest is seam A: an in-process server on a temp state dir,
// temp socket and temp ticket store, with the fake herdr executable first in
// PATH, driven only through the API client. Like herdrfake.Start it calls
// t.Setenv, so tests using it must not be parallel, and the package's
// TestMain must call herdrfake.RunHelperProcess.
package servertest

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/config"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/testutil"
	"github.com/elentok/gx/testutil/herdrfake"
)

// Harness is a running test server.
type Harness struct {
	Client      *apiclient.Client
	Server      *server.Server // for reading the run registry; drive everything else through Client
	Herdr       *herdrfake.State
	StateDir    string
	SessionsDir string // the interactive sessions' state dirs
	TicketStore string
	TCPAddr     string // empty unless started with StartWithTCP
	// Banners records the OS banners the server sent, unless an option set
	// its own Notifier. A harness never shows a real banner.
	Banners *Banners
	// Sounds records the session sounds played, unless an option set its own
	// Player.
	Sounds    *Sounds
	herdrDown atomic.Bool

	extraRunners []agentrunner.Runner
	// Stop does what SIGTERM does to the real server: shut down, release the
	// lock, and return Serve's result. Safe to call more than once.
	Stop func() error

	cfg server.Config
}

// WriteTicket creates <store>/<project>/<epic>/issues/<id>-<slug>.md (and the
// project's project.json) with minimal valid frontmatter. Call it before Start.
// WithConfigFile points the server's config reload at path.
func WithConfigFile(path string) func(*server.Config) {
	return func(c *server.Config) { c.ConfigPath = path }
}

// WriteConfig writes fields as a config.json at path. The ticket-store path
// is set to store unless fields names one, so a reload sees no startup-only
// change by default. server.watch-config is off unless fields sets it, so a
// command reload never starts a watcher behind a test's back.
func WriteConfig(t *testing.T, path, store string, fields map[string]any) {
	t.Helper()
	all := map[string]any{"ticket-store": map[string]any{"path": store}}
	for k, v := range fields {
		all[k] = v
	}
	srv := map[string]any{"watch-config": false}
	if given, ok := all["server"].(map[string]any); ok {
		maps.Copy(srv, given)
	}
	all["server"] = srv
	data, err := json.Marshal(all)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func WriteTicket(t *testing.T, store, project, epic, id, slug, blockedBy string) {
	t.Helper()
	opts := TicketOpts{}
	if blockedBy != "" {
		opts.BlockedBy = []string{blockedBy}
	}
	WriteTicketWith(t, store, project, epic, id, slug, opts)
}

// TicketOpts are the frontmatter fields WriteTicketWith can set beyond the minimum.
type TicketOpts struct {
	Type      string // default "implement"
	Parent    string
	BlockedBy []string
	Base      string
	Status    string // default "open"
	Agent     string // the ticket's agent: frontmatter field, omitted when empty
}

// WriteTicketWith is WriteTicket with a type, a parent and several blockers. Unlike
// WriteTicket it may also run after Start: that is how an agent's turn splits its ticket.
func WriteTicketWith(t *testing.T, store, project, epic, id, slug string, opts TicketOpts) {
	t.Helper()
	projectDir := filepath.Join(store, project)
	issues := filepath.Join(projectDir, epic, "issues")
	if err := os.MkdirAll(issues, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.EnsureEpicTicketMD(t, filepath.Join(projectDir, epic))
	// Keep an existing project.json: SetProjectRepo's repo must survive a mid-run write.
	if _, err := os.Stat(filepath.Join(projectDir, "project.json")); os.IsNotExist(err) {
		if err := os.WriteFile(filepath.Join(projectDir, "project.json"), []byte(`{"name":"`+project+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if opts.Type == "" {
		opts.Type = "implement"
	}
	if opts.Status == "" {
		opts.Status = "open"
	}
	fm := "---\nid: \"" + id + "\"\nstatus: " + opts.Status + "\ntype: " + opts.Type + "\n"
	if opts.Parent != "" {
		fm += "parent: \"" + opts.Parent + "\"\n"
	}
	if len(opts.BlockedBy) > 0 {
		fm += "blocked_by: [\"" + strings.Join(opts.BlockedBy, "\", \"") + "\"]\n"
	}
	if opts.Base != "" {
		fm += "base: " + opts.Base + "\n"
	}
	if opts.Agent != "" {
		fm += "agent: " + opts.Agent + "\n"
	}
	fm += "---\n\n# " + slug + "\n"
	if err := os.WriteFile(filepath.Join(issues, id+"-"+slug+".md"), []byte(fm), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Prompt is one `agent prompt` the server sent to a launched agent.
type Prompt struct {
	Address string // the ticket the skill prompt names
	Cwd     string // the worktree the agent's tab was opened in
	Text    string // the whole prompt, notes included
}

// RegisterLaunch answers the herdr commands a launch makes, giving each tab its
// own pane and cwd, and runs agent for every prompt (the agent's turn, e.g. a
// commit in p.Cwd). The agent is idle again when the prompt returns.
func (h *Harness) RegisterLaunch(agent func(p Prompt)) {
	var mu sync.Mutex
	cwds := map[string]string{}  // pane -> cwd
	named := map[string]string{} // agent name -> pane
	turns := map[string]int{}    // pane -> state_change_seq
	tabs := 0
	reply := func(pane string) map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return map[string]any{"agent": map[string]any{
			"pane_id": pane, "tab_id": "t" + strings.TrimPrefix(pane, "p"),
			"agent_status": "idle", "state_change_seq": turns[pane],
		}}
	}
	ok := func() (any, herdrfake.Identities, error) { return map[string]any{}, herdrfake.Identities{}, nil }
	h.Herdr.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "get", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		pane, byName := named[argv[2]]
		_, byPane := cwds[argv[2]]
		mu.Unlock()
		switch {
		case byName:
			return reply(pane), herdrfake.Identities{}, nil
		case byPane:
			return reply(argv[2]), herdrfake.Identities{}, nil
		}
		return nil, herdrfake.Identities{}, AgentNotFound(argv[2])
	})
	h.Herdr.Register("workspace", "create", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		return map[string]any{"workspace": map[string]any{"workspace_id": "w1"}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("tab", "create", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		mu.Lock()
		defer mu.Unlock()
		tabs++
		pane := "p" + strconv.Itoa(tabs)
		for i, a := range argv {
			if a == "--cwd" && i+1 < len(argv) {
				cwds[pane] = argv[i+1]
			}
		}
		return map[string]any{"tab": map[string]any{"tab_id": "t" + strconv.Itoa(tabs)}, "root_pane": map[string]any{"pane_id": pane}}, herdrfake.Identities{}, nil
	})
	h.Herdr.Register("tab", "close", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		// Closing the tab ends the agent in its pane, so a relaunch starts fresh.
		pane := "p" + strings.TrimPrefix(argv[2], "t")
		mu.Lock()
		defer mu.Unlock()
		delete(cwds, pane)
		for name, p := range named {
			if p == pane {
				delete(named, name)
			}
		}
		return ok()
	})
	h.Herdr.Register("agent", "start", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		pane := flagValue(argv, "--pane")
		mu.Lock()
		named[argv[2]] = pane
		mu.Unlock()
		return reply(pane), herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		return reply(argv[2]), herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		pane, text := argv[2], argv[3]
		mu.Lock()
		cwd := cwds[pane]
		turns[pane]++ // the prompt starts a turn
		mu.Unlock()
		_, rest, _ := strings.Cut(text, " ")
		addr, _, _ := strings.Cut(rest, "\n")
		addr = strings.TrimPrefix(addr, "--ticket ") // gx-merge's form
		agent(Prompt{Address: addr, Cwd: cwd, Text: text})
		return reply(pane), herdrfake.Identities{}, nil
	})
}

// AgentNotFound is the error a fake `herdr agent get` returns for an unknown target.
func AgentNotFound(target string) error {
	return errors.New(`{"error":{"code":"agent_not_found","message":"agent target ` + target + ` not found"}}`)
}

func flagValue(argv []string, flag string) string {
	for i, a := range argv {
		if a == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
}

// Banners is an osnotify.Notifier that records every banner.
type Banners struct {
	mu   sync.Mutex
	sent []osnotify.Notification
}

func (b *Banners) Notify(_ context.Context, note osnotify.Notification) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, note)
	return nil
}

// Sent returns the banners recorded so far.
func (b *Banners) Sent() []osnotify.Notification {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.sent)
}

// SessionBannerDelay is a harness's session banner delay: long enough for a
// test to merge a second trigger inside the window, shorter than the real one.
const SessionBannerDelay = 300 * time.Millisecond

// Sounds is a sound.Player that records every sound played.
type Sounds struct {
	mu     sync.Mutex
	played []sound.Name
}

func (p *Sounds) Play(_ context.Context, _ config.AgentsSoundsConfig, name sound.Name) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.played = append(p.played, name)
}

// Played returns the sounds recorded so far.
func (p *Sounds) Played() []sound.Name {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.played)
}

// WaitForEvent returns the first streamed event of typ for address (any
// address when empty), failing the test if the stream ends or ctx expires first.
func WaitForEvent(ctx context.Context, t *testing.T, evs <-chan server.Event, typ, address string) server.Event {
	t.Helper()
	for {
		select {
		case ev, open := <-evs:
			if !open {
				t.Fatalf("event stream ended before %s %s", typ, address)
			}
			if ev.Type == typ && (address == "" || ev.Address == address) {
				return ev
			}
		case <-ctx.Done():
			t.Fatalf("no %s event for %q: %v", typ, address, ctx.Err())
		}
	}
}

// SetProjectRepo points the project at the repo its agents run in. Call it
// after WriteTicket, before Start. It opts the project in to auto-ff-merge so a
// finished epic merges; SetProjectRepoNoAutoMerge leaves the default (off).
func SetProjectRepo(t *testing.T, store, project, repo string) {
	t.Helper()
	writeProjectRepo(t, store, project, repo, `,"auto-ff-merge":true`)
}

// SetProjectTrunk is SetProjectRepo with project.json's trunk set. Trunk is
// read live, so it may also run mid-run.
func SetProjectTrunk(t *testing.T, store, project, repo, trunk string) {
	t.Helper()
	writeProjectRepo(t, store, project, repo, `,"auto-ff-merge":true,"trunk":"`+trunk+`"`)
}

// SetProjectTrunkNoAutoMerge is SetProjectTrunk without the auto-ff-merge opt-in.
func SetProjectTrunkNoAutoMerge(t *testing.T, store, project, repo, trunk string) {
	t.Helper()
	writeProjectRepo(t, store, project, repo, `,"trunk":"`+trunk+`"`)
}

// SetProjectRepoNoAutoMerge is SetProjectRepo without the auto-ff-merge opt-in.
func SetProjectRepoNoAutoMerge(t *testing.T, store, project, repo string) {
	t.Helper()
	writeProjectRepo(t, store, project, repo, "")
}

func writeProjectRepo(t *testing.T, store, project, repo, extra string) {
	t.Helper()
	body := `{"name":"` + project + `","repo":"` + repo + `"` + extra + `}`
	if err := os.WriteFile(filepath.Join(store, project, "project.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// SetHerdrDown makes the fake herdr fail (or answer again) from now on.
func (h *Harness) SetHerdrDown(down bool) { h.herdrDown.Store(down) }

// StartHerdrDown is Start with herdr failing from before the server starts.
func StartHerdrDown(t *testing.T, opts ...func(*server.Config)) *Harness {
	t.Helper()
	return startWith(t, t.TempDir(), "", true, opts...)
}

// Start runs a server until the test ends. Tickets already written to
// TicketStore are indexed at startup; since the harness creates TicketStore
// itself, use StartWithStore to seed it first.
func Start(t *testing.T) *Harness {
	t.Helper()
	return StartWithStore(t, t.TempDir())
}

// StartWithStore is Start over a caller-seeded ticket store.
// Options tweak the server config before it starts.
func StartWithStore(t *testing.T, store string, opts ...func(*server.Config)) *Harness {
	t.Helper()
	return startWith(t, store, "", false, opts...)
}

// StartWithTCP is Start with the loopback TCP listener on a free port;
// Harness.TCPAddr is where it bound.
func StartWithTCP(t *testing.T) *Harness {
	t.Helper()
	return startWith(t, t.TempDir(), "127.0.0.1:0", false)
}

// processHome is HOME as the test process started, to tell a test that set its
// own from one that left the real one.
var processHome = os.Getenv("HOME")

func startWith(t *testing.T, store, tcpAddr string, herdrDown bool, opts ...func(*server.Config)) *Harness {
	t.Helper()
	// Unix socket paths are capped near 100 bytes; t.TempDir() can exceed that.
	stateDir, err := os.MkdirTemp("", "gxs")
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

	h := &Harness{
		Herdr:       herdrfake.NewState(t),
		StateDir:    filepath.Join(stateDir, "state"),
		SessionsDir: filepath.Join(stateDir, "sessions"),
		TicketStore: store,
	}
	// Runs before the temp dir goes, so no session holder outlives the test.
	t.Cleanup(func() { stopSessionHolders(h.SessionsDir) })
	h.herdrDown.Store(herdrDown)
	// The server's herdr probe is `workspace list`; answer it unless told to fail.
	h.Herdr.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		if h.herdrDown.Load() {
			return nil, herdrfake.Identities{}, errors.New("herdr unavailable")
		}
		return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
	})
	herdrfake.StartState(t, h.Herdr)

	// The herdr runner's wait N defaults to 5s, which every launch's slash
	// command prompt would sit out: the fake agent starts no turn. Read from
	// the config file, so give the server a home with a short one, unless the
	// test already set up its own.
	if os.Getenv("HOME") == processHome {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if err := os.MkdirAll(filepath.Join(home, ".config", "gx"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".config", "gx", "config.json"), []byte(`{"runner-wait":"100ms"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	h.cfg = server.Config{StateDir: h.StateDir, SessionsDir: h.SessionsDir, Build: "test-build", TicketStore: h.TicketStore, TCPAddr: tcpAddr}
	h.cfg.RecentlyFinished = config.DefaultRecentlyFinished
	// Every merge pauses on ralphloop's 3s finish debounce; the fake agents
	// settle at once, so a hundredth keeps the order without the wait.
	h.cfg.Sleep = func(d time.Duration) { time.Sleep(d / 100) }
	// Fake agents idle without a report and never leave idle after a nudge, so
	// the stall nudge is off unless a test sets its delay.
	h.cfg.StalledNudgeDelay = -1
	for _, o := range opts {
		o(&h.cfg)
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
	h.serve(t)
	return h
}

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
	srv, err := server.New(h.cfg)
	if err != nil {
		t.Fatal(err)
	}
	h.Server = srv
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
		if !srv.WaitSettled(10 * time.Second) {
			t.Logf("servertest: a run was still settling 10s after stop")
		}
	})

	h.Client = apiclient.New(server.SocketPath(h.StateDir))
}
