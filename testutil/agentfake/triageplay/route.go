package triageplay

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/triage"
)

// NewArea is the area the fake Route proposes when there is room for one.
const NewArea = "tooling"

// NewAreaQuestionID is the id of the new-area confirmation question.
const NewAreaQuestionID = "area-new"

// The phrases a typed message may use; anything else gets a one-line reply.
// They are listed in webui/e2e/CATALOG.md.
const (
	// PhraseYes answers the final "Write this plan?" card.
	PhraseYes = "yes"
	// PhraseMove is a routing correction: "Move row <n> to <area>" or "Move
	// row <n> to ready", with n counting the rows from 1.
	PhraseMove = "Move row <n> to <area|ready>"
	// PhraseAnswer answers a question of the open round.
	PhraseAnswer = "Answer <question id>: <answer>"
	// PhraseObject objects to the n-th self-decided answer, counting from 1.
	PhraseObject = "Object to decision <n>"
)

var (
	moveRe   = regexp.MustCompile(`^Move row (\d+) to (\S+)$`)
	answerRe = regexp.MustCompile(`^Answer (\S+): (.+)$`)
	objectRe = regexp.MustCompile(`^Object to decision (\d+)$`)
)

// RoutePlan is the plan the fake Route saves: per six rows a ready row with an
// ask question, a row to the new area (or the closest one) with a design
// question, a row to an existing area, a ready row with an ask question, a
// close and a keep. areas are the existing area epics.
func RoutePlan(rec *triage.Record, areas []string) []triage.PlanRow {
	closest := NewArea
	if len(areas) > 0 {
		closest = areas[0]
	}
	propose := len(areas) < tickets.MaxAreas
	var plan []triage.PlanRow
	for i, r := range rec.Rows {
		row := triage.PlanRow{RowID: r.Address}
		ready := func(estimate string, q triage.Question) {
			row.Action, row.Destination, row.Estimate, row.Fallback = "ready", triageReadyEpic, estimate, closest
			row.Questions = []triage.Question{q}
		}
		switch i % 6 {
		case 0:
			ready("10K", triage.Question{ID: fmt.Sprintf("q%d", i+1), Text: "Fix it in the parser or the caller?", Kind: triage.QuestionAsk, Options: []string{"parser", "caller"}, Default: "parser"})
		case 1:
			row.Action, row.Destination = triage.ActionArea, closest
			if propose {
				row.Destination = NewArea
			}
			row.Questions = []triage.Question{{ID: fmt.Sprintf("d%d", i+1), Text: "Which store should it use?", Kind: triage.QuestionDesign}}
		case 2:
			row.Action, row.Destination = triage.ActionArea, closest
			if len(areas) > 1 {
				row.Destination = areas[1]
			}
		case 3:
			ready("12K", triage.Question{ID: fmt.Sprintf("q%d", i+1), Text: "Keep the old flag?", Kind: triage.QuestionAsk, Options: []string{"yes", "no"}, Default: "no"})
		case 4:
			row.Action, row.Destination = "close", "obsolete"
		default:
			row.Action = "keep"
		}
		plan = append(plan, row)
	}
	return plan
}

// Decisions are the answers the fake settles itself: a small one on the first
// row and, at the area cap, the filing of the second row under the closest area.
func Decisions(rec *triage.Record, areas []string) []triage.Decided {
	if len(rec.Rows) == 0 {
		return nil
	}
	out := []triage.Decided{{RowID: rec.Rows[0].Address, Question: "Rename the helper while there?", Choice: "yes", Reason: "it is a one-line change"}}
	if len(areas) >= tickets.MaxAreas && len(rec.Rows) > 1 {
		out = append(out, triage.Decided{RowID: rec.Rows[1].Address, Question: "Which area does it go to?", Choice: areas[0], Reason: "12 areas exist, so it is filed under the closest one"})
	}
	return out
}

// NewAreaQuestion confirms the new area: create it, file under the closest
// area (when one exists) or keep for later.
func NewAreaQuestion(rec *triage.Record, areas []string) triage.Question {
	create := "Create " + NewArea
	q := triage.Question{ID: NewAreaQuestionID, Text: fmt.Sprintf("Create the area %q?", NewArea), Kind: triage.QuestionAsk, Default: create, Options: []string{create}}
	if len(areas) > 0 {
		q.Options = append(q.Options, "File under "+areas[0])
	}
	q.Options = append(q.Options, "Keep for later")
	for _, r := range rec.Rows {
		if r.Plan != nil && r.Plan.Action == triage.ActionArea && r.Plan.Destination == NewArea {
			q.RowID = r.Address
			break
		}
	}
	return q
}

