// Package servertest is seam A: an in-process server on a temp state dir,
// temp socket and temp ticket store, driven only through the API client. Start
// changes no process env, so harnesses may run in parallel subtests; the
// package's TestMain must call herdrfake.RunHelperProcess.
//
// # The runner rule
//
// A test that sets no runner runs on the PTY runner, built the production way
// over holder processes and a TUI fake; OnTurn runs a hook for each agent turn
// and Starts records each launch. Pick another runner only for a reason:
//
//   - PTY (the default): the test needs a real agent turn.
//   - runnerfake (WithRunner, WithRunnerFor): the test sets a runner state
//     directly, or checks only what the server passed to Start.
//   - herdr (WithHerdr): herdr is the subject, or the test is marked
//     UntilPtyAsDefault.
//
// The fake herdr executable is first in PATH of every server's Config.Env but
// fails any call unless the test asked for WithHerdr, so a stray herdr call
// fails the test and names the command.
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
	"github.com/elentok/gx/git"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/osnotify"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/sound"
	"github.com/elentok/gx/subscription"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/claudeenv"
	"github.com/elentok/gx/testutil/herdrfake"
	"github.com/elentok/gx/transcript"
)

// Harness is a running test server.
type Harness struct {
	Client *apiclient.Client
	Server *server.Server // for reading the run registry; drive everything else through Client
	Herdr  *herdrfake.State
	// Disks is the free-space source, unless an option set its own FreeSpace.
	Disks *FakeDisks
	// System is the CPU and memory source, unless an option set its own SystemHealth.
	System *FakeSystem
	// Processes is the process table source, unless an option set its own ProcessSource.
	Processes   *FakeProcesses
	StateDir    string
	SessionsDir string // the interactive sessions' state dirs
	// ConversationsDir holds the linked sessions' conversation records.
	ConversationsDir string
	TicketStore      string
	TCPAddr          string // empty unless started with StartWithTCP
	// Banners records the OS banners the server sent, unless an option set
	// its own Notifier. A harness never shows a real banner.
	Banners *Banners
	// Sounds records the session sounds played, unless an option set its own
	// Player.
	Sounds    *Sounds
	herdrDown atomic.Bool

	extraRunners []agentrunner.Runner
	// pty is set when the harness builds the PTY runner itself; turn is the
	// OnTurn hook it runs on a prompt.
	pty  bool
	turn atomic.Pointer[func(*Turn)]
	// tb reports a failure from a goroutine that is not the test's.
	tb testing.TB
	// failStart, stops and stopGate back FailNextStart, Stops and BlockStops.
	failStart atomic.Pointer[error]
	stops     atomic.Int32
	stopGate  atomic.Pointer[chan struct{}]

	startsMu sync.Mutex
	starts   []AgentStart
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
	// PauseReason writes status: paused with this pause_reason; Status is ignored.
	PauseReason string
	// Category is the ticket's category: frontmatter field ("bug", "idea"), omitted when empty.
	Category string
	// ClaimedAt, DoneAt and CancelledAt are RFC 3339 stamps, omitted when empty.
	ClaimedAt, DoneAt, CancelledAt string
}

// WriteTicketWith is WriteTicket with a type, a parent and several blockers. Unlike
// WriteTicket it may also run after Start: that is how an agent's turn splits its ticket.
func WriteTicketWith(t *testing.T, store, project, epic, id, slug string, opts TicketOpts) {
	t.Helper()
	if err := WriteTicketFile(store, project, epic, id, slug, opts); err != nil {
		t.Fatal(err)
	}
}

// WriteEpic creates an epic with no tickets, and the project it lives in.
// An empty epic name creates only the project.
func WriteEpic(store, project, epic string) error {
	projectDir := filepath.Join(store, project)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return err
	}
	// Keep an existing project.json: SetProjectRepo's repo must survive a mid-run write.
	if _, err := os.Stat(filepath.Join(projectDir, "project.json")); os.IsNotExist(err) {
		if err := os.WriteFile(filepath.Join(projectDir, "project.json"), []byte(`{"name":"`+project+`"}`), 0o644); err != nil {
			return err
		}
	}
	if epic == "" {
		return nil
	}
	epicDir := filepath.Join(projectDir, epic)
	if err := os.MkdirAll(filepath.Join(epicDir, "issues"), 0o755); err != nil {
		return err
	}
	path := filepath.Join(epicDir, "ticket.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte("---\nstatus: open\n---\n"), 0o644)
}

