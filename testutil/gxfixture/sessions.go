package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/servertest"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// sessionSpec is one interactive session a scenario creates once the server is
// up, in order. A prompt makes a chat turn (the scenario turns the fake's chat
// knob on); with the agentfake.ChatSlowMarker the turn never ends.
type sessionSpec struct {
	project string
	// harness is empty for Claude.
	harness string
	prompt  string
	// link is the map address the finished session is marked a design session of.
	link string
	// read clears the Unread the finished turn (or the crash) raised.
	read bool
	// actionable is the reason the session raises with `gx agent actionable`.
	actionable string
	// crash makes the fake exit with code 1 once the session is idle.
	crash bool
	// control is sent to the fake once the session settled; failure is the
	// Failure.Kind the session must then show.
	control *agentfake.Control
	failure string
	// thenPrompt is one more chat turn sent after the control, so a reply
	// carries what the control changed.
	thenPrompt string
}

const sessionWaitTimeout = 20 * time.Second

// seedSessions creates the scenario's sessions one at a time, each settled
// before the next so the list keeps creation order.
func (f *fixture) seedSessions(sc scenario) error {
	for _, s := range sc.sessions {
		// A Codex session cannot start while codex is off PATH, which is the
		// state the missing-harness flows need.
		if s.harness == "codex" && !f.codex {
			continue
		}
		if err := f.seedSession(sc, s); err != nil {
			return fmt.Errorf("session in %s: %w", s.project, err)
		}
	}
	return nil
}

func (f *fixture) seedSession(sc scenario, s sessionSpec) error {
	before := f.controlFiles()
	id, err := f.createSession(s.project, f.repoDir(sc, s.project), s.prompt, s.harness)
	if err != nil {
		return err
	}
	slow := strings.Contains(s.prompt, agentfake.ChatSlowMarker)
	if s.prompt != "" {
		role := transcript.ChatRoleAgent
		if slow {
			role = transcript.ChatRolePerson
		}
		if err := f.waitFor(id, "a "+role+" message", func(_ server.SessionInfo, chat server.SessionChatResponse) bool {
			return slices.ContainsFunc(chat.Messages, func(m server.SessionChatMessage) bool { return m.Role == role })
		}); err != nil {
			return err
		}
	}
	state := server.SessionIdle
	if slow {
		state = server.SessionWorking
	}
	if err := f.waitFor(id, state, func(si server.SessionInfo, _ server.SessionChatResponse) bool { return si.Status.State == state }); err != nil {
		return err
	}
	if s.link != "" {
		if err := f.linkSession(id, s.link); err != nil {
			return err
		}
	}
	if s.actionable != "" {
		cmd := exec.Command(f.gx, "agent", "actionable", s.actionable)
		cmd.Env = append(slices.Clone(f.env), nativerunner.AgentSessionEnv+"="+id)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("actionable %s: %w\n%s", id, err, out)
		}
	}
	control := s.control
	if s.crash {
		control = &agentfake.Control{Action: agentfake.ActionExit, Code: 1}
	}
	if control != nil {
		// The fake's control FIFO is named by Claude's session id, which gx's
		// session id is not; the one new file is this session's.
		fresh := slices.DeleteFunc(f.controlFiles(), func(name string) bool { return slices.Contains(before, name) })
		if len(fresh) != 1 {
			return fmt.Errorf("session %s: want one new control file, got %v", id, fresh)
		}
		claudeID := strings.TrimSuffix(fresh[0], ".ctl")
		if err := agentfake.SendTUIControl(filepath.Join(f.root, "home"), claudeID, *control); err != nil {
			return err
		}
	}
	if s.thenPrompt != "" {
		if err := f.promptAgain(id, s.thenPrompt); err != nil {
			return err
		}
	}
	if s.crash {
		if err := f.waitFor(id, server.SessionExited, func(si server.SessionInfo, _ server.SessionChatResponse) bool {
			return si.Status.State == server.SessionExited
		}); err != nil {
			return err
		}
	}
	if s.failure != "" {
		if err := f.waitFor(id, "failure "+s.failure, func(si server.SessionInfo, _ server.SessionChatResponse) bool {
			return si.Failure != nil && si.Failure.Kind == s.failure
		}); err != nil {
			return err
		}
	}
	if s.read {
		// The server raises Unread just after the state changes, so clearing it
		// first would leave it raised.
		if err := f.waitFor(id, "Unread", func(si server.SessionInfo, _ server.SessionChatResponse) bool { return si.Unread.Unread }); err != nil {
			return err
		}
		if out, err := f.cli("agents", "mark-read", id); err != nil {
			return fmt.Errorf("mark-read %s: %w\n%s", id, err, out)
		}
	}
	return nil
}

