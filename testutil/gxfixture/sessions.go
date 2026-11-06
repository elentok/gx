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

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/agentfake/codex"
	"github.com/elentok/gx/transcript"
)

const sessionWaitTimeout = 20 * time.Second

// sessionSeeder holds the fixture's session-driving methods, so fixture stays under the
// per-type method limit. It reaches the fixture's fields and methods by embedding.
type sessionSeeder struct{ *fixture }

func sessionsOf(f *fixture) sessionSeeder { return sessionSeeder{f} }

// seedSessions creates the scenario's sessions one at a time, each settled
// before the next so the list keeps creation order.
func (f sessionSeeder) seedSessions(sc scenario) error {
	for _, s := range sc.Sessions {
		// A Codex session cannot start while codex is off PATH, which is the
		// state the missing-harness flows need.
		if s.Harness == "codex" && !f.codex {
			continue
		}
		if err := f.seedSession(sc, s); err != nil {
			return fmt.Errorf("session in %s: %w", s.Project, err)
		}
	}
	return nil
}

func (f sessionSeeder) seedSession(sc scenario, s sessionSpec) error {
	before, codexBefore := f.controlFiles(), f.codexIDs()
	id, err := f.createSession(s.Project, f.repoDir(sc, s.Project), s.Prompt, s.Harness)
	if err != nil {
		return err
	}
	if s.Rejected {
		return f.waitFor(id, "an Unknown command Actionable", func(si wire.SessionInfo, _ wire.SessionChatResponse) bool {
			return si.Actionable.GxRaised && strings.Contains(si.Actionable.Reason, "Unknown command:")
		})
	}
	slow := strings.Contains(s.Prompt, agentfake.ChatSlowMarker)
	// A Codex fake holds its turn until a finish control, so the seeder ends it.
	codexTurn := s.Harness == "codex" && s.Prompt != ""
	sendControl := f.controlSender(id, s.Harness, before, codexBefore)
	if s.Prompt != "" {
		role := transcript.ChatRoleAgent
		if slow || codexTurn {
			role = transcript.ChatRolePerson
		}
		if err := f.waitFor(id, "a "+role+" message", func(_ wire.SessionInfo, chat wire.SessionChatResponse) bool {
			return slices.ContainsFunc(chat.Messages, func(m wire.SessionChatMessage) bool { return m.Role == role })
		}); err != nil {
			return err
		}
	}
	if codexTurn {
		if err := sendControl(agentfake.Control{Action: agentfake.ActionFinish}); err != nil {
			return err
		}
	}
	state := wire.SessionIdle
	if slow {
		state = wire.SessionWorking
	}
	if err := f.waitFor(id, state, func(si wire.SessionInfo, _ wire.SessionChatResponse) bool { return si.Status.State == state }); err != nil {
		return err
	}
	if s.Link != "" {
		if err := f.linkSession(id, s.Link); err != nil {
			return err
		}
	}
	if s.Actionable != "" {
		cmd := exec.Command(f.gx, "agent", "actionable", s.Actionable)
		cmd.Env = append(slices.Clone(f.env), holder.AgentSessionEnv+"="+id)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("actionable %s: %w\n%s", id, err, out)
		}
	}
	control := s.Control
	if s.Crash {
		control = &agentfake.Control{Action: agentfake.ActionExit, Code: 1}
	}
	if control != nil {
		if err := sendControl(*control); err != nil {
			return err
		}
	}
	if s.ThenPrompt != "" {
		if err := f.promptAgain(id, s.ThenPrompt); err != nil {
			return err
		}
	}
	for _, st := range s.Steps {
		if err := f.runCommandStep(id, st, sendControl); err != nil {
			return err
		}
	}
	if s.Crash {
		if err := f.waitFor(id, wire.SessionExited, func(si wire.SessionInfo, _ wire.SessionChatResponse) bool {
			return si.Status.State == wire.SessionExited
		}); err != nil {
			return err
		}
	}
	if s.Failure != "" {
		if err := f.waitFor(id, "failure "+s.Failure, func(si wire.SessionInfo, _ wire.SessionChatResponse) bool {
			return si.Failure != nil && si.Failure.Kind == s.Failure
		}); err != nil {
			return err
		}
	}
	if s.Read {
		// The server raises Unread just after the state changes, so clearing it
		// first would leave it raised.
		if err := f.waitFor(id, "Unread", func(si wire.SessionInfo, _ wire.SessionChatResponse) bool { return si.Unread.Unread }); err != nil {
			return err
		}
		if res, err := apiclient.New(server.SocketPath(f.stateD)).Sessions.MarkRead(context.Background(), id); err != nil || res.Refused {
			return fmt.Errorf("mark-read %s: %v (refused %v: %s)", id, err, res.Refused, res.Reason)
		}
	}
	if s.Images {
		return f.seedImages(id, s.Harness)
	}
	return nil
}

