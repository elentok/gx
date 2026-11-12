package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/elentok/gx/agentrunner/claudeui"
	"github.com/elentok/gx/nativerunner/holder"
	"github.com/elentok/gx/server/wire"
)

// DefaultDialogFallbackDelay is how long a session must wait on "input needed"
// with no hook question before the screen is read: the hook normally registers
// its question within this time.
const DefaultDialogFallbackDelay = 2 * time.Second

// claudeInputNeeded is the waitingFor of a question dialog. A slash picker
// reports "dialog open" and permission and plan prompts "permission prompt".
const claudeInputNeeded = "input needed"

// dialogWaits remembers since when each session has waited on "input needed".
type dialogWaits struct {
	mu    sync.Mutex
	since map[string]time.Time
}

// waited reports whether the session has been waiting for at least delay,
// starting the clock at the first call; clear stops it.
func (w *dialogWaits) waited(id string, delay time.Duration) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.since == nil {
		w.since = map[string]time.Time{}
	}
	t, ok := w.since[id]
	if !ok {
		t = time.Now()
		w.since[id] = t
	}
	return time.Since(t) >= delay
}

func (w *dialogWaits) clear(id string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.since, id)
}

// claudeFallbackCards is the keystroke-fallback card of a question dialog the
// hook did not register: the session waits on "input needed", no hook holds a
// question, the dialog was not handed back to the terminal and its screen
// parses.
func (c *Chat) claudeFallbackCards(dir string, m Meta, snap Snapshot) []wire.QuestionCard {
	if m.Status.State != wire.SessionBlocked || m.Status.Kind != "question" || m.Status.Message != claudeInputNeeded {
		c.dialogWaits.clear(m.ID)
		return nil
	}
	if !c.dialogWaits.waited(m.ID, c.cfg.DialogFallbackDelay) {
		return nil
	}
	conn, err := holder.DialHolder(dir)
	if err != nil {
		return nil
	}
	rows, _, err := conn.Screen(0)
	if err != nil {
		c.log.Warn("read screen for claude dialog card", "session", m.ID, "err", err)
		return nil
	}
	d, ok := claudeui.ParseAskDialog(rows)
	if !ok || c.src.AskReleased(m.ID, d.Question) {
		return nil
	}
	return ClaudeFallbackCard(d, personTurns(snap.Msgs))
}

// ClaudeFallbackCard is the dialog on screen as a claude-dialog card with no
// note field. Its id holds the dialog id, the turn and a hash of the whole
// block, so the same dialog on a later turn, or after an answer changed the
// block, is a new card. An option's id is its 1-based position.
func ClaudeFallbackCard(d claudeui.AskDialog, turn int) []wire.QuestionCard {
	q := wire.QuestionCardQuestion{Kind: "options", Title: d.Question}
	if d.Review {
		q.Body = reviewBody(d.ReviewItems)
	}
	if d.Multi {
		q.Kind = "multi-select"
	}
	hashed := []string{d.Question}
	for _, t := range d.Tabs {
		hashed = append(hashed, t.Label, strconv.FormatBool(t.Answered))
	}
	for i, o := range d.Options {
		opt := wire.QuestionCardOption{ID: strconv.Itoa(i + 1), Label: o.Label, Description: o.Description, Text: o.Other}
		if o.Other {
			opt.Label, opt.ID = "Other", "other"
		}
		q.Options = append(q.Options, opt)
		hashed = append(hashed, o.Label, o.Description, strconv.FormatBool(o.Checked))
	}
	for _, it := range d.ReviewItems {
		hashed = append(hashed, it.Question, it.Answer)
	}
	sum := sha256.Sum256([]byte(strings.Join(hashed, "\x00")))
	return []wire.QuestionCard{{
		Source:    wire.CardSourceClaudeDialog,
		ID:        fmt.Sprintf("%s-%d-%s", d.ID, turn, hex.EncodeToString(sum[:6])),
		Questions: []wire.QuestionCardQuestion{q},
		Fallback:  true,
	}}
}

// reviewBody lists the answers of the review screen, one per line.
func reviewBody(items []claudeui.AskReviewItem) string {
	lines := make([]string, len(items))
	for i, it := range items {
		lines[i] = "- " + it.Question + " → " + it.Answer
	}
	return strings.Join(lines, "\n")
}