// promptAgain sends text as another chat turn and waits for its reply.
func (f *fixture) promptAgain(id, text string) error {
	agentMessages := func(chat server.SessionChatResponse) int {
		return len(slices.DeleteFunc(slices.Clone(chat.Messages), func(m server.SessionChatMessage) bool {
			return m.Role != transcript.ChatRoleAgent
		}))
	}
	var before server.SessionChatResponse
	if out, err := f.cli("agents", "chat", id, "--json"); err == nil {
		_ = json.Unmarshal(out, &before)
	}
	// The control travels on its own FIFO; give it time to land before the turn.
	time.Sleep(300 * time.Millisecond)
	if out, err := f.cli("agents", "prompt", id, text); err != nil {
		return fmt.Errorf("prompt %s: %w\n%s", id, err, out)
	}
	return f.waitFor(id, "a reply to "+text, func(si server.SessionInfo, chat server.SessionChatResponse) bool {
		return si.Status.State == server.SessionIdle && agentMessages(chat) > agentMessages(before)
	})
}

// controlFiles lists the control FIFOs of the TUI fakes started so far.
func (f *fixture) controlFiles() []string {
	paths, _ := filepath.Glob(agentfake.TUIControlPath(filepath.Join(f.root, "home"), "*"))
	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = filepath.Base(p)
	}
	return names
}

// repoDir is the repo root the project's sessions run in.
func (f *fixture) repoDir(sc scenario, project string) string {
	if slices.Contains(sc.ownRepos, project) {
		return filepath.Join(f.root, project+"-repo")
	}
	return filepath.Join(f.root, "repo")
}

// waitFor polls the session until ok accepts its list row and chat.
func (f *fixture) waitFor(id, what string, ok func(server.SessionInfo, server.SessionChatResponse) bool) error {
	for deadline := time.Now().Add(sessionWaitTimeout); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		var list []server.SessionInfo
		out, err := f.cli("agents", "list", "--json")
		if err != nil || json.Unmarshal(out, &list) != nil {
			continue
		}
		i := slices.IndexFunc(list, func(si server.SessionInfo) bool { return si.ID == id })
		if i < 0 {
			continue
		}
		var chat server.SessionChatResponse
		if out, err := f.cli("agents", "chat", id, "--json"); err == nil {
			_ = json.Unmarshal(out, &chat)
		}
		if ok(list[i], chat) {
			return nil
		}
	}
	return fmt.Errorf("session %s never reached %s", id, what)
}

// createSession starts a session in dir and returns its id. The harness is
// Claude when empty.
func (f *fixture) createSession(project, dir, prompt, harness string) (string, error) {
	args := []string{"agents", "create", "--project", project, "--dir", dir, "--json"}
	if prompt != "" {
		args = append(args, "--prompt", prompt)
	}
	if harness != "" {
		args = append(args, "--harness", harness)
	}
	out, err := f.cli(args...)
	if err != nil {
		return "", fmt.Errorf("create session in %s: %w\n%s", project, err, out)
	}
	var res struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := json.Unmarshal(out, &res); err != nil || res.Session.ID == "" {
		return "", fmt.Errorf("create session in %s: no session id in %s", project, out)
	}
	return res.Session.ID, nil
}