// areas lists the area epics of the player's project.
func (p *Player) areas() []string {
	root, err := p.gx("", "tickets", "root")
	if err != nil || root == "" {
		return nil
	}
	names, _ := tickets.AreaNames(root)
	return names
}

func closestArea(areas []string) string {
	if len(areas) > 0 {
		return areas[0]
	}
	return NewArea
}

// route saves the area proposal and the plan, then prints the summary card.
func (p *Player) route(rec *triage.Record) (string, error) {
	if _, err := p.gx("", "agent", "step", tickets.StepRoute); err != nil {
		return "", err
	}
	areas := p.areas()
	if len(areas) < tickets.MaxAreas {
		if _, err := p.triage("", "area", "--name", NewArea, "--source", triage.AreaAgent); err != nil {
			return "", err
		}
	}
	// The move units are saved at the Route step, so before the plan, which
	// moves the record on to Questions.
	moveLines, err := p.oldEpicMoves(areas)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(RoutePlan(rec, areas))
	if err != nil {
		return "", err
	}
	if _, err := p.triage(string(body), "plan"); err != nil {
		return "", err
	}
	v, err := p.show()
	if err != nil {
		return "", err
	}
	lines := append(summaryLines(v.Record, nil), moveLines...)
	for i, l := range lines {
		lines[i] = "- " + l
	}
	card := strings.Join(lines, "\n")
	// Route chains to Questions in the same session; the round is the last
	// thing printed, with no run-end line.
	next, err := p.questions(v.Record)
	if err != nil {
		return "", err
	}
	return "Verify is done. Here is the plan I made. Tell me if a group is wrong.\n\nRoute summary:\n\n" + card + "\n\n" + next, nil
}

