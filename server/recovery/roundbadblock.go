package recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/elentok/gx/grillround"
	"github.com/elentok/gx/server/sessions/chat"
	"github.com/elentok/gx/server/ticketwrite"
	"github.com/elentok/gx/tickets/schema"
)

const (
	badBlockTitle = "Design round has an unreadable gx-question block"
	badBlockSlug  = "design-round-bad-block"
	badBlockKey   = badBlockSlug
)

// badRoundBlock is one question of a round whose gx-question block cannot be
// read.
type badRoundBlock struct {
	Label string
	Err   error
	Raw   string
}

// badRoundBlocks lists the questions of a round with an unreadable block. A
// question with no block is plain text, not a bad block.
func badRoundBlocks(text string) []badRoundBlock {
	var bad []badRoundBlock
	for i, seg := range grillround.Segments(text) {
		if p := chat.ParseRoundSegment(i+1, seg); p.Err != nil {
			bad = append(bad, badRoundBlock{p.Card.Label, p.Err, p.Raw})
		}
	}
	return bad
}

// FileBadRoundBlocks files the unreadable blocks of a round into the one open
// follow-up that collects them. A round already filed adds nothing. A missing
// follow-ups project or a failed write only logs a warning.
func (rec *Recovery) FileBadRoundBlocks(id string, design chat.DesignRef, text string) {
	bad := badRoundBlocks(text)
	if len(bad) == 0 {
		return
	}
	sum := sha256.Sum256([]byte(text))
	round := "Round: " + hex.EncodeToString(sum[:6])
	path, err := rec.writeBadBlockFollowUp(id, design, round, bad)
	switch {
	case errors.Is(err, errNoFollowUpProject):
		rec.d.Log.Warn("follow-ups project missing, unreadable gx-question block not filed", "session", id, "err", err)
	case err != nil:
		rec.d.Log.Warn("file unreadable gx-question block", "session", id, "err", err)
	case path != "":
		rec.d.Log.Info("filed unreadable gx-question block", "session", id, "path", path)
	}
}

// writeBadBlockFollowUp adds the round to the open draft carrying badBlockKey,
// or files a new one. It returns "" when the round is already filed.
func (rec *Recovery) writeBadBlockFollowUp(id string, design chat.DesignRef, round string, bad []badRoundBlock) (string, error) {
	epicPath, err := rec.FollowUpsEpicPath("")
	if err != nil {
		return "", err
	}
	if err := EnsureDraftEpic(epicPath); err != nil {
		return "", err
	}
	if _, ok := rec.filedRounds.Load(round); ok {
		return "", nil
	}
	if filed, err := roundFiled(epicPath, round); filed || err != nil {
		if filed {
			rec.filedRounds.Store(round, struct{}{})
		}
		return "", err
	}
	var ev strings.Builder
	fmt.Fprintf(&ev, "%s, session %s, Design %s. %s\n", time.Now().Format("2006-01-02"), id, design.Ticket, round)
	for _, b := range bad {
		fmt.Fprintf(&ev, "\nQ%s: %v\n\n```gx-question\n%s\n```\n", b.Label, b.Err, b.Raw)
	}
	if path, found, err := noteOccurrence(epicPath, badBlockKey, "Seen again: "+ev.String()); found || err != nil {
		if err == nil {
			rec.filedRounds.Store(round, struct{}{})
		}
		return path, err
	}
	path, _, err := ticketwrite.WriteNewTicket(epicPath, "", badBlockSlug, func(tid string) (schema.Ticket, string) {
		body := fmt.Sprintf(
			"\n# %s — %s\n\n## Source\n\nThe transcript watcher, on a Design session that went Idle after a grilling round.\n\n## Problem\n\nA gx-question block in a grilling round does not parse, so its question shows without options.\n\n## Evidence\n\n%s\n\n%s\n## Suggested fix\n\nFix the gx-design skill's block format, or the parser if the block is valid.\n",
			tid, badBlockTitle, dedupeLine(badBlockKey), ev.String(),
		)
		return schema.Ticket{ID: schema.TicketID(tid), Status: schema.StatusDraft, Type: schema.TypeResearch, Category: schema.CategoryBug}, body
	})
	if err == nil {
		rec.filedRounds.Store(round, struct{}{})
	}
	return path, err
}

// roundFiled reports whether any ticket in the follow-ups epic already holds
// the round's marker.
func roundFiled(epicPath, round string) (bool, error) {
	files, err := filepath.Glob(filepath.Join(epicPath, "issues", "*.md"))
	if err != nil {
		return false, err
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err == nil && strings.Contains(string(raw), round) {
			return true, nil
		}
	}
	return false, nil
}
