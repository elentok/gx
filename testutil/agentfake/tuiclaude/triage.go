package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/testutil/agentfake/triageplay"
)

// TriagePromptPrefix, at the start of a submitted prompt, makes the TUI fake
// play one step of /gx-triage-follow-ups with the real `gx tickets triage`
// verbs, as the skill would.
const TriagePromptPrefix = triageplay.PromptPrefix

// IsTriagePrompt reports whether text launches a Triage step.
func IsTriagePrompt(text string) bool { return strings.HasPrefix(text, TriagePromptPrefix) }

// triageTurn plays the step the record's next branch names, then ends the turn
// the way designTurn does: the run-end line lands in the transcript while the
// turn is busy, and the turn ends shortly after.
func (t *tui) triageTurn(text string) error {
	t.prompt = text
	// A typed message of a live Triage session has no skill prefix: it goes to
	// the player of the project the session started with.
	typed := ""
	if IsTriagePrompt(text) {
		t.triageProject = triageplay.Project(text)
	} else {
		typed = text
	}
	cfg, _ := LoadWorkerConfig(t.pid.Cwd)
	if err := t.setStatus(StatusBusy, ""); err != nil {
		return err
	}
	path, err := t.transcriptPath()
	if err != nil {
		return err
	}
	if typed != "" {
		if err := AppendMessage(path, "user", typed); err != nil {
			return err
		}
	}
	project := t.triageProject
	go func() {
		p := triageplay.Player{Gx: cfg.Gx, Env: cfg.Env, Project: project, Dir: t.pid.Cwd, Typed: typed}
		if cfg.Triage != nil {
			p.Config = *cfg.Triage
		}
		if p.Config.HoldAtStep {
			WaitHold(cfg)
		}
		msg, err := p.Play()
		if err != nil {
			msg = fmt.Sprintf("Error: %v\n\n✅ triage: stopped on an error. Next: %s", err, p.Next())
		}
		if err := AppendReply(path, t.opts.model, []map[string]any{{"type": "text", "text": msg}}); err != nil {
			fmt.Fprintln(os.Stderr, "fakeclaude triage:", err)
		}
		time.Sleep(DesignTurnTime)
		_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
	}()
	return nil
}