// oldEpicMoves plans the old epics' moves as the skill does: each epic with
// tickets to move goes to its table area (the closest area when the table has
// none) and an epic that only holds claimed tickets is a "waiting" line. It
// returns the card's lines.
func (p *Player) oldEpicMoves(areas []string) ([]string, error) {
	out, err := p.triage("", "oldepics", "--json")
	if err != nil || out == "" {
		return nil, err
	}
	var list []struct {
		Epic    string   `json:"epic"`
		Area    string   `json:"area"`
		Move    []string `json:"move"`
		Waiting bool     `json:"waiting"`
		Claimed []string `json:"claimed"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return nil, err
	}
	var lines []string
	units := []triage.MoveUnit{}
	seen := map[string]bool{}
	for _, e := range list {
		if e.Waiting {
			lines = append(lines, fmt.Sprintf("waiting: %s holds claimed %s", e.Epic, strings.Join(e.Claimed, ", ")))
			continue
		}
		area := e.Area
		if area == "" {
			area = closestArea(areas)
		} else if !seen[area] && !slices.Contains(areas, area) {
			seen[area] = true
			if _, err := p.triage("", "area", "--name", area, "--source", triage.AreaTable); err != nil {
				return nil, err
			}
		}
		if len(e.Move) == 0 {
			continue
		}
		units = append(units, triage.MoveUnit{OldEpic: e.Epic, Area: area})
		lines = append(lines, fmt.Sprintf("move %s -> area %s: %d tickets", e.Epic, area, len(e.Move)))
	}
	if len(units) == 0 {
		return lines, nil
	}
	body, err := json.Marshal(units)
	if err != nil {
		return nil, err
	}
	_, err = p.triage(string(body), "moves")
	return lines, err
}

func bucket(r triage.Row) string {
	action, dest := r.Effective()
	switch action {
	case triage.ActionArea:
		return "area " + dest
	case "ready":
		return "ready epic"
	}
	return action
}

// summaryLines is the summary card: the rows per action and per area. With
// only, just those buckets, zero counts included.
func summaryLines(rec *triage.Record, only []string) []string {
	counts := map[string]int{}
	for _, r := range rec.Rows {
		if r.Write != triage.WriteGone {
			counts[bucket(r)]++
		}
	}
	var keys []string
	if only != nil {
		keys = slices.Compact(slices.Sorted(slices.Values(only)))
	} else {
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}
	var out []string
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s: %d", k, counts[k]))
	}
	return out
}

func titleOf(rec *triage.Record, address string) string {
	for _, r := range rec.Rows {
		if r.Address == address {
			return r.Title
		}
	}
	return address
}

func decidedText(rec *triage.Record) string {
	if len(rec.Decided) == 0 {
		return ""
	}
	lines := []string{"Decided unless you object:", ""}
	for _, d := range rec.Decided {
		lines = append(lines, fmt.Sprintf("- %s: %s -> %s (%s)", titleOf(rec, d.RowID), d.Question, d.Choice, d.Reason))
	}
	return strings.Join(lines, "\n")
}

// printRound is a saved round as the agent prints it: every question of the
// round, answered or not, so the card counts "k of n" over the whole round,
// then the decided list. The turn ends on it.
func printRound(rec *triage.Record, i int, _ []triage.Question) string {
	var cards []string
	for n, q := range rec.Rounds[i].Questions {
		cards = append(cards, questionCard(rec, n+1, q))
	}
	return fmt.Sprintf("I need your call on %d things. The rest I decided.\n\n", len(cards)) +
		strings.Join(cards, "\n\n") + "\n\n" + decidedText(rec)
}

// questionCard prints one question in the round format the chat parser turns
// into a round card. Option ids are letters; the recommended one is the
// question's default.
func questionCard(rec *triage.Record, n int, q triage.Question) string {
	block := []string{"```gx-question", "type: options"}
	var opts []string
	for i, o := range q.Options {
		id := string(rune('a' + i))
		if o == q.Default {
			block = append(block, "recommended: "+id)
		}
		opts = append(opts, fmt.Sprintf("  - id: %s\n    label: %q", id, o))
	}
	block = append(block, "options:")
	block = append(block, opts...)
	block = append(block, "```")
	// The card's bold title is the question; the hint line names the row it is about.
	return fmt.Sprintf("%s%d** - **%s**: Asked about %s\n\n%s", grillround.Marker, n, q.Text, titleOf(rec, q.RowID), strings.Join(block, "\n"))
}

// finalCard is the summary, the decided list and the one-question final round
// "Write this plan?" in the round format, so the page shows it as the final card.
func finalCard(rec *triage.Record) string {
	block := "```gx-question\ntype: options\nrecommended: a\noptions:\n  - id: a\n    label: \"Yes, write it\"\n  - id: b\n    label: \"No, keep talking\"\n```"
	answered := 0
	for _, rd := range rec.Rounds {
		answered += len(rd.Answers)
	}
	lines := summaryLines(rec, nil)
	for i, l := range lines {
		lines[i] = "- " + l
	}
	return fmt.Sprintf("All %d answered. These I decided unless you object:\n\n", answered) +
		strings.Join(lines, "\n") + "\n\n" + decidedText(rec) + "\n\n" +
		fmt.Sprintf("%s1** - **Write this plan?**: Nothing changes until you say yes.\n\n%s", grillround.Marker, block)
}

// questions plays Questions: it reprints an open round, else saves and prints
// the next one, else prints the final card.
func (p *Player) questions(rec *triage.Record) (string, error) {
	if _, err := p.gx("", "agent", "step", tickets.StepQuestions); err != nil {
		return "", err
	}
	areas := p.areas()
	if len(rec.Decided) == 0 {
		for _, d := range Decisions(rec, areas) {
			if _, err := p.triage("", "decide", "--row", d.RowID, "--question", d.Question, "--choice", d.Choice, "--reason", d.Reason); err != nil {
				return "", err
			}
		}
		v, err := p.show()
		if err != nil {
			return "", err
		}
		rec = v.Record
	}
	if i, open, ok := rec.OpenRound(); ok {
		return printRound(rec, i, open), nil
	}
	ordinary := 0
	for _, rd := range rec.Rounds {
		if !rd.Objection {
			ordinary++
		}
	}
	qs := p.nextRound(rec, areas)
	if len(qs) == 0 || ordinary >= triage.MaxOrdinaryRounds {
		return finalCard(rec), nil
	}
	body, err := json.Marshal(qs)
	if err != nil {
		return "", err
	}
	if _, err := p.triage(string(body), "round"); err != nil {
		return "", err
	}
	v, err := p.show()
	if err != nil {
		return "", err
	}
	i, open, _ := v.OpenRound()
	return printRound(v.Record, i, open), nil
}

// nextRound is the next round's questions: the new-area one first, then the
// ask questions no round holds, bugs before ideas, at most one round's worth.
func (p *Player) nextRound(rec *triage.Record, areas []string) []triage.Question {
	var qs []triage.Question
	placed := slices.ContainsFunc(rec.Rounds, func(rd triage.Round) bool {
		return slices.ContainsFunc(rd.Questions, func(q triage.Question) bool { return q.ID == NewAreaQuestionID })
	})
	unconfirmed := slices.ContainsFunc(rec.Areas, func(a triage.Area) bool { return a.Source == triage.AreaAgent && !a.Confirmed })
	if unconfirmed && !placed {
		qs = append(qs, NewAreaQuestion(rec, areas))
	}
	asks := rec.Unrounded()
	category := func(address string) int {
		for _, r := range rec.Rows {
			if r.Address == address && r.Category == "bug" {
				return 0
			}
		}
		return 1
	}
	slices.SortStableFunc(asks, func(a, b triage.Question) int { return category(a.RowID) - category(b.RowID) })
	qs = append(qs, asks...)
	if len(qs) > triage.MaxRoundQuestions {
		qs = qs[:triage.MaxRoundQuestions]
	}
	return qs
}

// roundLineRe is one answered question line of the prompt the page sends for a
// round: "Q<label> - <title>: <answer>". An unanswered one reads "(unanswered)".
var roundLineRe = regexp.MustCompile(`^Q(\d+) - .*: (.+)$`)

// noteMarker starts the round's note, which is how a person types a phrase
// while a round is open.
const noteMarker = "\n\nNote:\n"

// reactRound answers a round the page sent: each answered line, then the note
// as a typed phrase. ok is false for a message that is no round prompt.
func (p *Player) reactRound(rec *triage.Record) (string, bool, error) {
	body, note, _ := strings.Cut(strings.TrimSpace(p.Typed), noteMarker)
	var replies []string
	found := false
	for line := range strings.SplitSeq(body, "\n") {
		m := roundLineRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil || m[2] == "(unanswered)" {
			continue
		}
		found = true
		n, _ := strconv.Atoi(m[1])
		reply, err := p.answerLine(rec, n, m[2])
		if err != nil {
			return "", true, err
		}
		replies = append(replies, reply)
	}
	if !found && note == "" {
		return "", false, nil
	}
	if note != "" {
		v, err := p.show()
		if err != nil {
			return "", true, err
		}
		p.Typed = note
		reply, err := p.react(v.Record)
		if err != nil {
			return "", true, err
		}
		replies = append(replies, reply)
	}
	return strings.Join(replies, "\n\n"), true, nil
}

// answerLine answers question n of the open round with a chosen label; with no
// open round it is the final "Write this plan?" card.
func (p *Player) answerLine(rec *triage.Record, n int, label string) (string, error) {
	i, _, open := rec.OpenRound()
	if !open {
		if label == "Yes, write it" {
			return p.approve(rec)
		}
		return "Nothing is written until you say yes.", nil
	}
	qs := rec.Rounds[i].Questions
	if n < 1 || n > len(qs) {
		return fmt.Sprintf("There is no question %d.", n), nil
	}
	if rec.Rounds[i].Answers[qs[n-1].ID] != "" {
		return "", nil
	}
	return p.answer(rec, qs[n-1].ID, label)
}

// react answers a typed message.
func (p *Player) react(rec *triage.Record) (string, error) {
	if reply, ok, err := p.reactRound(rec); ok || err != nil {
		return reply, err
	}
	text := strings.TrimSpace(p.Typed)
	areas := p.areas()
	switch m := moveRe.FindStringSubmatch(text); {
	case strings.EqualFold(text, PhraseYes):
		return p.approve(rec)
	case m != nil:
		return p.move(rec, areas, m[1], m[2])
	case answerRe.MatchString(text):
		m := answerRe.FindStringSubmatch(text)
		return p.answer(rec, m[1], strings.TrimSpace(m[2]))
	case objectRe.MatchString(text):
		return p.object(rec, objectRe.FindStringSubmatch(text)[1])
	}
	return "I did not understand that. Type " + strings.Join([]string{PhraseYes, PhraseMove, PhraseAnswer, PhraseObject}, ", ") + ".", nil
}

func (p *Player) approve(rec *triage.Record) (string, error) {
	if _, err := p.triage("", "approve", "--revision", strconv.Itoa(rec.Revision)); err != nil {
		return "", err
	}
	return "✅ questions: the plan is approved. Next: " + p.Next(), nil
}

// move is a routing correction, saved right after the reply.
func (p *Player) move(rec *triage.Record, areas []string, n, to string) (string, error) {
	i, _ := strconv.Atoi(n)
	if i < 1 || i > len(rec.Rows) {
		return fmt.Sprintf("There is no row %s.", n), nil
	}
	row := rec.Rows[i-1]
	args := []string{"correct", "--row", row.Address, "--action", triage.ActionArea, "--destination", to}
	if to == "ready" {
		args = []string{"correct", "--row", row.Address, "--action", "ready", "--fallback", closestArea(areas)}
	}
	if _, err := p.triage("", args...); err != nil {
		return "", err
	}
	v, err := p.show()
	if err != nil {
		return "", err
	}
	after := v.Rows[i-1]
	lines := summaryLines(v.Record, []string{bucket(row), bucket(after)})
	reply := fmt.Sprintf("Moved %s to %s. Summary lines changed:\n%s", row.Title, to, strings.Join(lines, "\n"))
	if _, _, open := v.OpenRound(); !open && len(v.Unrounded()) > 0 {
		next, err := p.questions(v.Record)
		if err != nil {
			return "", err
		}
		reply += "\n\n" + next
	}
	return reply, nil
}

// answer saves the answer to a question of the open round. The new-area
// answer also settles the area; a round that is now answered chains to the
// next round or the final card.
func (p *Player) answer(rec *triage.Record, id, answer string) (string, error) {
	if _, err := p.triage("", "answer", "--question", id, "--answer", answer); err != nil {
		return "", err
	}
	if id == NewAreaQuestionID {
		if err := p.settleNewArea(rec, answer); err != nil {
			return "", err
		}
	}
	v, err := p.show()
	if err != nil {
		return "", err
	}
	if _, open, ok := v.OpenRound(); ok {
		return fmt.Sprintf("Saved. %d still waiting.", len(open)), nil
	}
	return p.questions(v.Record)
}

// settleNewArea acts on the new-area answer, then confirms the area entry:
// approve is refused while an agent area is unconfirmed, and Write creates
// an area only when a row goes to it.
func (p *Player) settleNewArea(rec *triage.Record, answer string) error {
	for _, r := range rec.Rows {
		if _, dest := r.Effective(); dest != NewArea {
			continue
		}
		switch {
		case strings.HasPrefix(answer, "File under "):
			if _, err := p.triage("", "correct", "--row", r.Address, "--action", triage.ActionArea, "--destination", strings.TrimPrefix(answer, "File under ")); err != nil {
				return err
			}
		case answer == "Keep for later":
			v, err := p.show()
			if err != nil {
				return err
			}
			if _, err := p.triage("", "row", "--row", r.Address, "--keep-for-later", "--revision", strconv.Itoa(v.Revision)); err != nil {
				return err
			}
		}
	}
	_, err := p.triage("", "area", "--name", NewArea, "--source", triage.AreaAgent, "--confirm")
	return err
}

// object saves the objection to the n-th decision and prints the objection
// round.
func (p *Player) object(rec *triage.Record, n string) (string, error) {
	i, _ := strconv.Atoi(n)
	if i < 1 || i > len(rec.Decided) {
		return fmt.Sprintf("There is no decision %s.", n), nil
	}
	d := rec.Decided[i-1]
	if _, err := p.triage("", "object", "--row", d.RowID, "--question", d.Question, "--option", d.Choice, "--option", "something else"); err != nil {
		return "", err
	}
	v, err := p.show()
	if err != nil {
		return "", err
	}
	idx, open, _ := v.OpenRound()
	return printRound(v.Record, idx, open), nil
}
