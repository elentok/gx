package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/testutil/agentfake/triageplay"
)

// submit is Enter. A stalled fake leaves the text unsent in the input box.
func (t *tui) submit() error {
	if t.stalled {
		t.draw()
		return nil
	}
	if t.pid.Status == StatusIdle && t.armUndecided() {
		t.draw()
		return nil
	}
	if t.swallow > 0 {
		t.swallow--
		t.draw()
		return nil
	}
	text := t.expandPastes(t.typed.String())
	if t.pid.Status == StatusIdle {
		if cleaned, n := stripHidden(text); n > 0 {
			if strings.HasPrefix(text, "/") || strings.HasPrefix(text, "!") {
				// The first Enter only cleans the box; the next one sends it.
				t.typed.Reset()
				t.typed.WriteString(cleaned)
				t.pastes = nil
				t.notice, t.invisibleShown, t.noticeRows = invisibleNotice(n), true, 1
				t.draw()
				return nil
			}
			text = cleaned
		}
	}
	if t.invisibleShown {
		t.notice, t.invisibleShown, t.noticeRows = "", false, 0
	}
	t.typed.Reset()
	t.pastes = nil
	images := t.images
	t.images = nil
	if len(images) > 0 {
		text = strings.TrimSpace(text)
	}
	if text == "" && t.pid.Status != StatusWaiting {
		// No turn starts, so nothing else would redraw the now empty box.
		t.draw()
		return nil
	}
	if t.pid.Status == StatusIdle {
		// A new prompt ends the screen of the last local command.
		t.landed = nil
	}
	switch t.pid.Status {
	case StatusIdle:
		if handled, err := t.skillPromptKnobs(text); handled || err != nil {
			return err
		}
		if t.failsFirstMessage() && text != "" && !strings.HasPrefix(text, "/") {
			return t.modelErrorTurn(text)
		}
		if w := t.resume; w != nil && text != "" {
			t.resume = nil
			if t.limitResetsAt != 0 {
				// Resuming after a rate limit means the window rolled over.
				t.limitResetsAt = 0
				t.limitLifted = true
			}
			t.prompt = text
			go t.work(w.Cfg, w.Address)
			return t.setStatus(StatusBusy, "")
		}
		kind, address, cfg := t.classify(text)
		switch kind {
		case promptDesign:
			return t.designTurn(text)
		case promptStartDesign:
			return t.startDesignTurn(text)
		case promptTriage:
			return t.triageTurn(text)
		case promptWorker:
			t.prompt = text
			if t.opts.classifierOutage > 0 {
				t.outageWork = &PendingWork{Cfg: cfg, Address: address}
			}
			if cfg.Transcript {
				path, err := t.transcriptPath()
				if err != nil {
					return err
				}
				if err := AppendMessage(path, "user", text); err != nil {
					return err
				}
			}
			switch cfg.Behaviour[address] {
			case BehaviourPermission:
				t.dialog = BashPermissionDialog(PermissionCommand)
				return t.setStatus(StatusWaiting, "permission prompt")
			case BehaviourQuestionMenu:
				t.resume = &PendingWork{Cfg: cfg, Address: address}
				t.dialog = QuestionMenuDialog()
				return t.setStatus(StatusWaiting, "question menu")
			case BehaviourRateLimit:
				t.resume = &PendingWork{Cfg: cfg, Address: address}
				return t.rateLimit(cfg)
			case BehaviourStall, BehaviourStallNoRecord:
				t.resume = &PendingWork{Cfg: cfg, Address: address}
				dropRecord := cfg.Behaviour[address] == BehaviourStallNoRecord
				go func() {
					// Long enough for a poll to see the turn start, and the launch
					// proof to read the user record before it goes.
					time.Sleep(stallTurnLength)
					if dropRecord {
						if path, err := t.transcriptPath(); err == nil {
							_ = os.Truncate(path, 0)
						}
					}
					t.runStatusLine()
					_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
				}()
				return t.setStatus(StatusBusy, "")
			case BehaviourWriteAsk:
				// A kept agent gets the answer as its next prompt and finishes on it;
				// a relaunch sees the file and finishes at once.
				if !FileExists(UncommittedFile) {
					t.resume = &PendingWork{Cfg: cfg, Address: address}
				}
			}
			go t.work(cfg, address)
			return t.setStatus(StatusBusy, "")
		case promptSlash:
			return t.slashCommand(text)
		case promptPermission:
			t.prompt = text
			t.dialog = BashPermissionDialog(strings.TrimPrefix(text, PermissionPromptPrefix))
			return t.setStatus(StatusWaiting, "permission prompt")
		}
		t.prompt = text
		if w := t.outageWork; w != nil {
			// A re-sent "continue" is a turn on the ticket again.
			go t.work(w.Cfg, w.Address)
			return t.setStatus(StatusBusy, "")
		}
		if t.fast {
			return t.fastTurn()
		}
		if isCompact(text) {
			if err := t.startCompact(text); err != nil {
				return err
			}
		}
		if cfg, err := LoadWorkerConfig(t.pid.Cwd); err == nil && cfg.Chat {
			return t.chatTurn(text, images)
		}
		return t.setStatus(StatusBusy, "")
	case StatusWaiting:
		if t.ignoreAllow {
			return nil
		}
		// Enter picks the dialog's first option, "Yes".
		return t.setStatus(StatusBusy, "")
	}
	return nil
}

