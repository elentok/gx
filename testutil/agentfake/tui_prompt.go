package agentfake

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// submit is Enter. A stalled fake leaves the text unsent in the input box.
func (t *tui) submit() error {
	if t.stalled {
		t.draw()
		return nil
	}
	text := t.typed.String()
	t.typed.Reset()
	if text == "" && t.pid.Status != StatusWaiting {
		return nil
	}
	switch t.pid.Status {
	case StatusIdle:
		if handled, err := t.skillPromptKnobs(text); handled || err != nil {
			return err
		}
		if w := t.resume; w != nil && text != "" {
			t.resume = nil
			t.prompt = text
			go t.work(w.cfg, w.address)
			return t.setStatus(StatusBusy, "")
		}
		kind, address, cfg := t.classify(text)
		switch kind {
		case promptDesign:
			return t.designTurn(text)
		case promptWorker:
			t.prompt = text
			if cfg.Transcript {
				path, err := t.transcriptPath()
				if err != nil {
					return err
				}
				if err := appendMessage(path, "user", text); err != nil {
					return err
				}
			}
			switch cfg.Behaviour[address] {
			case BehaviourPermission:
				t.dialog = BashPermissionDialog(PermissionCommand)
				return t.setStatus(StatusWaiting, "permission prompt")
			case BehaviourQuestionMenu:
				t.resume = &pendingWork{cfg, address}
				t.dialog = QuestionMenuDialog()
				return t.setStatus(StatusWaiting, "question menu")
			case BehaviourRateLimit:
				t.resume = &pendingWork{cfg, address}
				return t.rateLimit(cfg)
			case BehaviourStall, BehaviourStallNoRecord:
				t.resume = &pendingWork{cfg, address}
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
				if !fileExists(UncommittedFile) {
					t.resume = &pendingWork{cfg, address}
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
		if t.fast {
			return t.fastTurn()
		}
		if cfg, err := LoadWorkerConfig(t.pid.Cwd); err == nil && cfg.Chat {
			return t.chatTurn(text)
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

// skillPromptKnobs applies the launch-proof knobs to a skill prompt and reports
// whether it handled the prompt. The launch-record knob only writes the user
// record and leaves the rest to the normal path.
func (t *tui) skillPromptKnobs(text string) (bool, error) {
	if !strings.HasPrefix(text, "/") || text == "/clear" || text == "/exit" || text == "/compact" {
		return false, nil
	}
	o := t.opts
	if !o.unknownSkill && o.registryDelay == 0 && !o.noTurn && !o.launchRecord {
		return false, nil
	}
	skill, _, _ := strings.Cut(text, " ")
	path, err := t.transcriptPath()
	if err != nil {
		return false, err
	}
	switch {
	case t.opts.unknownSkill || time.Since(t.started) < t.opts.registryDelay:
		t.notice = UnknownCommandText(skill)
		t.draw()
		return true, appendLine(path, map[string]any{
			"type":      "system",
			"subtype":   "informational",
			"content":   t.notice,
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		})
	case t.opts.noTurn:
		return true, nil
	case t.opts.launchRecord && t.commandOutput == nil:
		// An armed command output writes the command line itself.
		name := strings.TrimPrefix(skill, "/")
		args := strings.TrimSpace(strings.TrimPrefix(text, skill))
		return false, appendEntry(path, "user", "<command-message>"+name+"</command-message>\n<command-name>"+skill+"</command-name>\n<command-args>"+args+"</command-args>", nil)
	}
	return false, nil
}

// promptKind is how an idle fake handles a submitted prompt.
type promptKind int

const (
	promptPlain promptKind = iota
	promptDesign
	promptWorker
	promptSlash
	promptPermission
)

// classify picks the prompt's handler. The order matters: a design prompt also
// looks like `/skill <address>`, so it comes before the worker check, and a
// worker prompt is also a slash command. For a worker prompt it returns the
// ticket address and the worker config.
func (t *tui) classify(text string) (promptKind, string, WorkerConfig) {
	if isDesignPrompt(text) || (t.design.active && text != "" && !strings.HasPrefix(text, "/")) {
		return promptDesign, "", WorkerConfig{}
	}
	if address, cfg, ok := t.workerPrompt(text); ok {
		return promptWorker, address, cfg
	}
	if strings.HasPrefix(text, "/") && text != "/compact" {
		return promptSlash, "", WorkerConfig{}
	}
	if strings.HasPrefix(text, PermissionPromptPrefix) {
		return promptPermission, "", WorkerConfig{}
	}
	return promptPlain, "", WorkerConfig{}
}

// workerPrompt reports whether text is a skill prompt (`/skill <address>`) for
// a fake that runs in a worktree carrying a worker config, and returns the
// ticket address and that config.
func (t *tui) workerPrompt(text string) (string, WorkerConfig, bool) {
	fields := strings.Fields(text)
	if len(fields) < 2 || !strings.HasPrefix(fields[0], "/") {
		return "", WorkerConfig{}, false
	}
	// A fix agent's prompt ends in free text, not a ticket address.
	if fields[0] == "/gx-investigate" {
		return "", WorkerConfig{}, false
	}
	cfg, err := LoadWorkerConfig(t.pid.Cwd)
	if err != nil {
		return "", WorkerConfig{}, false
	}
	return fields[len(fields)-1], cfg, true
}

// work does the ticket's work off the input loop, then sends the turn's end
// down the control channel like a test would.
func (t *tui) work(cfg WorkerConfig, address string) {
	// The finish report ends the agent, so a statusLine run after the turn can
	// be too late; claude also refreshes it mid-turn.
	t.runStatusLine()
	began := time.Now()
	if err := WorkTicket(cfg, address); err != nil {
		fmt.Fprintln(os.Stderr, "fakeclaude worker:", err)
	}
	if b := cfg.Behaviour[address]; b == BehaviourAsk || b == BehaviourWriteAsk {
		// An ask is one gx call, so under load the whole turn can fit between two
		// polls of the pid file: the launch then sees no turn and pauses as
		// agent_prompt_stalled over the question.
		time.Sleep(max(0, stallTurnLength-time.Since(began)))
	}
	if cfg.Behaviour[address] == BehaviourFail {
		_ = SendControl(t.ctlPath, Control{Action: ActionExit, Code: 1})
		return
	}
	_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
}

// fastTurn runs an armed fast turn: working, then done, with no busy status.
func (t *tui) fastTurn() error {
	t.fast = false
	writeReport(t.out, "working", "", "")
	writeReport(t.out, "done", "", "")
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
	}
	return nil
}
