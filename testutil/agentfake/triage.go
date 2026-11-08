package agentfake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/elentok/gx/triage"
)

// TriagePromptPrefix, at the start of a submitted prompt, makes the TUI fake
// play one step of /gx-triage-follow-ups with the real `gx tickets triage`
// verbs, as the skill would.
const TriagePromptPrefix = "/gx-triage-follow-ups "

// TriageConfig is the Triage fake's knobs. The zero value verifies every
// pending row in one session and fails no row.
type TriageConfig struct {
	// VerifyBatch is how many pending rows one Verify session saves; zero saves
	// them all.
	VerifyBatch int `json:"verify_batch,omitempty"`
	// FailBeforeMove and FailAfterMove name follow-up addresses whose Write
	// fails once with a scripted refusal, before the row's move and after it.
	// The rerun after Retry succeeds.
	FailBeforeMove []string `json:"fail_before_move,omitempty"`
	FailAfterMove  []string `json:"fail_after_move,omitempty"`
}

// The scripted refusals of the fail-once knobs. A row that already carries one
// has failed once, which is how the rerun knows to succeed.
const (
	TriageRefusalBeforeMove = "ticket claimed by another agent"
	TriageRefusalAfterMove  = "epic add failed"
)

const (
	triageReadyEpic = "triage-ready"
	triageMapEpic   = "triage-map"
)

// triageActions is the plan's action per row, in row order, so five rows cover
// every action.
var triageActions = []string{"ready", "map", "merge", "close", "keep"}

// IsTriagePrompt reports whether text launches a Triage step.
func IsTriagePrompt(text string) bool { return strings.HasPrefix(text, TriagePromptPrefix) }

func triageProject(text string) string {
	fields := strings.Fields(text)
	if i := slices.Index(fields, "--project"); i >= 0 && i+1 < len(fields) {
		return fields[i+1]
	}
	return ""
}

// triageTurn plays the step the record's next branch names, then ends the turn
// the way designTurn does: the run-end line lands in the transcript while the
// turn is busy, and the turn ends shortly after.
func (t *tui) triageTurn(text string) error {
	t.prompt = text
	cfg, _ := LoadWorkerConfig(t.pid.Cwd)
	project := triageProject(text)
	if err := t.setStatus(StatusBusy, ""); err != nil {
		return err
	}
	path, err := t.transcriptPath()
	if err != nil {
		return err
	}
	go func() {
		p := triagePlayer{cfg: cfg, project: project, dir: t.pid.Cwd}
		msg, err := p.play()
		if err != nil {
			msg = fmt.Sprintf("Error: %v\n\n✅ triage: stopped on an error. Next: %s", err, p.next())
		}
		if err := appendReply(path, t.opts.model, []map[string]any{{"type": "text", "text": msg}}); err != nil {
			fmt.Fprintln(os.Stderr, "fakeclaude triage:", err)
		}
		time.Sleep(DesignTurnTime)
		_ = SendControl(t.ctlPath, Control{Action: ActionFinish})
	}()
	return nil
}

// triagePlayer runs the real verbs for one session.
type triagePlayer struct {
	cfg     WorkerConfig
	project string
	dir     string
}

func (p *triagePlayer) knobs() TriageConfig {
	if p.cfg.Triage == nil {
		return TriageConfig{}
	}
	return *p.cfg.Triage
}

func (p *triagePlayer) next() string {
	return "/clear, then " + strings.TrimSpace(TriagePromptPrefix) + " --project " + p.project
}