// failsFirstMessage reports whether the launch's model answers its first
// message with an API error.
func (t *tui) failsFirstMessage() bool {
	b := t.opts.modelRow.Behaviour
	return !t.modelErrored && (b == ModelNotFound || b == ModelError)
}

// modelErrorTurn answers the first message with the assistant line claude
// writes for a failed API call (model <synthetic>), then goes idle again.
func (t *tui) modelErrorTurn(text string) error {
	t.modelErrored = true
	code := cmp.Or(t.opts.modelRow.ErrorCode, "unknown")
	if t.opts.modelRow.Behaviour == ModelNotFound {
		code = "model_not_found"
	}
	path, err := t.transcriptPath()
	if err != nil {
		return err
	}
	if err := AppendMessage(path, "user", text); err != nil {
		return err
	}
	if err := t.setStatus(StatusBusy, ""); err != nil {
		return err
	}
	if err := AppendLine(path, map[string]any{
		"type":              "assistant",
		"timestamp":         time.Now().UTC().Format(time.RFC3339Nano),
		"isApiErrorMessage": true,
		"error":             code,
		"apiErrorStatus":    nil,
		"message": map[string]any{
			"role":    "assistant",
			"model":   "<synthetic>",
			"content": []map[string]any{{"type": "text", "text": "API Error: " + code}},
		},
	}); err != nil {
		return err
	}
	return t.setStatus(StatusIdle, "")
}

// skillInstalled reports whether the install put skill ("/name") in the home.
func (o tuiOptions) skillInstalled(skill string) bool {
	_, err := os.Stat(filepath.Join(o.home, ".claude", "skills", strings.TrimPrefix(skill, "/"), "SKILL.md"))
	return err == nil
}

// skillUnknown reports whether the registry rejects skill ("/name") now.
func (o tuiOptions) skillUnknown(skill string, sinceStart time.Duration) bool {
	if o.unknownUntilFile != "" {
		if _, err := os.Stat(o.unknownUntilFile); err != nil {
			return true
		}
	}
	return o.unknownSkill || sinceStart < o.registryDelay || (o.unknownUntilInstalled && !o.skillInstalled(skill))
}

