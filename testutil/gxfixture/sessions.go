package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/ralphloop"
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
	// steps are "/" commands sent once the session is idle, in order.
	steps []commandStep
	// rejected says the agent rejects the first prompt (see scenario.unknownSkill):
	// the session is settled once it raises the Unknown command Actionable.
	rejected bool
}

// commandStep is one "/" command a seeded session runs. output is the stdout
// the Claude fake writes for it (empty for a command with no output); nil
// sends no control, as for Codex, which writes no output. turn sends a command
// the fake treats as a chat turn (/compact) and waits for its reply.
type commandStep struct {
	prompt string
	output *string
	turn   bool
	// control is sent before the prompt. blocks says the command leaves the
	// session waiting at a dialog, so the step does not wait for it to idle.
	control *agentfake.Control
	blocks  bool
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
	before, codexBefore := f.controlFiles(), f.codexIDs()
	id, err := f.createSession(s.project, f.repoDir(sc, s.project), s.prompt, s.harness)
	if err != nil {
		return err
	}
	if s.rejected {
		return f.waitFor(id, "an Unknown command Actionable", func(si server.SessionInfo, _ server.SessionChatResponse) bool {
			return si.Actionable.GxRaised && strings.Contains(si.Actionable.Reason, "Unknown command:")
		})
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
	// The fake's control FIFO is named by Claude's session id, which gx's
	// session id is not; the one new file is this session's.
	sendControl := func(c agentfake.Control) error {
		if s.harness == "codex" {
			// The Codex fake's FIFO is named by its own id, which it logs with
			// its argv under the Codex home.
			fresh := slices.DeleteFunc(f.codexIDs(), func(name string) bool { return slices.Contains(codexBefore, name) })
			if len(fresh) != 1 {
				return fmt.Errorf("session %s: want one new codex fake, got %v", id, fresh)
			}
			return agentfake.SendProgramControl(fresh[0], c)
		}
		fresh := slices.DeleteFunc(f.controlFiles(), func(name string) bool { return slices.Contains(before, name) })
		if len(fresh) != 1 {
			return fmt.Errorf("session %s: want one new control file, got %v", id, fresh)
		}
		return agentfake.SendTUIControl(filepath.Join(f.root, "home"), strings.TrimSuffix(fresh[0], ".ctl"), c)
	}
	if control != nil {
		if err := sendControl(*control); err != nil {
			return err
		}
	}
	if s.thenPrompt != "" {
		if err := f.promptAgain(id, s.thenPrompt); err != nil {
			return err
		}
	}
	for _, st := range s.steps {
		if err := f.runCommandStep(id, st, sendControl); err != nil {
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
		if res, err := apiclient.New(server.SocketPath(f.stateD)).MarkSessionRead(context.Background(), id); err != nil || res.Refused {
			return fmt.Errorf("mark-read %s: %v (refused %v: %s)", id, err, res.Refused, res.Reason)
		}
	}
	return nil
}

// prompt sends text to the session. The server's status lags the fake's turn,
// so under load it can still report "working" after the reply landed; a
// session-not-idle refusal is retried until the session settles.
func (f *fixture) prompt(id, text string) error {
	deadline := time.Now().Add(sessionWaitTimeout)
	for {
		out, err := f.cli("agents", "prompt", id, text)
		if err == nil {
			return nil
		}
		if !strings.Contains(string(out), "session-not-idle") || time.Now().After(deadline) {
			return fmt.Errorf("prompt %s: %w\n%s", id, err, out)
		}
		time.Sleep(200 * time.Millisecond)
	}
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
	if err := f.prompt(id, text); err != nil {
		return err
	}
	return f.waitFor(id, "a reply to "+text, func(si server.SessionInfo, chat server.SessionChatResponse) bool {
		return si.Status.State == server.SessionIdle && agentMessages(chat) > agentMessages(before)
	})
}

// runCommandStep sends one "/" command and waits until the chat shows it (and
// its output entry, when the step has output).
func (f *fixture) runCommandStep(id string, st commandStep, sendControl func(agentfake.Control) error) error {
	if st.turn {
		return f.promptAgain(id, st.prompt)
	}
	if st.output != nil {
		if err := sendControl(agentfake.Control{Action: agentfake.ActionCommandOutput, Message: *st.output}); err != nil {
			return err
		}
		// The control travels on its own FIFO; give it time to land.
		time.Sleep(300 * time.Millisecond)
	}
	if st.control != nil {
		if err := sendControl(*st.control); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
	if st.blocks {
		return f.prompt(id, st.prompt)
	}
	count := func(chat server.SessionChatResponse, kind string) int {
		return len(slices.DeleteFunc(slices.Clone(chat.Messages), func(m server.SessionChatMessage) bool {
			return m.Kind != kind || (kind == transcript.ChatKindCommand && m.Text != st.prompt)
		}))
	}
	var before server.SessionChatResponse
	if out, err := f.cli("agents", "chat", id, "--json"); err == nil {
		_ = json.Unmarshal(out, &before)
	}
	if err := f.prompt(id, st.prompt); err != nil {
		return err
	}
	wantOutput := st.output != nil && strings.TrimSpace(*st.output) != ""
	return f.waitFor(id, "the "+st.prompt+" entries", func(si server.SessionInfo, chat server.SessionChatResponse) bool {
		return si.Status.State == server.SessionIdle &&
			count(chat, transcript.ChatKindCommand) > count(before, transcript.ChatKindCommand) &&
			(!wantOutput || count(chat, transcript.ChatKindOutput) > count(before, transcript.ChatKindOutput))
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

// codexIDs lists the ids of the Codex fakes started so far.
func (f *fixture) codexIDs() []string {
	paths, _ := filepath.Glob(agentfake.CodexArgsPath(filepath.Join(f.root, "home", ".codex"), "*"))
	ids := make([]string, len(paths))
	for i, p := range paths {
		ids[i] = strings.TrimSuffix(filepath.Base(p), ".args")
	}
	return ids
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
	// In-process calls: this loop runs for every seeded session, and a spawned
	// gx per poll dominated the scenario's start-up.
	cl := apiclient.New(server.SocketPath(f.stateD))
	for deadline := time.Now().Add(sessionWaitTimeout); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		ctx := context.Background()
		list, err := cl.Sessions(ctx)
		if err != nil {
			continue
		}
		i := slices.IndexFunc(list, func(si server.SessionInfo) bool { return si.ID == id })
		if i < 0 {
			continue
		}
		chat, _ := cl.SessionChat(ctx, id)
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
		ranOnClaude:  []string{addr("ran-on-claude/01")},
		queue: []string{
			addr("codex-running"), addr("claude-running"),
			addr("follows-default"), addr("pinned"), addr("ran-on-claude"),
		},
		queueAgents: map[string]string{addr("codex-running"): "codex", addr("claude-running"): "claude"},
	}
}

// seedRanOnClaude leaves the ticket open with an earlier Claude run: an
// iteration-started event in the epic's run log and a transcript for its session
// in the fixture home, which is all server.earlierRunHarness reads. A live
// worker would race the server's adoption of its question and hold a slot.
func (f *fixture) seedRanOnClaude(addr string) error {
	project, rest, _ := strings.Cut(addr, ":")
	epicName, id, _ := strings.Cut(rest, "/")
	const session = "00000000-0000-4000-8000-0000000000c1"
	ev := ralphloop.Event{
		Time:         time.Now(),
		Type:         string(events.IterationStarted),
		Ticket:       id,
		Agent:        ralphloop.AgentClaude,
		AgentSession: session,
		Cwd:          filepath.Join(f.root, "repo"),
	}
	if err := ralphloop.AppendEvent(filepath.Join(f.store, project), epicName, ev); err != nil {
		return fmt.Errorf("run log for %s: %w", addr, err)
	}
	dir := filepath.Join(f.root, "home", ".claude", "projects", "fixture")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, session+".jsonl"), []byte("{}\n"), 0o644)
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