// agentsFailuresScenario is one Claude session per failure the Agents page
// shows above the composer. It is not part of agentsScenario because the short
// stall threshold would stall that scenario's Working session too.
func agentsFailuresScenario() scenario {
	return scenario{
		chat:           true,
		stallThreshold: "3s",
		sessions: []sessionSpec{
			{project: project, prompt: agentfake.ChatSlowMarker + " a stuck turn", failure: server.FailureKindStalled},
			{project: project, prompt: "hello", read: true, failure: server.FailureKindAuth,
				control: &agentfake.Control{Action: agentfake.ActionAPIError, Error: "authentication_error", Status: 401, Message: "Invalid API key"}},
			{project: project, prompt: "hello", read: true, failure: server.FailureKindRateLimit,
				control: &agentfake.Control{Action: agentfake.ActionAPIError, Error: "rate_limit", Status: 429, Message: "You've hit your limit · resets at 3pm"}},
			{project: project, prompt: "hello", read: true, failure: server.FailureKindBlocked,
				control: &agentfake.Control{Action: agentfake.ActionBlock}},
		},
	}
}

// queueAgentsScenario lists one PTY Queue agent and one Design agent. The
// ticket's worker idles in a long turn until the hold file is removed.
func queueAgentsScenario() scenario {
	const mapAddr = "design-epic"
	return scenario{
		epics: []epic{
			{project, "queue-epic", []ticket{{"01", "held", servertest.TicketOpts{}}}},
			{project, mapAddr, nil},
		},
		designSteps: map[string]string{project + ":" + mapAddr: string(tickets.StepResolveTickets)},
		queue:       []string{project + ":queue-epic"},
		hold:        true,
		chat:        true,
		sessions: []sessionSpec{
			{project: project, prompt: "hello", link: project + ":" + mapAddr, read: true},
		},
	}
}

// defaultHarnessScenario is the Settings and Agents fixture for the Default
// harness. The global default is Claude. gx overrides to Codex; demo and
// dotfiles inherit. demo's cap of one agent keeps the second epic's ticket
// waiting behind the held worker of the first.
func defaultHarnessScenario() scenario {
	const codexOverride = `,"agents":{"default-harness":"codex"}`
	return scenario{
		epics: []epic{
			{project, "running-epic", []ticket{{"01", "running", servertest.TicketOpts{}}}},
			{project, "waiting-epic", []ticket{{"01", "waiting", servertest.TicketOpts{}}}},
			{"dotfiles", "dotfiles-epic", nil},
			{"gx", "gx-epic", nil},
		},
		projectExtra: map[string]string{project: `,"max-agents":1`, "gx": codexOverride},
		ownRepos:     []string{"dotfiles", "gx"},
		queue:        []string{project + ":running-epic", project + ":waiting-epic"},
		hold:         true,
		chat:         true,
		sessions: []sessionSpec{
			{project: project, prompt: "hello", read: true},
			{project: "gx", harness: "codex"},
			{project: "dotfiles", prompt: "hello", read: true,
				control:    &agentfake.Control{Action: agentfake.ActionModel, Model: switchedModel},
				thenPrompt: "and again"},
		},
	}
}

// codexQueueScenario fills demo's two agent slots with held workers (one Codex,
// one Claude) and leaves three tickets waiting. A queue item's agent covers its
// whole root, so each ticket has an epic of its own.
func codexQueueScenario() scenario {
	addr := func(name string) string { return project + ":" + name }
	one := func(opts servertest.TicketOpts) []ticket { return []ticket{{"01", "work", opts}} }
	return scenario{
		codex: true,
		epics: []epic{
			{project, "codex-running", one(servertest.TicketOpts{})},
			{project, "claude-running", one(servertest.TicketOpts{})},
			{project, "follows-default", one(servertest.TicketOpts{})},
			{project, "pinned", one(servertest.TicketOpts{Agent: "codex"})},
			{project, "ran-on-claude", one(servertest.TicketOpts{})},
		},
		projectExtra: map[string]string{project: `,"max-agents":2`},
		hold:         true,
		behaviour:    map[string]string{addr("ran-on-claude/01"): agentfake.BehaviourAsk},
		ranOnClaude:  []string{addr("ran-on-claude/01")},
		config:       []string{"execution-queue.waiting-slot-grace=1s"},
		transcripts:  true,
		queue: []string{
			addr("codex-running"), addr("claude-running"),
			addr("follows-default"), addr("pinned"), addr("ran-on-claude"),
		},
		queueAgents: map[string]string{addr("codex-running"): "codex", addr("claude-running"): "claude"},
	}
}