// WriteTicketFile is WriteTicketWith for a caller with no *testing.T.
func WriteTicketFile(store, project, epic, id, slug string, opts TicketOpts) error {
	if err := WriteEpic(store, project, epic); err != nil {
		return err
	}
	issues := filepath.Join(store, project, epic, "issues")
	if opts.Type == "" {
		opts.Type = "implement"
	}
	if opts.Status == "" {
		opts.Status = "open"
	}
	fm := "---\nid: \"" + id + "\"\nstatus: " + opts.Status + "\ntype: " + opts.Type + "\n"
	if opts.PauseReason != "" {
		fm = "---\nid: \"" + id + "\"\nstatus: paused\npause_reason: " + opts.PauseReason + "\ntype: " + opts.Type + "\n"
	}
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
	if opts.Category != "" {
		fm += "category: " + opts.Category + "\n"
	}
	for _, stamp := range [][2]string{{"claimed_at", opts.ClaimedAt}, {"done_at", opts.DoneAt}, {"cancelled_at", opts.CancelledAt}} {
		if stamp[1] != "" {
			fm += stamp[0] + ": " + stamp[1] + "\n"
		}
	}
	fm += "---\n\n# " + slug + "\n"
	return os.WriteFile(filepath.Join(issues, id+"-"+slug+".md"), []byte(fm), 0o644)
}

// Turn is one agent turn: the `agent prompt` the server sent to a launched agent.
type Turn struct {
	Address string // the ticket the skill prompt names
	Cwd     string // the worktree the agent's tab was opened in
	Text    string // the whole prompt, notes included

	control turnControl
	h       *Harness
	// sessionID is the agent's conversation id; empty on herdr.
	sessionID string
}

// turnControl is what a hook asked of the turn besides ending it. The harness
// applies it to the TUI fake once the prompt is in, since the hook runs before
// the prompt reaches the fake.
type turnControl struct {
	name string // the Turn method, for the herdr failure message
	// busy keeps the turn open past the prompt instead of ending it at once.
	busy bool
	// after is sent once the prompt is in.
	after *agentfake.Control
}

// Stall makes the turn start and never end: the run stays working. PTY only.
func (t *Turn) Stall() { t.control = turnControl{name: "Stall", busy: true} }

// Block makes the agent wait on a permission prompt for reason. PTY only.
func (t *Turn) Block(reason string) {
	t.control = turnControl{name: "Block", busy: true, after: &agentfake.Control{
		Action: agentfake.ActionBlock, Dialog: agentfake.BashPermissionDialog(reason),
	}}
}

// Exit makes the agent process exit with code, ending the session. PTY only.
func (t *Turn) Exit(code int) {
	t.control = turnControl{name: "Exit", busy: true, after: &agentfake.Control{Action: agentfake.ActionExit, Code: code}}
}

// Hold keeps the turn open until the returned release is called, which ends
// it. PTY only.
func (t *Turn) Hold() (release func()) {
	t.control = turnControl{name: "Hold", busy: true}
	if t.sessionID == "" {
		return func() {}
	}
	return func() {
		err := agentfake.SendTUIControl(t.h.Env("HOME"), t.sessionID, agentfake.Control{Action: agentfake.ActionFinish})
		if err != nil {
			t.h.tb.Errorf("servertest: release the held turn: %v", err)
		}
	}
}

// Transcript appends an assistant line saying text to the agent's conversation
// transcript, where the server reads the agent's last words. PTY only.
func (t *Turn) Transcript(text string) {
	if t.sessionID == "" {
		t.control.name = "Transcript"
		return
	}
	line, _ := json.Marshal(map[string]any{"type": "assistant", "message": map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	}})
	path := transcript.PathIn(t.h.Env("HOME"), t.Cwd, t.sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.h.tb.Errorf("servertest: write the transcript: %v", err)
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err == nil {
		_, err = f.Write(append(line, '\n'))
		err = errors.Join(err, f.Close())
	}
	if err != nil {
		t.h.tb.Errorf("servertest: write the transcript: %v", err)
	}
}

// ptyOnly reports whether the harness runs on PTY; on herdr it fails the test
// naming hook, as the turn controls do.
func (h *Harness) ptyOnly(hook string) bool {
	if !h.pty {
		h.tb.Errorf("servertest: Harness.%s is not supported on herdr; run this test on PTY (drop WithHerdr)", hook)
	}
	return h.pty
}

// FailNextStart makes the next agent start fail with err, so the launch fails
// the way a runner start error does. PTY only.
func (h *Harness) FailNextStart(err error) {
	if h.ptyOnly("FailNextStart") {
		h.failStart.Store(&err)
	}
}

// Stops is how many times the server stopped an agent. PTY only.
func (h *Harness) Stops() int {
	if !h.ptyOnly("Stops") {
		return 0
	}
	return int(h.stops.Load())
}

// BlockStops holds every agent stop, after it is counted, until release is
// called. The test's end releases it too. PTY only.
func (h *Harness) BlockStops() (release func()) {
	if !h.ptyOnly("BlockStops") {
		return func() {}
	}
	gate := make(chan struct{})
	h.stopGate.Store(&gate)
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	h.tb.Cleanup(release)
	return release
}

// AgentStart is one agent launch the server made.
type AgentStart struct {
	Label string // the agent's name, e.g. epic-a-iter-01
	Cwd   string // the worktree the agent started in
	Kind  string // the agent kind, e.g. claude
}

// Starts is every agent launch so far, in order, on PTY and on herdr (the
// latter once OnTurn answers its commands).
func (h *Harness) Starts() []AgentStart {
	h.startsMu.Lock()
	defer h.startsMu.Unlock()
	return slices.Clone(h.starts)
}