// controlSender sends a control to the fake of the session just created. The
// fake's control FIFO is named by its own id, not gx's session id, so the one
// new file (Claude) or new args log under the Codex home (Codex) is this session's.
func (f sessionSeeder) controlSender(id, harness string, before, codexBefore []string) func(agentfake.Control) error {
	return func(c agentfake.Control) error {
		if harness == "codex" {
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
}

// prompt sends text to the session. The server's status lags the fake's turn,
// so under load it can still report "working" after the reply landed; a
// session-not-idle refusal is retried until the session settles.
func (f sessionSeeder) prompt(id, text string) error {
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
func (f sessionSeeder) promptAgain(id, text string) error {
	agentMessages := func(chat wire.SessionChatResponse) int {
		return len(slices.DeleteFunc(slices.Clone(chat.Messages), func(m wire.SessionChatMessage) bool {
			return m.Role != transcript.ChatRoleAgent
		}))
	}
	var before wire.SessionChatResponse
	if out, err := f.cli("agents", "chat", id, "--json"); err == nil {
		_ = json.Unmarshal(out, &before)
	}
	// The control travels on its own FIFO; give it time to land before the turn.
	time.Sleep(300 * time.Millisecond)
	if err := f.prompt(id, text); err != nil {
		return err
	}
	return f.waitFor(id, "a reply to "+text, func(si wire.SessionInfo, chat wire.SessionChatResponse) bool {
		return si.Status.State == wire.SessionIdle && agentMessages(chat) > agentMessages(before)
	})
}

// runCommandStep sends one "/" command and waits until the chat shows it (and
// its output entry, when the step has output).
func (f sessionSeeder) runCommandStep(id string, st commandStep, sendControl func(agentfake.Control) error) error {
	if st.Turn {
		return f.promptAgain(id, st.Prompt)
	}
	if st.Output != nil {
		if err := sendControl(agentfake.Control{Action: agentfake.ActionCommandOutput, Message: *st.Output}); err != nil {
			return err
		}
		// The control travels on its own FIFO; give it time to land.
		time.Sleep(300 * time.Millisecond)
	}
	if st.Control != nil {
		if err := sendControl(*st.Control); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
	if st.Blocks {
		return f.prompt(id, st.Prompt)
	}
	count := func(chat wire.SessionChatResponse, kind string) int {
		return len(slices.DeleteFunc(slices.Clone(chat.Messages), func(m wire.SessionChatMessage) bool {
			return m.Kind != kind || (kind == transcript.ChatKindCommand && m.Text != st.Prompt)
		}))
	}
	var before wire.SessionChatResponse
	if out, err := f.cli("agents", "chat", id, "--json"); err == nil {
		_ = json.Unmarshal(out, &before)
	}
	if err := f.prompt(id, st.Prompt); err != nil {
		return err
	}
	wantOutput := st.Output != nil && strings.TrimSpace(*st.Output) != ""
	return f.waitFor(id, "the "+st.Prompt+" entries", func(si wire.SessionInfo, chat wire.SessionChatResponse) bool {
		return si.Status.State == wire.SessionIdle &&
			count(chat, transcript.ChatKindCommand) > count(before, transcript.ChatKindCommand) &&
			(!wantOutput || count(chat, transcript.ChatKindOutput) > count(before, transcript.ChatKindOutput))
	})
}

// controlFiles lists the control FIFOs of the TUI fakes started so far.
func (f sessionSeeder) controlFiles() []string {
	paths, _ := filepath.Glob(agentfake.TUIControlPath(filepath.Join(f.root, "home"), "*"))
	names := make([]string, len(paths))
	for i, p := range paths {
		names[i] = filepath.Base(p)
	}
	return names
}

// codexIDs lists the ids of the Codex fakes started so far.
func (f sessionSeeder) codexIDs() []string {
	paths, _ := filepath.Glob(codex.CodexArgsPath(filepath.Join(f.root, "home", ".codex"), "*"))
	ids := make([]string, len(paths))
	for i, p := range paths {
		ids[i] = strings.TrimSuffix(filepath.Base(p), ".args")
	}
	return ids
}

// repoDir is the repo root the project's sessions run in.
func (f sessionSeeder) repoDir(sc scenario, project string) string {
	if slices.Contains(sc.OwnRepos, project) {
		return filepath.Join(f.root, project+"-repo")
	}
	return filepath.Join(f.root, "repo")
}

// waitFor polls the session until ok accepts its list row and chat.
func (f sessionSeeder) waitFor(id, what string, ok func(wire.SessionInfo, wire.SessionChatResponse) bool) error {
	// In-process calls: this loop runs for every seeded session, and a spawned
	// gx per poll dominated the scenario's start-up.
	cl := apiclient.New(server.SocketPath(f.stateD))
	for deadline := time.Now().Add(sessionWaitTimeout); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		ctx := context.Background()
		list, err := cl.Sessions.List(ctx)
		if err != nil {
			continue
		}
		i := slices.IndexFunc(list, func(si wire.SessionInfo) bool { return si.ID == id })
		if i < 0 {
			continue
		}
		chat, _ := cl.Sessions.Chat(ctx, id)
		if ok(list[i], chat) {
			return nil
		}
	}
	return fmt.Errorf("session %s never reached %s", id, what)
}

// createSession starts a session in dir and returns its id. The harness is
// Claude when empty.
func (f sessionSeeder) createSession(project, dir, prompt, harness string) (string, error) {
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

// seedRanOnClaude leaves the ticket open with an earlier Claude run: an
// iteration-started event in the epic's run log and a transcript for its session
// in the fixture home, which is all server.earlierRunHarness reads. A live
// worker would race the server's adoption of its question and hold a slot.
func (f sessionSeeder) seedRanOnClaude(addr string) error {
	project, rest, _ := strings.Cut(addr, ":")
	epicName, id, _ := strings.Cut(rest, "/")
	const session = "00000000-0000-4000-8000-0000000000c1"
	ev := runlog.Event{
		Time:         time.Now(),
		Type:         string(events.IterationStarted),
		Ticket:       id,
		Agent:        agent.AgentClaude,
		AgentSession: session,
		Cwd:          filepath.Join(f.root, "repo"),
	}
	if err := runlog.AppendEvent(filepath.Join(f.store, project), epicName, ev); err != nil {
		return fmt.Errorf("run log for %s: %w", addr, err)
	}
	dir := filepath.Join(f.root, "home", ".claude", "projects", "fixture")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, session+".jsonl"), []byte("{}\n"), 0o644)
}