// skillPromptKnobs applies the launch-proof knobs to a skill prompt and reports
// whether it handled the prompt. The launch-record knob only writes the user
// record and leaves the rest to the normal path.
func (t *tui) skillPromptKnobs(text string) (bool, error) {
	if !strings.HasPrefix(text, "/") || text == "/clear" || text == "/exit" || isCompact(text) || t.opensLocalDialog(text) {
		return false, nil
	}
	o := t.opts
	if !o.unknownSkill && !o.unknownUntilInstalled && o.unknownUntilFile == "" && o.registryDelay == 0 && !o.noTurn && !o.launchRecord && o.pasteOver == 0 {
		return false, nil
	}
	skill, _, _ := strings.Cut(text, " ")
	path, err := t.transcriptPath()
	if err != nil {
		return false, err
	}
	switch {
	case o.skillUnknown(skill, time.Since(t.started)):
		if text := UnknownCommandText(skill); text == t.notice {
			t.noticeRows++
		} else {
			t.notice, t.noticeRows = text, 1
		}
		t.draw()
		return true, AppendLine(path, map[string]any{
			"type":      "system",
			"subtype":   "informational",
			"content":   t.notice,
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		})
	case t.opts.noTurn:
		return true, nil
	case t.opts.pasteOver > 0 && len(text) > t.opts.pasteOver:
		// A paste is no command: the skill never runs and no turn follows.
		return true, AppendEntry(path, "user", "<pasted_content>\n"+text+"\n</pasted_content>", map[string]any{"queueSkipSlashCommands": true})
	case t.opts.launchRecord && t.commandOutput == nil:
		// An armed command output writes the command line itself.
		name := strings.TrimPrefix(skill, "/")
		args := strings.TrimSpace(strings.TrimPrefix(text, skill))
		return false, AppendEntry(path, "user", "<command-message>"+name+"</command-message>\n<command-name>"+skill+"</command-name>\n<command-args>"+args+"</command-args>", nil)
	}
	return false, nil
}

// promptKind is how an idle fake handles a submitted prompt.
type promptKind int

const (
	promptPlain promptKind = iota
	promptDesign
	promptStartDesign
	promptTriage
	promptWorker
	promptSlash
	promptPermission
)

// classify picks the prompt's handler. The order matters: a design prompt also
// looks like `/skill <address>`, so it comes before the worker check, and a
// worker prompt is also a slash command. For a worker prompt it returns the
// ticket address and the worker config.
func (t *tui) classify(text string) (promptKind, string, WorkerConfig) {
	if triageplay.IsStartDesignPrompt(text) || (t.startDesign != nil && text != "" && !strings.HasPrefix(text, "/")) {
		return promptStartDesign, "", WorkerConfig{}
	}
	if IsDesignPrompt(text) || (t.design.Active && text != "" && !strings.HasPrefix(text, "/")) {
		return promptDesign, "", WorkerConfig{}
	}
	if IsTriagePrompt(text) || (t.triageProject != "" && text != "" && !strings.HasPrefix(text, "/")) {
		return promptTriage, "", WorkerConfig{}
	}
	// Like claude, `/compact <args>` starts a plain turn, never ticket work.
	if isCompact(text) {
		return promptPlain, "", WorkerConfig{}
	}
	if address, cfg, ok := t.workerPrompt(text); ok {
		return promptWorker, address, cfg
	}
	if strings.HasPrefix(text, "/") {
		return promptSlash, "", WorkerConfig{}
	}
	if strings.HasPrefix(text, PermissionPromptPrefix) {
		return promptPermission, "", WorkerConfig{}
	}
	return promptPlain, "", WorkerConfig{}
}

// isCompact reports whether text is `/compact`, with or without arguments.
func isCompact(text string) bool {
	return text == "/compact" || strings.HasPrefix(text, "/compact ")
}

// workerPrompt reports whether text is a skill prompt (`/skill <address>`) for
// a fake that runs in a worktree carrying a worker config, and returns the
// ticket address and that config.
func (t *tui) workerPrompt(text string) (string, WorkerConfig, bool) {
	if strings.HasPrefix(text, FinishBlockPrefix) {
		cfg, err := LoadWorkerConfig(t.pid.Cwd)
		if err != nil {
			return "", WorkerConfig{}, false
		}
		address, ok := UncheckedAddress(cfg)
		return address, cfg, ok
	}
	fields := strings.Fields(text)
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "/") {
		return "", WorkerConfig{}, false
	}
	// The address is the first argument: a relaunch appends earlier iteration
	// summaries after it.
	address := fields[1]
	if fields[0] == "/gx-investigate" {
		// A fix agent's prompt ends in free text, not a ticket address. A queued
		// investigate ticket's launch sets the env to its address and waits for a turn.
		address = os.Getenv("GX_INVESTIGATE_TICKET")
		if address == "" {
			return "", WorkerConfig{}, false
		}
	}
	cfg, err := LoadWorkerConfig(t.pid.Cwd)
	if err != nil {
		return "", WorkerConfig{}, false
	}
	return address, cfg, true
}