func (h *Harness) recordStart(s AgentStart) {
	h.startsMu.Lock()
	defer h.startsMu.Unlock()
	h.starts = append(h.starts, s)
}

// OnTurn runs agent for every turn (e.g. a commit in t.Cwd). The agent is idle
// again when the hook returns. On herdr it also answers the commands a launch
// makes, giving each tab its own pane and cwd.
func (h *Harness) OnTurn(agent func(t *Turn)) {
	if h.pty {
		h.turn.Store(&agent)
		return
	}
	// A test on its own runner never launches through herdr: nothing to answer.
	if h.Herdr == nil {
		return
	}
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
		cwd := cwds[pane]
		mu.Unlock()
		h.recordStart(AgentStart{Label: argv[2], Cwd: cwd, Kind: flagValue(argv, "--kind")})
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
		turn := &Turn{Address: addr, Cwd: cwd, Text: text, h: h}
		agent(turn)
		if name := turn.control.name; name != "" {
			h.tb.Errorf("servertest: Turn.%s is not supported on herdr; run this test on PTY (drop WithHerdr)", name)
		}
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

// WaitFor polls cond every few milliseconds until it holds, failing the test
// with "timed out waiting for <what>" after timeout.
func WaitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	if !Poll(timeout, cond) {
		t.Fatalf("timed out waiting for %s", what)
	}
}

// Poll is WaitFor without the failure: it reports whether cond held in time.
func Poll(timeout time.Duration, cond func() bool) bool {
	for deadline := time.Now().Add(timeout); !cond(); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			return false
		}
	}
	return true
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
	if err := WriteProjectRepo(store, project, repo, extra); err != nil {
		t.Fatal(err)
	}
}

// WriteProjectRepo is SetProjectRepo for a caller with no *testing.T; extra is
// raw JSON members appended to project.json, such as `,"auto-ff-merge":true`.
func WriteProjectRepo(store, project, repo, extra string) error {
	body := `{"name":"` + project + `","repo":"` + repo + `"` + extra + `}`
	return os.WriteFile(filepath.Join(store, project, "project.json"), []byte(body), 0o644)
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

// HerdrReason says why a test turns the fake herdr on.
type HerdrReason int

const (
	// HerdrIsTheSubject is for a test of herdr itself: the herdr runner, its
	// failures, or herdr-only behaviour.
	HerdrIsTheSubject HerdrReason = iota + 1
	// UntilPtyAsDefault is for a test that only needs herdr because herdr is
	// still the default runner. gx:pty-as-default deletes it.
	UntilPtyAsDefault
	// UntilPtyFixture is for a test that waits on the PTY turn fixture's turn
	// controls or leftover-run helper. It is deleted before this epic's verify.
	UntilPtyFixture
)

// runnerChoice is what the runner options recorded; startWith resolves it once
// every option has run, so the order of the options never matters.
type runnerChoice struct {
	runner    agentrunner.Runner
	runnerFor func(name, dir string) agentrunner.Runner
	herdr     bool
}

// choices maps a config under construction to its runnerChoice. Options are
// plain func(*server.Config), so this is how they reach the harness.
var choices sync.Map

func choiceOf(c *server.Config) *runnerChoice {
	v, _ := choices.Load(c)
	return v.(*runnerChoice)
}

// WithRunner makes r the configured runner and the runner for every pair.
func WithRunner(r agentrunner.Runner) func(*server.Config) {
	return func(c *server.Config) {
		ch := choiceOf(c)
		ch.runner = r
		ch.runnerFor = func(string, string) agentrunner.Runner { return r }
	}
}

// WithRunnerFor makes configured the configured runner and fn the runner for
// each pair.
func WithRunnerFor(configured agentrunner.Runner, fn func(name, dir string) agentrunner.Runner) func(*server.Config) {
	return func(c *server.Config) {
		ch := choiceOf(c)
		ch.runner = configured
		ch.runnerFor = fn
	}
}

// WithHerdr turns the fake herdr on. It makes herdr the configured runner only
// when no WithRunner or WithRunnerFor is given.
func WithHerdr(HerdrReason) func(*server.Config) {
	return func(c *server.Config) { choiceOf(c).herdr = true }
}

// resolveRunner applies the recorded choice: WithRunner / WithRunnerFor first,
// then herdr (server.New builds it when cfg.Runner is nil) for WithHerdr. It
// reports whether the harness builds the PTY runner itself, the default.
func resolveRunner(cfg *server.Config, ch *runnerChoice) (pty bool) {
	if ch.runner != nil {
		cfg.Runner = ch.runner
		cfg.RunnerFor = ch.runnerFor
		return false
	}
	return !ch.herdr
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
	// settle at once, so a hundredth keeps the order without the wait.
	h.cfg.Sleep = func(d time.Duration) { time.Sleep(d / 100) }
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
		herdr.Forbid()
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
		if h.pty {
			h.stopAgents()
		}
		if !srv.WaitSettled(10 * time.Second) {
			t.Logf("servertest: a run was still settling 10s after stop")
		}
	})

	h.Client = apiclient.New(server.SocketPath(h.StateDir))
}
