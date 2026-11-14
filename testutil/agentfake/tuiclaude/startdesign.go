package tuiclaude

import . "github.com/elentok/gx/testutil/agentfake"

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/elentok/gx/testutil/agentfake/triageplay"
)

// startDesignTurn plays the Start Design chat: the skill prompt proposes the
// card, a typed message edits it or, on yes, creates the Design. The turn ends
// as triageTurn's does.
func (t *tui) startDesignTurn(text string) error {
	t.prompt = text
	cfg, _ := LoadWorkerConfig(t.pid.Cwd)
	if err := t.setStatus(StatusBusy, ""); err != nil {
		return err
	}
	path, err := t.transcriptPath()
	if err != nil {
		return err
	}
	typed := ""
	if !strings.HasPrefix(text, triageplay.StartDesignPromptPrefix) {
		typed = text
		if err := AppendMessage(path, "user", typed); err != nil {
			return err
		}
	}
	go func() {
		p := triageplay.DesignPlayer{Gx: cfg.Gx, Env: cfg.Env, Dir: t.pid.Cwd}
		var msg string
		var err error
		switch {
		case typed == "" || t.startDesign == nil:
			t.startDesign, msg, err = p.Propose(text)
		default:
			msg, err = p.React(t.startDesign, typed)
		}
		if err != nil {
			msg = fmt.Sprintf("Error: %v", err)
		}
		if err := AppendReply(path, t.opts.model, []map[string]any{{"type": "text", "text": msg}}); err != nil {
			fmt.Fprintln(os.Stderr, "fakeclaude start-design:", err)
		}
		time.Sleep(DesignTurnTime)
		_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
	}()
	return nil
}