// work does the ticket's work off the input loop, then sends the turn's end
// down the control channel like a test would.
func (t *tui) work(cfg WorkerConfig, address string) {
	// The finish report ends the agent, so a statusLine run after the turn can
	// be too late; claude also refreshes it mid-turn.
	t.runStatusLine()
	began := time.Now()
	// The asking turn is the one that has not left UncommittedFile yet; a relaunch
	// after the answer finds it and must stay up to finish.
	exitAfterAsk := cfg.ExitAfterAsk && IsAsk(cfg.Behaviour[address]) && !FileExists(UncommittedFile)
	failing := t.opts.classifierOutage > 0
	if failing {
		t.opts.classifierOutage--
		if path, err := t.transcriptPath(); err == nil {
			_ = appendClassifierNoVerdict(path)
		}
	} else if err := WorkTicket(cfg, address); err != nil {
		fmt.Fprintln(os.Stderr, "fakeclaude worker:", err)
	}
	// An ask is one gx call, and an outage turn is one appended record, so under
	// load the whole turn can fit between two polls of the pid file: the launch
	// then sees no turn and pauses as agent_prompt_stalled.
	if IsAsk(cfg.Behaviour[address]) || failing {
		time.Sleep(max(0, stallTurnLength-time.Since(began)))
	}
	if cfg.Behaviour[address] == BehaviourFail {
		_ = SendControl(t.ctlPath, Control{Action: ActionExit, Code: 1})
		return
	}
	if exitAfterAsk {
		// Go idle first and leave once the server has adopted the ask: an exit
		// before that reads as a died session.
		_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
		WaitTicketLeavesClaimed(cfg, address, exitAfterAskLimit)
		if cfg.Hold != "" {
			_ = os.WriteFile(cfg.Hold+ExitedSuffix, nil, 0o644)
		}
		_ = SendControl(t.ctlPath, Control{Action: ActionExit})
		return
	}
	if StartsBackgroundTask(cfg.Behaviour[address]) {
		if err := t.startBackgroundTask(cfg.Hold); err != nil {
			fmt.Fprintln(os.Stderr, "fakeclaude background task:", err)
		}
		_ = SendControl(t.ctlPath, Control{Action: ActionShell})
		return
	}
	_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
}

// appendClassifierNoVerdict appends one failed tool call: the assistant's call
// and the "no verdict" error that answers it.
func appendClassifierNoVerdict(path string) error {
	const id = "toolu_noverdict"
	call := []any{map[string]any{"type": "tool_use", "id": id, "name": "Bash", "input": map[string]any{"command": "gx tickets show"}}}
	failure := []any{map[string]any{"type": "tool_result", "tool_use_id": id, "is_error": true, "content": "Error: auto mode classifier gave no verdict"}}
	if err := AppendMessage(path, "assistant", call); err != nil {
		return err
	}
	return AppendMessage(path, "user", failure)
}

// fastTurn runs an armed fast turn: working, then done, with no busy status.
func (t *tui) fastTurn() error {
	t.fast = false
	WriteReport(t.out, "working", "", "")
	WriteReport(t.out, "done", "", "")
	return nil
}

// slashCommand handles a "/" command at idle as claude does: it starts no
// turn. /clear starts a new conversation; a dialog command waits on a dialog.
// The server launches every iteration with a slash prompt, so an armed fast or
// busy turn makes one run a turn anyway.
func (t *tui) slashCommand(text string) error {
	switch {
	case t.dialogNext:
		t.dialogNext = false
		return t.setStatus(StatusWaiting, "input needed")
	case text == "/clear":
		t.pid.SessionID += "-cleared"
		return t.writePid()
	case text == "/exit":
		t.quit = true
		return nil
	case t.commandOutput != nil:
		stdout := *t.commandOutput
		t.commandOutput = nil
		return t.writeCommandOutput(text, stdout)
	case t.fast:
		return t.fastTurn()
	case t.busyNext:
		t.busyNext = false
		t.prompt = text
		return t.setStatus(StatusBusy, "")
	case t.opensLocalDialog(text):
		t.openLocalDialog(text)
	}
	return nil
}
