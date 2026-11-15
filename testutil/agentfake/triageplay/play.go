// Package triageplay is the scripted /gx-triage-follow-ups session the TUI fake
// plays, driving the real `gx tickets triage` verbs as the skill would.
package triageplay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/elentok/gx/triage"
)

// Config is the Triage fake's knobs. The zero value verifies every
// pending row in one session and fails no row.
type Config struct {
	// VerifyBatch is how many pending rows one Verify session saves; zero saves
	// them all.
	VerifyBatch int `json:"verify_batch,omitempty"`
	// FailBeforeMove and FailAfterMove name follow-up addresses whose Write
	// fails once with a scripted refusal, before the row's move and after it.
	// The rerun after Retry succeeds.
	FailBeforeMove []string `json:"fail_before_move,omitempty"`
	FailAfterMove  []string `json:"fail_after_move,omitempty"`
	// HoldAtStep makes the session wait for the worker's Hold file before it
	// plays its step, so the page stays at the step the record is on.
	HoldAtStep bool `json:"hold_at_step,omitempty"`
}

// The scripted refusals of the fail-once knobs. A row that already carries one
// has failed once, which is how the rerun knows to succeed.
const (
	RefusalBeforeMove = "ticket claimed by another agent"
	RefusalAfterMove  = "epic add failed"
)

// PromptPrefix, at the start of a submitted prompt, makes the TUI fake play one
// step of /gx-triage-follow-ups.
const PromptPrefix = "/gx-triage-follow-ups "

const (
	triageReadyEpic = "triage-ready"
	triageMapEpic   = "triage-map"
)

// Project is the --project a prompt names.
func Project(text string) string {
	fields := strings.Fields(text)
	if i := slices.Index(fields, "--project"); i >= 0 && i+1 < len(fields) {
		return fields[i+1]
	}
	return ""
}

// Player runs the real verbs, with the gx binary at Gx, for one session in Dir.
type Player struct {
	Gx      string
	Env     []string
	Config  Config
	Project string
	Dir     string
	// Typed is a message the person typed into the session; empty for the
	// skill prompt.
	Typed string
}

// Next is the line telling the person how to continue after a step.
func (p *Player) Next() string {
	return "/clear, then " + strings.TrimSpace(PromptPrefix) + " --project " + p.Project
}

// gx runs the real gx binary in the session's directory and returns its stdout;
// a failure's error is the verb's own text.
func (p *Player) gx(stdin string, args ...string) (string, error) {
	cmd := exec.Command(p.Gx, args...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), p.Env...)
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

func (p *Player) triage(stdin string, args ...string) (string, error) {
	return p.gx(stdin, append([]string{"tickets", "triage", args[0], "--project", p.Project}, args[1:]...)...)
}

// triageShow is `gx tickets triage show --json`: the record and the next step.
type triageShow struct {
	*triage.Record
	Next triage.Branch `json:"next"`
}

func (p *Player) show() (triageShow, error) {
	var v triageShow
	out, err := p.triage("", "show", "--json")
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal([]byte(out), &v)
}

