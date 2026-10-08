// Package servertest is seam A: an in-process server on a temp state dir,
// temp socket and temp ticket store, with the fake herdr executable first in
// PATH, driven only through the API client. Like herdrfake.Start it calls
// t.Setenv, so tests using it must not be parallel, and the package's
// TestMain must call herdrfake.RunHelperProcess.
package servertest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/herdrfake"
)

// Harness is a running test server.
type Harness struct {
	Client      *apiclient.Client
	Server      *server.Server // for reading the run registry; drive everything else through Client
	Herdr       *herdrfake.State
	StateDir    string
	TicketStore string
	TCPAddr     string // empty unless started with StartWithTCP
	herdrDown   atomic.Bool
	// Stop does what SIGTERM does to the real server: shut down, release the
	// lock, and return Serve's result. Safe to call more than once.
	Stop func() error

	cfg server.Config
}

// WriteTicket creates <store>/<project>/<epic>/issues/<id>-<slug>.md (and the
// project's project.json) with minimal valid frontmatter. Call it before Start.
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
	// Keep an existing project.json: SetProjectRepo's repo must survive a mid-run write.
	if _, err := os.Stat(filepath.Join(projectDir, "project.json")); os.IsNotExist(err) {
		if err := os.WriteFile(filepath.Join(projectDir, "project.json"), []byte(`{"name":"`+project+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if opts.Type == "" {
		opts.Type = "implement"
	}
	fm := "---\nid: \"" + id + "\"\nstatus: open\ntype: " + opts.Type + "\n"
	if opts.Parent != "" {
		fm += "parent: \"" + opts.Parent + "\"\n"
	}
	if len(opts.BlockedBy) > 0 {
		fm += "blocked_by: [\"" + strings.Join(opts.BlockedBy, "\", \"") + "\"]\n"
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
}

// RegisterLaunch answers the herdr commands a launch makes, giving each tab its
// own pane and cwd, and runs agent for every prompt (the agent's turn, e.g. a
// commit in p.Cwd). The agent is idle again when the prompt returns.
func (h *Harness) RegisterLaunch(agent func(p Prompt)) {
	var mu sync.Mutex
	cwds := map[string]string{} // pane -> cwd
	tabs := 0
	reply := func(pane string) map[string]any {
		return map[string]any{"agent": map[string]any{"pane_id": pane, "agent_status": "idle"}}
	}
	ok := func() (any, herdrfake.Identities, error) { return map[string]any{}, herdrfake.Identities{}, nil }
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
	h.Herdr.Register("tab", "close", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) { return ok() })
	h.Herdr.Register("agent", "start", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		return reply(flagValue(argv, "--pane")), herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "wait", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		return reply(argv[2]), herdrfake.Identities{}, nil
	})
	h.Herdr.Register("agent", "prompt", func(_ *herdrfake.State, argv []string) (any, herdrfake.Identities, error) {
		pane, text := argv[2], argv[3]
		mu.Lock()
		cwd := cwds[pane]
		mu.Unlock()
		_, addr, _ := strings.Cut(text, " ")
		agent(Prompt{Address: addr, Cwd: cwd})
		return reply(pane), herdrfake.Identities{}, nil
	})
}

func flagValue(argv []string, flag string) string {
	for i, a := range argv {
		if a == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
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
// after WriteTicket, before Start.
func SetProjectRepo(t *testing.T, store, project, repo string) {
	t.Helper()
	body := `{"name":"` + project + `","repo":"` + repo + `"}`
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

func startWith(t *testing.T, store, tcpAddr string, herdrDown bool, opts ...func(*server.Config)) *Harness {
	t.Helper()
	// Unix socket paths are capped near 100 bytes; t.TempDir() can exceed that.
	stateDir, err := os.MkdirTemp("", "gxs")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(stateDir) })

	h := &Harness{
		Herdr:       herdrfake.NewState(t),
		StateDir:    filepath.Join(stateDir, "state"),
		TicketStore: store,
	}
	h.herdrDown.Store(herdrDown)
	// The server's herdr probe is `workspace list`; answer it unless told to fail.
	h.Herdr.Register("workspace", "list", func(*herdrfake.State, []string) (any, herdrfake.Identities, error) {
		if h.herdrDown.Load() {
			return nil, herdrfake.Identities{}, errors.New("herdr unavailable")
		}
		return map[string]any{"workspaces": []any{}}, herdrfake.Identities{}, nil
	})
	herdrfake.StartState(t, h.Herdr)

	h.cfg = server.Config{StateDir: h.StateDir, Build: "test-build", TicketStore: h.TicketStore, TCPAddr: tcpAddr}
	// The product default is off; most tests exercise the landing.
	h.cfg.AutoMergeEpic = true
	for _, o := range opts {
		o(&h.cfg)
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
	})

	h.Client = apiclient.New(server.SocketPath(h.StateDir))
}
