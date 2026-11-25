package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/elentok/gx/ralphloop/agent"
	"github.com/elentok/gx/server/wire"

	"github.com/elentok/gx/apiclient"
	"github.com/elentok/gx/brief"
	"github.com/elentok/gx/events"
	"github.com/elentok/gx/nativerunner"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/server"
	"github.com/elentok/gx/server/sessions"
	"github.com/elentok/gx/testutil/agentfake"
	"github.com/elentok/gx/testutil/gxfixture/spec"
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
	if sc.SettleSessionsTogether {
		if err := f.seedSessionsTogether(sc); err != nil {
			return err
		}
		return f.seedDeadSessions(sc)
	}
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
	return f.seedDeadSessions(sc)
}

// seedSessionsTogether creates the sessions one at a time, so the list keeps
// creation order, then settles them all at once: a scenario of two dozen
// sessions waits out one session's start-up, not twenty-four.
func (f sessionSeeder) seedSessionsTogether(sc scenario) error {
	type created struct {
		spec sessionSpec
		id   string
	}
	var all []created
	for _, s := range sc.Sessions {
		if s.Harness == "codex" && !f.codex {
			continue
		}
		id, err := f.createSpecSession(sc, s)
		if err != nil {
			return fmt.Errorf("session in %s: %w", s.Project, err)
		}
		all = append(all, created{s, id})
	}
	errs := make([]error, len(all))
	var wg sync.WaitGroup
	for i, c := range all {
		wg.Go(func() {
			if err := f.settleSession(c.spec, c.id); err != nil {
				errs[i] = fmt.Errorf("session in %s: %w", c.spec.Project, err)
			}
		})
	}
	wg.Wait()
	return errors.Join(errs...)
}

// seedDeadSessions creates sc.DeadSessions sessions concurrently, then crashes
// every fake that is up, each through the control file of its own conversation.
func (f sessionSeeder) seedDeadSessions(sc scenario) error {
	if sc.DeadSessions == 0 {
		return nil
	}
	errs := make([]error, sc.DeadSessions)
	ids := make([]string, sc.DeadSessions)
	var wg sync.WaitGroup
	for i := range ids {
		wg.Go(func() { ids[i], errs[i] = f.createSession(project, f.repoDir(sc, project), "", "") })
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return err
	}
	for _, id := range ids {
		if err := f.waitFor(id, wire.SessionIdle, func(si wire.SessionInfo, _ wire.SessionChatResponse) bool { return si.Status.State == wire.SessionIdle }); err != nil {
			return err
		}
	}
	for _, id := range ids {
		if err := f.controlSender(id, "")(agentfake.Control{Action: agentfake.ActionExit, Code: 1}); err != nil {
			return err
		}
	}
	for _, id := range ids {
		if err := f.waitFor(id, wire.SessionExited, func(si wire.SessionInfo, _ wire.SessionChatResponse) bool {
			return si.Status.State == wire.SessionExited
		}); err != nil {
			return err
		}
	}
	return nil
}

func (f sessionSeeder) seedSession(sc scenario, s sessionSpec) error {
	id, err := f.createSpecSession(sc, s)
	if err != nil {
		return err
	}
	return f.settleSession(s, id)
}

func (f sessionSeeder) createSpecSession(sc scenario, s sessionSpec) (string, error) {
	dir := f.repoDir(sc, s.Project)
	if s.HostFilesSecond {
		dir = f.hostFiles.SecondWorktree
	}
	if s.Dir != "" {
		dir = strings.ReplaceAll(s.Dir, spec.RootToken, f.root)
	}
	return f.createSession(s.Project, dir, s.Prompt, s.Harness)
}