// gx runs the real gx binary in the session's directory and returns its stdout;
// a failure's error is the verb's own text.
func (p *triagePlayer) gx(stdin string, args ...string) (string, error) {
	cmd := exec.Command(p.cfg.Gx, args...)
	cmd.Dir = p.dir
	cmd.Env = append(os.Environ(), p.cfg.Env...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(stderr.String())
		if text == "" {
			text = strings.TrimSpace(stdout.String())
		}
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("%s", text)
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (p *triagePlayer) triage(stdin string, args ...string) (string, error) {
	return p.gx(stdin, append([]string{"tickets", "triage", args[0], "--project", p.project}, args[1:]...)...)
}

// triageShow is `gx tickets triage show --json`: the record and the next step.
type triageShow struct {
	*triage.Record
	Next triage.Branch `json:"next"`
}

func (p *triagePlayer) show() (triageShow, error) {
	var v triageShow
	out, err := p.triage("", "show", "--json")
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal([]byte(out), &v)
}

func (p *triagePlayer) play() (string, error) {
	if _, err := p.gx("", "agent", "link", "--triage", p.project); err != nil {
		return "", err
	}
	v, err := p.show()
	if err != nil {
		return "", err
	}
	if v.Next == triage.BranchStart {
		if _, err := p.triage("", "create"); err != nil {
			return "", err
		}
		if v, err = p.show(); err != nil {
			return "", err
		}
	}
	switch v.Next {
	case triage.BranchVerify:
		return p.verify(v.Record)
	case triage.BranchPlan:
		return p.plan(v.Record)
	case triage.BranchWrite:
		return p.write(v.Record)
	case triage.BranchDone:
		if _, err := p.gx("", "agent", "step", "done"); err != nil {
			return "", err
		}
		return "✅ done: the follow-ups are triaged. Next: nothing, the Triage is over", nil
	}
	return "✅ approve: the plan is saved. Next: approve it in the web plan view", nil
}

func (p *triagePlayer) verify(rec *triage.Record) (string, error) {
	if _, err := p.gx("", "agent", "step", "verify"); err != nil {
		return "", err
	}
	rows := rec.PendingVerdicts()
	if n := p.knobs().VerifyBatch; n > 0 && len(rows) > n {
		rows = rows[:n]
	}
	for _, r := range rows {
		if _, err := p.triage("", "verdict", "--row", r.Address, "--verdict", "relevant", "--evidence", "still reproduces on trunk"); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("✅ verify: saved %d verdicts. Next: %s", len(rows), p.next()), nil
}

// triagePlanRows is the plan: one action per row from triageActions, two quick
// questions on the first row and a design question on the second.
func triagePlanRows(rec *triage.Record) []triage.PlanRow {
	var plan []triage.PlanRow
	for i, r := range rec.Rows {
		row := triage.PlanRow{RowID: r.Address, Action: triageActions[i%len(triageActions)]}
		switch row.Action {
		case "ready":
			row.Destination = triageReadyEpic
		case "map":
			row.Destination = triageMapEpic
		case "merge":
			row.Destination = rec.Rows[0].Address
		case "close":
			row.Destination = "obsolete"
		}
		switch i {
		case 0:
			row.Questions = []triage.Question{
				{ID: "q1", Text: "Fix it in the parser or the caller?", Kind: triage.QuestionQuick, Options: []string{"parser", "caller"}, Default: "parser"},
				{ID: "q2", Text: "Keep the old flag?", Kind: triage.QuestionQuick, Options: []string{"yes", "no"}, Default: "no"},
			}
		case 1:
			row.Questions = []triage.Question{{ID: "d1", Text: "Which store should it use?", Kind: triage.QuestionDesign}}
		}
		plan = append(plan, row)
	}
	return plan
}

func (p *triagePlayer) plan(rec *triage.Record) (string, error) {
	if _, err := p.gx("", "agent", "step", "plan"); err != nil {
		return "", err
	}
	body, err := json.Marshal(triagePlanRows(rec))
	if err != nil {
		return "", err
	}
	if _, err := p.triage(string(body), "plan"); err != nil {
		return "", err
	}
	return fmt.Sprintf("✅ plan: %d rows planned. Next: approve the plan in the web plan view", len(rec.Rows)), nil
}

// writeRow is one row's Write: the ordered verbs, each skipped when locate says
// it already happened.
func (p *triagePlayer) writeRow(r triage.Row) (newAddr string, err error) {
	action, dest := r.Effective()
	var loc struct {
		Position string `json:"position"`
		Address  string `json:"address"`
	}
	out, err := p.triage("", "locate", "--row", r.Address)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal([]byte(out), &loc); err != nil {
		return "", err
	}
	newAddr = loc.Address
	if _, err := p.triage("", "answers", "--row", r.Address); err != nil {
		return "", err
	}
	knobs := p.knobs()
	switch action {
	case "merge":
		if _, err := p.triage("", "seen-again", "--row", dest, "--from", r.Address); err != nil {
			return "", err
		}
		return "", p.closeRow(loc.Position, r.Address, "duplicate")
	case "close":
		return "", p.closeRow(loc.Position, r.Address, dest)
	case "ready", "map":
		if slices.Contains(knobs.FailBeforeMove, r.Address) && r.Refusal != TriageRefusalBeforeMove {
			return "", fmt.Errorf("%s", TriageRefusalBeforeMove)
		}
		epic := p.project + ":" + dest
		if loc.Position == "follow-ups" {
			out, err := p.gx("", "tickets", "move", r.Address, epic, "--json")
			if err != nil {
				return "", err
			}
			var moved struct {
				To string `json:"to"`
			}
			if err := json.Unmarshal([]byte(out), &moved); err != nil {
				return "", err
			}
			newAddr = moved.To
		}
		if slices.Contains(knobs.FailAfterMove, r.Address) && r.Refusal != TriageRefusalAfterMove {
			return newAddr, fmt.Errorf("%s", TriageRefusalAfterMove)
		}
		typ := "implement"
		if action == "map" {
			typ = "grilling"
			for _, q := range r.AllQuestions() {
				if q.Kind == triage.QuestionDesign {
					if _, err := p.gx("## Question\n"+q.Text+"\n", "tickets", "section", newAddr, "Question", "-"); err != nil {
						return newAddr, err
					}
				}
			}
		}
		_, err = p.gx("", "tickets", "set", newAddr, "--type", typ, "--status", "open")
		return newAddr, err
	}
	return "", nil
}

func (p *triagePlayer) closeRow(position, address, reason string) error {
	if position != "follow-ups" {
		return nil
	}
	_, err := p.gx("", "tickets", "close", address, "--reason", reason, "--note", "closed by the scripted Triage")
	return err
}

func (p *triagePlayer) write(rec *triage.Record) (string, error) {
	if _, err := p.gx("", "agent", "step", "write"); err != nil {
		return "", err
	}
	var ready, maps []string
	for _, r := range rec.Rows {
		switch action, _ := r.Effective(); action {
		case "ready":
			ready = []string{p.project + ":" + triageReadyEpic}
		case "map":
			maps = []string{p.project + ":" + triageMapEpic}
		}
	}
	if err := p.recordEpics(ready, maps); err != nil {
		return "", err
	}
	var done, failed int
	for _, r := range rec.Rows {
		if r.Write == triage.WriteDone || r.Write == triage.WriteGone || r.Write == triage.WriteFailed || r.Person.KeepForLater {
			continue
		}
		newAddr, err := p.writeRow(r)
		if err != nil {
			failed++
			args := []string{"write-state", "--row", r.Address, "--state", "failed", "--refusal", err.Error()}
			if newAddr != "" && newAddr != r.Address {
				args = append(args, "--new-address", newAddr)
			}
			if _, err := p.triage("", args...); err != nil {
				return "", err
			}
			continue
		}
		args := []string{"write-state", "--row", r.Address, "--state", "done"}
		if newAddr != "" && newAddr != r.Address {
			args = append(args, "--new-address", newAddr)
		}
		if _, err := p.triage("", args...); err != nil {
			return "", err
		}
		done++
	}
	if err := p.lastPass(len(ready) > 0); err != nil {
		return "", err
	}
	return fmt.Sprintf("✅ write: %d rows written, %d failed. Next: retry or keep the failed rows in the web plan view", done, failed), nil
}

// recordEpics records the epics and creates the ones the record did not list
// before; a refusal on a "known" epic means an earlier session made it.
func (p *triagePlayer) recordEpics(ready, maps []string) error {
	if len(ready)+len(maps) == 0 {
		return nil
	}
	var args []string
	for _, e := range ready {
		args = append(args, "--ready", e)
	}
	for _, e := range maps {
		args = append(args, "--map", e)
	}
	out, err := p.triage("", append([]string{"epics"}, args...)...)
	if err != nil {
		return err
	}
	for line := range strings.SplitSeq(out, "\n") {
		epic, kind, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		add := []string{"tickets", "epic", "add", epic, "--kind", "loop", "--status", "open"}
		stdin := ""
		if strings.HasSuffix(epic, ":"+triageMapEpic) {
			add = []string{"tickets", "epic", "add", epic, "--kind", "map", "--status", "open", "--body", "-"}
			stdin = "---\ndestination_unconfirmed: true\n---\n\n## Destination\n\nDecide these items, then a spec.\n\n## Notes\n\nTriaged from follow-ups.\n"
		}
		if _, err := p.gx(stdin, add...); err != nil && kind != "known" {
			return err
		}
	}
	return nil
}

// lastPass adds the ready epic's code-review stub, sweeps the closed follow-ups
// and sets the flag. The scripted plan has no overlapping rows, so no chain
// edges are set.
func (p *triagePlayer) lastPass(hasReady bool) error {
	if hasReady {
		if _, err := p.gx("", "tickets", "ensure-code-review", p.project+":"+triageReadyEpic); err != nil {
			return err
		}
	}
	if _, err := p.gx("", "tickets", "close", "--sweep", p.project); err != nil {
		return err
	}
	_, err := p.triage("", "last-pass")
	return err
}