// seedRanOnClaude gives the ticket a real Claude run: its epic is queued with no
// agent, the fake worker asks, and once the ticket waits for the answer it is
// taken off the queue and sent back to open. The run log keeps the session. The
// asked run keeps its slot until the waiting-slot grace ends, which the scenario
// sets short.
func (f *fixture) seedRanOnClaude(addr string) error {
	epicAddr, _, _ := strings.Cut(addr, "/")
	if out, err := f.cli("server", "queue", "add", epicAddr); err != nil {
		return fmt.Errorf("queue %s: %w\n%s", epicAddr, err, out)
	}
	deadline := time.Now().Add(sessionWaitTimeout * 3)
	for {
		out, _ := f.cli("server", "tickets", "detail", addr, "--json")
		if strings.Contains(string(out), `"needs-answer"`) {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s never asked its question:\n%s", addr, out)
		}
		time.Sleep(500 * time.Millisecond)
	}
	// The queue loop adds the epic again. An epic that already left the queue
	// makes remove fail, which is as good.
	_, _ = f.cli("server", "queue", "remove", epicAddr)
	// start-over finds the ticket's worktree from the working directory.
	startOver := exec.Command(f.gx, "server", "tickets", "start-over", addr, "--reason", "fixture: seed a waiting ticket that ran on Claude")
	startOver.Env = f.env
	startOver.Dir = filepath.Join(f.root, "repo")
	if out, err := startOver.CombinedOutput(); err != nil {
		return fmt.Errorf("start over %s: %w\n%s", addr, err, out)
	}
	return nil
}

// codexDesignScenario opens a design session on a map while the global Default
// harness is Codex, so the launch resolves to Codex.
func codexDesignScenario() scenario {
	const mapAddr = "design-epic"
	return scenario{
		codex:        true,
		epics:        []epic{{project, mapAddr, nil}},
		designSteps:  map[string]string{project + ":" + mapAddr: string(tickets.StepResolveTickets)},
		projectExtra: map[string]string{project: `,"agents":{"default-harness":"codex"}`},
		design:       []string{agentfake.DesignTurnRound, agentfake.DesignTurnRunEnd},
		openDesigns:  []string{project + ":" + mapAddr},
	}
}

// switchedModel is the model the switched session's later replies report; it
// differs from the model its tier launched with.
const switchedModel = "claude-haiku-5-5"

// agentsScenario is the Agents page fixture: eight sessions in three
// projects, one per row of the mock, and a worktree in demo.
func agentsScenario() scenario {
	const mapAddr = "design-epic"
	return scenario{
		codex:    true,
		projects: []string{"other"},
		epics: []epic{
			{project, mapAddr, nil},
			{"site", mapAddr, nil},
		},
		designSteps: map[string]string{
			project + ":" + mapAddr: string(tickets.StepResolveTickets),
			"site:" + mapAddr:       string(tickets.StepResolveTickets),
		},
		ownRepos: []string{"other", "site"},
		chat:     true,
		gitSetup: func(f *fixture, repo string) error {
			return f.git(repo, "worktree", "add", "-b", "feature", filepath.Join(repo, ".worktrees", "feature"))
		},
		sessions: []sessionSpec{
			{project: project, prompt: "hello", read: true},
			{project: project, prompt: agentfake.ChatSlowMarker + " a long turn"},
			{project: project, prompt: "hello", link: project + ":" + mapAddr, read: true, actionable: "needs a decision"},
			{project: project, harness: "codex"},
			{project: "other", prompt: "hello"},
			{project: "other", actionable: "needs a decision"},
			{project: "site", prompt: "hello", link: "site:" + mapAddr, read: true},
			{project: "site", prompt: "hello", crash: true, read: true},
		},
	}
}
