package chat

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/elentok/gx/agentrunner"
	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/codexsession"
	"github.com/elentok/gx/harness"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/transcript"
)

// Cards is the questions list of a session's chat response: the cards of every
// slot the session's class shows. dir is the session's state dir, read for a
// live blocker.
func (c *Chat) Cards(dir string, m Meta, snap Snapshot) []wire.QuestionCard {
	class := ClassOf(m)
	src := CardSources{SlotRound: RoundCard(snap.Round)}
	if slices.Contains(classSlots[class], SlotLiveBlocker) {
		src[SlotLiveBlocker] = c.permissionCards(dir, m, snap)
	}
	if addr := askAddress(m); addr != "" && slices.Contains(classSlots[class], SlotTicketAsk) {
		if q, state, ok := c.src.TicketAsk(addr); ok {
			src[SlotTicketAsk] = TicketAskCard(addr, q, state)
		}
	}
	if class == ClassPlain && m.Ticket == "" && m.Harness == string(harness.Codex) {
		src[SlotCodexQuestion] = c.codexQuestionCards(m)
	}
	if class == ClassPlain && m.Ticket == "" && m.Harness == string(harness.Claude) {
		if held := c.src.HeldAsks(m.ID); len(held) > 0 {
			src[SlotClaudeDialog] = ClaudeDialogCard(held[0])
		} else {
			src[SlotClaudeDialog] = c.claudeFallbackCards(dir, m, snap)
		}
	}
	return BuildCards(class, src)
}

type codexQuestionRead struct {
	q  codexsession.OpenQuestion
	ok bool
}

// codexQuestionCards is the card of the open request_user_input call in the
// session's rollout, listed as soon as the call is on disk.
func (c *Chat) codexQuestionCards(m Meta) []wire.QuestionCard {
	path, ok := c.codexRollout(m.ID, m)
	if !ok {
		return nil
	}
	r, err := c.codexQuestions.Read(path, func(p string) (codexQuestionRead, error) {
		q, ok, err := codexsession.ReadOpenQuestion(p)
		return codexQuestionRead{q, ok}, err
	})
	if err != nil {
		c.log.Warn("read codex question", "session", m.ID, "err", err)
		return nil
	}
	if !r.ok {
		return nil
	}
	return CodexQuestionCard(r.q)
}

// permissionCards is the card of the permission dialog a Blocked Claude session
// shows, or none when it is not blocked on one or the screen does not parse.
func (c *Chat) permissionCards(dir string, m Meta, snap Snapshot) []wire.QuestionCard {
	if m.Status.State != wire.SessionBlocked || m.Status.Kind != "permission" {
		return nil
	}
	if m.Harness == string(harness.Codex) {
		return c.codexPermissionCards(dir, m)
	}
	conn, err := holder.DialHolder(dir)
	if err != nil {
		return nil
	}
	rows, _, err := conn.Screen(0)
	if err != nil {
		c.log.Warn("read screen for permission card", "session", m.ID, "err", err)
		return nil
	}
	return PermissionCard(rows, personTurns(snap.Msgs), harness.LookupEnv(c.cfg.Env, "HOME"))
}

// codexPermissionCards is the Allow and Deny card of an approval the
// PermissionRequest hook caught. A screen-only approval gets none: the hook
// request is the only exact source, and its id is the card id.
func (c *Chat) codexPermissionCards(dir string, m Meta) []wire.QuestionCard {
	req, hooked, err := holder.ReadPermissionRequest(dir)
	if err != nil {
		c.log.Warn("read hook request for permission card", "session", m.ID, "err", err)
		return nil
	}
	if !hooked {
		return nil
	}
	return []wire.QuestionCard{{
		Source: wire.CardSourcePermission,
		ID:     req.ID,
		Questions: []wire.QuestionCardQuestion{{
			Kind: "options", Title: "Allow this?", Body: cmp.Or(req.Description, req.Tool),
			Options: []wire.QuestionCardOption{
				{ID: string(agentrunner.DecisionAllow), Label: "Allow"},
				{ID: string(agentrunner.DecisionDeny), Label: "Deny"},
			},
		}},
	}}
}

func personTurns(msgs []wire.SessionChatMessage) int {
	n := 0
	for _, m := range msgs {
		if m.Role == transcript.ChatRolePerson {
			n++
		}
	}
	return n
}

// TextRowLabel is the label of a plan dialog's row that takes typed feedback.
const TextRowLabel = "Tell Claude what to change"

// PermissionCard is the permission or plan dialog on rows as one single-select
// card with every option and no preselection, or no cards. Nothing is marked
// recommended: a standing grant such as "always allow" gets no hint. A
// permission id holds the dialog id, the turn and a hash of the whole block, so
// the same dialog on a later turn is a new card. A plan card carries the plan
// file's text and its id is the file name plus a hash of the block, so a new
// plan is a new card; its option labelled TextRowLabel is a text row. home
// resolves the "~" of the plan file path.
func PermissionCard(rows []string, turn int, home string) []wire.QuestionCard {
	v := claudeui.ReadDialog(rows)
	if v.Kind == claudeui.DialogUnreadable {
		return nil
	}
	q := wire.QuestionCardQuestion{
		Kind: "options", Title: v.Question, Body: joinDialogText(v.Header, v.Body),
	}
	for i, o := range v.Options {
		q.Options = append(q.Options, wire.QuestionCardOption{ID: strconv.Itoa(i + 1), Label: o, Text: v.Kind == claudeui.DialogPlan && o == TextRowLabel})
	}
	hashed := append([]string{v.Header, v.Body, v.Question}, v.Options...)
	id := fmt.Sprintf("%s-%d-", v.ID, turn)
	if v.Kind == claudeui.DialogPlan {
		path := claudeui.PlanFilePath(rows)
		q.Body = readPlanFile(path, home)
		hashed = append(hashed, q.Body, planBlock(rows))
		id = planFileName(path) + "-"
	}
	sum := sha256.Sum256([]byte(strings.Join(hashed, "\n")))
	return []wire.QuestionCard{{
		Source:    wire.CardSourcePermission,
		ID:        id + hex.EncodeToString(sum[:6]),
		Questions: []wire.QuestionCardQuestion{q},
	}}
}

// planBlock is the screen text from the "Ready to code?" title down, which
// holds the part of the plan the dialog shows.
func planBlock(rows []string) string {
	for i, r := range rows {
		if strings.TrimSpace(r) == "Ready to code?" {
			rows = rows[i:]
			break
		}
	}
	return strings.Join(rows, "\n")
}

func planFileName(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".md")
}

// readPlanFile is the text of the plan file a dialog names, or "" when the
// path is not a Claude plan file or cannot be read.
func readPlanFile(path, home string) string {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		path = filepath.Join(home, rest)
	}
	if !filepath.IsAbs(path) || !strings.HasSuffix(path, ".md") || !strings.Contains(path, "/.claude/plans/") {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