// settleSession drives a created session to the state its spec describes.
func (f sessionSeeder) settleSession(s sessionSpec, id string) error {
	if s.Rejected {
		return f.waitFor(id, "an Unknown command Actionable", func(si wire.SessionInfo, _ wire.SessionChatResponse) bool {
			return si.Actionable.GxRaised && strings.Contains(si.Actionable.Reason, "Unknown command:")
		})
	}
	slow := strings.Contains(s.Prompt, agentfake.ChatSlowMarker)
	// A Codex fake holds its turn until a finish control, so the seeder ends it.
	codexTurn := s.Harness == "codex" && s.Prompt != ""
	sendControl := f.controlSender(id, s.Harness)
	if s.Prompt != "" {
		role := transcript.ChatRoleAgent
		if slow || codexTurn {
			role = transcript.ChatRolePerson
		}
		if err := f.waitTurn(id, "a "+role+" message", !slow && !codexTurn, func(_ wire.SessionInfo, chat wire.SessionChatResponse) bool {
			return slices.ContainsFunc(chat.Messages, func(m wire.SessionChatMessage) bool { return m.Role == role })
		}); err != nil {
			return err
		}
	}
	if codexTurn {
		if s.Tokens > 0 {
			if err := sendControl(agentfake.Control{Action: agentfake.ActionUsage, Tokens: s.Tokens}); err != nil {
				return err
			}
		}
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
	if s.Tokens > 0 && s.Harness != "codex" {
		if err := sendControl(agentfake.Control{Action: agentfake.ActionUsage, Tokens: s.Tokens}); err != nil {
			return err
		}
		// The control travels on its own FIFO; give it time to land.
		time.Sleep(300 * time.Millisecond)
	}
	control := s.Control
	if s.Plan != "" {
		planPath := filepath.Join(f.root, "home", ".claude", "plans", "fixture-plan.md")
		if err := os.MkdirAll(filepath.Dir(planPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(planPath, []byte(s.Plan), 0o644); err != nil {
			return err
		}
		control = &agentfake.Control{Action: agentfake.ActionBlock,
			Dialog: agentfake.PlanDialog(planPath, "Yes, and auto-accept edits", "Yes, and manually approve edits", "Tell Claude what to change")}
	}
	if s.Crash {
		control = &agentfake.Control{Action: agentfake.ActionExit, Code: 1}
	}
	if s.Finish {
		control = &agentfake.Control{Action: agentfake.ActionExit}
	}
	if control != nil {
		if err := sendControl(*control); err != nil {
			return err
		}
		// The control leaves the session busy; the scenario starts once the
		// server shows it Working with the calls in flight.
		if control.Action == agentfake.ActionToolUse {
			if err := f.waitFor(id, "Working with a tool in flight", func(si wire.SessionInfo, chat wire.SessionChatResponse) bool {
				return si.Status.State == wire.SessionWorking && (chat.Current != nil || len(chat.Tasks) > 0)
			}); err != nil {
				return err
			}
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
	if s.Crash || s.Finish {
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
	if s.Workspace != "" {
		if res, err := apiclient.New(server.SocketPath(f.stateD)).Admin.MoveToWorkspace(context.Background(), s.Workspace, id); err != nil || len(res.Results) != 1 || res.Results[0].Status == wire.TargetRefused {
			return fmt.Errorf("move %s to %q: %v %+v", id, s.Workspace, err, res)
		}
	}
	if s.Key != "" || s.NoTranscript {
		if err := f.recordTranscript(id, s); err != nil {
			return err
		}
	}
	if len(s.Notices) > 0 {
		dir := filepath.Join(f.root, "data", "gx", "sessions", id)
		meta, err := sessions.ReadMeta(dir)
		if err != nil {
			return err
		}
		// The fake agent has no composer marker, so the first prompt already raised that notice.
		meta.Notices = slices.Clone(s.Notices)
		if err := sessions.WriteMeta(dir, meta); err != nil {
			return err
		}
	}
	if s.Images {
		return f.seedImages(id, s.Harness)
	}
	if s.Interrupted {
		return f.seedInterrupted(id, s.Harness)
	}
	if s.HostFilesChat {
		return f.seedHostFilesChat(id, s.Harness)
	}
	if s.HostFilesSecond {
		return f.seedHostFilesSecondChat(id, s.Harness)
	}
	if s.BgSeed != nil {
		return f.seedBgProgress(id, s)
	}
	return nil
}

// recordTranscript notes the Claude session's transcript path under its Key, and
// removes the file for a NoTranscript session.
func (f sessionSeeder) recordTranscript(id string, s sessionSpec) error {
	var path string
	var err error
	if s.Harness == "codex" {
		path, err = f.chatFilePath(id, s.Harness)
	} else {
		path, err = f.claudeTranscriptPath(id)
	}
	if err != nil {
		return err
	}
	if s.Key != "" {
		if f.keyedTranscripts == nil {
			f.keyedTranscripts = map[string]string{}
		}
		f.keyedTranscripts[s.Key] = path
	}
	if s.NoTranscript {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (f sessionSeeder) claudeTranscriptPath(id string) (string, error) {
	raw, err := f.cli("agents", "chat", id, "--json")
	if err != nil {
		return "", fmt.Errorf("chat %s: %w\n%s", id, err, raw)
	}
	var chat wire.SessionChatResponse
	if err := json.Unmarshal(raw, &chat); err != nil || chat.ConversationID == "" {
		return "", fmt.Errorf("session %s has no conversation id in %s", id, raw)
	}
	list, err := apiclient.New(server.SocketPath(f.stateD)).Sessions.List(context.Background())
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(list, func(x wire.SessionInfo) bool { return x.ID == id })
	if i < 0 {
		return "", fmt.Errorf("session %s is not listed", id)
	}
	return transcript.PathIn(filepath.Join(f.root, "home"), list[i].Dir, chat.ConversationID), nil
}

func (f sessionSeeder) sessionDir(id string) string {
	return filepath.Join(f.root, "data", "gx", "sessions", id)
}

// waitTurn waits until ready holds and, when endsTurn, the fake's own turn is
// over (the server's Idle can predate it). A timeout always names the fake's
// pid-file status and the agent reply count.
func (f sessionSeeder) waitTurn(id, what string, endsTurn bool, ready func(wire.SessionInfo, wire.SessionChatResponse) bool) error {
	var last wire.SessionChatResponse
	err := f.waitFor(id, what, func(si wire.SessionInfo, chat wire.SessionChatResponse) bool {
		last = chat
		return ready(si, chat) && (!endsTurn || agentfake.TurnEndedIn(f.env, f.sessionDir(id), agentReplies(chat)) == nil)
	})
	if err != nil {
		return agentfake.TurnTimeoutIn(err, f.env, f.sessionDir(id), agentReplies(last))
	}
	return nil
}

func agentReplies(chat wire.SessionChatResponse) int {
	return len(slices.DeleteFunc(slices.Clone(chat.Messages), func(m wire.SessionChatMessage) bool { return m.Role != transcript.ChatRoleAgent }))
}

// controlSender sends a control to the fake of session id. The fake's control
// FIFO is named by its conversation id, not gx's session id, so each call reads
// the id from the session's meta.
func (f sessionSeeder) controlSender(id, harness string) func(agentfake.Control) error {
	return func(c agentfake.Control) error {
		conv, err := f.conversationID(id)
		if err != nil {
			return err
		}
		if harness == "codex" {
			return agentfake.SendProgramControl(conv, c)
		}
		return agentfake.SendTUIControl(filepath.Join(f.root, "home"), conv, c)
	}
}

// conversationID reads the session's conversation id from its meta, which is
// also the fake's id. The id can change after the first report, so each call
// reads it again, and an empty one is waited for.
func (f sessionSeeder) conversationID(id string) (string, error) {
	dir := f.sessionDir(id)
	for deadline := time.Now().Add(sessionWaitTimeout); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if m, err := sessions.ReadMeta(dir); err == nil && m.ConversationID != "" {
			return m.ConversationID, nil
		}
	}
	return "", fmt.Errorf("session %s: no conversation id in its meta", id)
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
	var before wire.SessionChatResponse
	if out, err := f.cli("agents", "chat", id, "--json"); err == nil {
		_ = json.Unmarshal(out, &before)
	}
	// The control travels on its own FIFO; give it time to land before the turn.
	time.Sleep(300 * time.Millisecond)
	if err := f.prompt(id, text); err != nil {
		return err
	}
	return f.waitTurn(id, "a reply to "+text, true, func(si wire.SessionInfo, chat wire.SessionChatResponse) bool {
		return si.Status.State == wire.SessionIdle && agentReplies(chat) > agentReplies(before)
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

// repoDir is the repo root the project's sessions run in.
func (f sessionSeeder) repoDir(sc scenario, project string) string {
	if f.hostFiles != nil {
		return f.hostFiles.Repo
	}
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
	return fmt.Errorf("session %s never reached %s%s", id, what, startFailureState(f.root, id))
}

const holderErrTailLines = 20

// startFailureState names what the fixture saw of the agent directories (the
// ones whose path contains id, or every one when id is empty) when a session
// did not start: the holder.err tail, the files, the holder pid and whether
// it lives, the saved exit code, and the machine's process count and load.
func startFailureState(root, id string) string {
	var b strings.Builder
	dirs := 0
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != nativerunner.MetaFile || !strings.Contains(path, id) {
			return nil
		}
		dirs++
		b.WriteString(agentDirState(filepath.Dir(path)))
		return nil
	})
	sample := brief.SystemSample()
	fmt.Fprintf(&b, "\nagent dirs found: %d; user processes: %d; load: %.2f", dirs, sample.Procs, sample.Load)
	return b.String()
}

func agentDirState(dir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nagent dir %s:", dir)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if info, err := e.Info(); err == nil {
			fmt.Fprintf(&b, "\n  %s %d bytes %s", e.Name(), info.Size(), info.ModTime().Format("15:04:05.000"))
		}
	}
	var meta nativerunner.Meta
	if raw, err := os.ReadFile(filepath.Join(dir, nativerunner.MetaFile)); err == nil {
		_ = json.Unmarshal(raw, &meta)
	}
	if meta.HolderPID > 0 {
		fmt.Fprintf(&b, "\n  holder pid %d alive: %t", meta.HolderPID, syscall.Kill(meta.HolderPID, 0) == nil)
	} else {
		b.WriteString("\n  holder pid: none recorded in meta.json")
	}
	if code, err := os.ReadFile(filepath.Join(dir, holder.ExitCodeFile)); err == nil {
		fmt.Fprintf(&b, "\n  saved exit code: %s", strings.TrimSpace(string(code)))
	} else {
		b.WriteString("\n  saved exit code: none")
	}
	if tail := holderErrTail(dir, "", holderErrTailLines); tail != "" {
		fmt.Fprintf(&b, "\n  holder.err (last lines):\n%s", tail)
	} else {
		b.WriteString("\n  holder.err: empty or missing")
	}
	return b.String()
}

// holderErrTail returns the last n lines of the holder.err in the agent dir
// whose path contains the session id, or "" when there is none or it is empty.
func holderErrTail(root, id string, n int) string {
	var tail string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.IsDir() || entry.Name() != holder.HolderErrFile || !strings.Contains(path, id) {
			return nil
		}
		lines := strings.Split(holder.HolderErrText(filepath.Dir(path)), "\n")
		tail = strings.Join(lines[max(0, len(lines)-n):], "\n")
		return filepath.SkipAll
	})
	return strings.TrimSpace(tail)
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
		return "", fmt.Errorf("create session in %s: %w\n%s%s", project, err, out, startFailureState(f.root, ""))
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
