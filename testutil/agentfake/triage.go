package agentfake

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
	cfg, _ := LoadWorkerConfig(t.pid.Cwd)
	if err := t.setStatus(StatusBusy, ""); err != nil {
		return err
	}
	path, err := t.transcriptPath()
	if err != nil {
		return err
	}
	go func() {
		p := triageplay.Player{Gx: cfg.Gx, Env: cfg.Env, Project: triageplay.Project(text), Dir: t.pid.Cwd}
		if cfg.Triage != nil {
			p.Config = *cfg.Triage
		}
		msg, err := p.Play()
		if err != nil {
			msg = fmt.Sprintf("Error: %v\n\n✅ triage: stopped on an error. Next: %s", err, p.Next())
		}
		if err := appendReply(path, t.opts.model, []map[string]any{{"type": "text", "text": msg}}); err != nil {
			fmt.Fprintln(os.Stderr, "fakeclaude triage:", err)
		}
		time.Sleep(DesignTurnTime)
		_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
	}()
	return nil
}
