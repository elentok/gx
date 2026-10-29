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
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/transcript"
)

// fakeHarness is the non-Claude harness the gxfixture build of gx registers.
const fakeHarness = "fakecodex"

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
}

const sessionWaitTimeout = 20 * time.Second

// seedSessions creates the scenario's sessions one at a time, each settled
// before the next so the list keeps creation order.
func (f *fixture) seedSessions(sc scenario) error {
	for _, s := range sc.sessions {
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
	if s.crash {
		// The fake's control FIFO is named by Claude's session id, which gx's
		// session id is not; the one new file is this session's.
		fresh := slices.DeleteFunc(f.controlFiles(), func(name string) bool { return slices.Contains(before, name) })
		if len(fresh) != 1 {
			return fmt.Errorf("session %s: want one new control file, got %v", id, fresh)
		}
		claudeID := strings.TrimSuffix(fresh[0], ".ctl")
		if err := agentfake.SendTUIControl(filepath.Join(f.root, "home"), claudeID, agentfake.Control{Action: agentfake.ActionExit, Code: 1}); err != nil {
			return err
		}
		if err := f.waitFor(id, server.SessionExited, func(si server.SessionInfo, _ server.SessionChatResponse) bool {
			return si.Status.State == server.SessionExited
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

// agentsScenario is the Agents page fixture: eight sessions in three
// projects, one per row of the mock, and a worktree in demo.
func agentsScenario() scenario {
	const mapAddr = "design-epic"
	return scenario{
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
			{project: project, prompt: "hello", link: project + ":" + mapAddr, read: true},
			{project: project, harness: fakeHarness},
			{project: "other", prompt: "hello"},
			{project: "other", actionable: "needs a decision"},
			{project: "site", prompt: "hello", link: "site:" + mapAddr, read: true},
			{project: "site", crash: true, read: true},
		},
	}
}