// Play plays the step the record's next branch names and returns the run-end
// line.
func (p *Player) Play() (string, error) {
	if _, err := p.gx("", "agent", "link", "--triage", p.Project); err != nil {
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
	if p.Typed != "" {
		return p.react(v.Record)
	}
	switch v.Next {
	case triage.BranchVerify:
		return p.verify(v.Record)
	case triage.BranchRoute:
		return p.route(v.Record)
	case triage.BranchQuestions:
		return p.questions(v.Record)
	case triage.BranchWrite:
		return p.write(v.Record)
	case triage.BranchDone:
		if _, err := p.gx("", "agent", "step", "done"); err != nil {
			return "", err
		}
		return "✅ done: the follow-ups are triaged. Next: nothing, the Triage is over", nil
	}
	return "✅ start: nothing to do yet. Next: " + p.Next(), nil
}

func (p *Player) verify(rec *triage.Record) (string, error) {
	if _, err := p.gx("", "agent", "step", "verify"); err != nil {
		return "", err
	}
	rows := rec.PendingVerdicts()
	if n := p.Config.VerifyBatch; n > 0 && len(rows) > n {
		rows = rows[:n]
	}
	for _, r := range rows {
		if _, err := p.triage("", "verdict", "--row", r.Address, "--verdict", "relevant", "--evidence", "still reproduces on trunk"); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("✅ verify: saved %d verdicts. Next: %s", len(rows), p.Next()), nil
}

// writeRow is one row's Write: the ordered verbs, each skipped when locate says
// it already happened.
func (p *Player) writeRow(r triage.Row) (newAddr string, err error) {
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
	knobs := p.Config
	switch action {
	case "merge":
		if _, err := p.triage("", "seen-again", "--row", dest, "--from", r.Address); err != nil {
			return "", err
		}
		return "", p.closeRow(loc.Position, r.Address, "duplicate")
	case "close":
		return "", p.closeRow(loc.Position, r.Address, dest)
	case "ready", "map":
		if slices.Contains(knobs.FailBeforeMove, r.Address) && r.Refusal != RefusalBeforeMove {
			return "", fmt.Errorf("%s", RefusalBeforeMove)
		}
		epic := p.Project + ":" + dest
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
		if slices.Contains(knobs.FailAfterMove, r.Address) && r.Refusal != RefusalAfterMove {
			return newAddr, fmt.Errorf("%s", RefusalAfterMove)
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

func (p *Player) closeRow(position, address, reason string) error {
	if position != "follow-ups" {
		return nil
	}
	_, err := p.gx("", "tickets", "close", address, "--reason", reason, "--note", "closed by the scripted Triage")
	return err
}

func (p *Player) write(rec *triage.Record) (string, error) {
	if _, err := p.gx("", "agent", "step", "write"); err != nil {
		return "", err
	}
	var ready, maps []string
	for _, r := range rec.Rows {
		switch action, _ := r.Effective(); action {
		case "ready":
			ready = []string{p.Project + ":" + triageReadyEpic}
		case "map":
			maps = []string{p.Project + ":" + triageMapEpic}
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
	for _, u := range rec.Moves {
		if u.State == triage.MoveDone || u.State == triage.MoveKept {
			continue
		}
		moved, err := p.writeMove(u)
		if err != nil {
			failed++
			if _, err := p.triage("", "move-state", "--epic", u.OldEpic, "--state", "failed", "--error", err.Error()); err != nil {
				return "", err
			}
			continue
		}
		if !moved {
			continue
		}
		if _, err := p.triage("", "move-state", "--epic", u.OldEpic, "--state", "done"); err != nil {
			return "", err
		}
		done++
	}
	if err := p.lastPass(len(ready) > 0); err != nil {
		return "", err
	}
	return fmt.Sprintf("✅ write: %d rows written, %d failed. Next: retry or keep the failed rows in the web plan view", done, failed), nil
}

// writeMove is one old-epic unit's Write, as the skill's "Old epics" section
// says: move what move-locate lists into the area (created when missing), then
// archive the emptied epic, which stamps it. A unit whose epic holds a claimed
// ticket is left pending (moved is false) and its claimed ticket stays.
func (p *Player) writeMove(u triage.MoveUnit) (moved bool, err error) {
	locate := func() (loc struct {
		Move    []string `json:"move"`
		Claimed []string `json:"claimed"`
		Archive bool     `json:"archive"`
	}, err error) {
		out, err := p.triage("", "move-locate", "--epic", u.OldEpic)
		if err != nil {
			return loc, err
		}
		return loc, json.Unmarshal([]byte(out), &loc)
	}
	loc, err := locate()
	if err != nil {
		return false, err
	}
	if len(loc.Move) == 0 && len(loc.Claimed) > 0 {
		return false, nil
	}
	// The table can name the old epic itself as its area; its tickets are home
	// already and `tickets move` refuses a move into the same epic.
	if u.Area == u.OldEpic {
		return true, nil
	}
	area := p.Project + ":" + u.Area
	if len(loc.Move) > 0 {
		// A refusal means the area exists already; a real failure shows in the move.
		_, _ = p.gx("", "tickets", "epic", "add", area, "--kind", "area", "--status", "open")
	}
	for _, addr := range loc.Move {
		if _, err := p.gx("", "tickets", "move", addr, area); err != nil {
			return false, err
		}
	}
	if loc, err = locate(); err != nil {
		return false, err
	}
	if loc.Archive {
		if _, err := p.gx("", "server", "epics", "archive", p.Project+":"+u.OldEpic, "--yes"); err != nil {
			return false, err
		}
	}
	return true, nil
}

// recordEpics records the epics and creates the ones the record did not list
// before; a refusal on a "known" epic means an earlier session made it.
func (p *Player) recordEpics(ready, maps []string) error {
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
func (p *Player) lastPass(hasReady bool) error {
	if hasReady {
		if _, err := p.gx("", "tickets", "ensure-code-review", p.Project+":"+triageReadyEpic); err != nil {
			return err
		}
	}
	if _, err := p.gx("", "tickets", "close", "--sweep", p.Project); err != nil {
		return err
	}
	_, err := p.triage("", "last-pass")
	return err
}
