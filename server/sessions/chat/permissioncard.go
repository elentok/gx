package chat

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/elentok/gx/agentrunner/claudeui"
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
	return BuildCards(class, src)
}

// permissionCards is the card of the permission dialog a Blocked Claude session
// shows, or none when it is not blocked on one or the screen does not parse.
func (c *Chat) permissionCards(dir string, m Meta, snap Snapshot) []wire.QuestionCard {
	if m.Status.State != wire.SessionBlocked || m.Status.Kind != "permission" || m.Harness == string(harness.Codex) {
		return nil
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
	return PermissionCard(rows, personTurns(snap.Msgs))
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

// PermissionCard is the permission dialog on rows as one single-select card
// with every option and no preselection, or no cards. Nothing is marked
// recommended: a standing grant such as "always allow" gets no hint. The id
// holds the dialog id, the turn and a hash of the whole block, so the same
// dialog on a later turn is a new card.
func PermissionCard(rows []string, turn int) []wire.QuestionCard {
	v := claudeui.ReadDialog(rows)
	if v.Kind != claudeui.DialogPermission {
		return nil
	}
	sum := sha256.Sum256([]byte(strings.Join(append([]string{v.Header, v.Body, v.Question}, v.Options...), "\n")))
	q := wire.QuestionCardQuestion{
		Kind: "options", Title: v.Question, Body: joinDialogText(v.Header, v.Body),
	}
	for i, o := range v.Options {
		q.Options = append(q.Options, wire.QuestionCardOption{ID: strconv.Itoa(i + 1), Label: o})
	}
	return []wire.QuestionCard{{
		Source:    wire.CardSourcePermission,
		ID:        fmt.Sprintf("%s-%d-%s", v.ID, turn, hex.EncodeToString(sum[:6])),
		Questions: []wire.QuestionCardQuestion{q},
	}}
}
