package notify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/chatmarkup"
	"github.com/elentok/gx/ralphloop/schedule"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/schema"
)

// TestMarkdownV2Validity_AllNotificationTexts drives every chatmarkup-backed
// *Text function in notification_text.go with telegramStyle, through
// adversarial input a live Telegram sendMessage call would reject if any
// call site forgot to escape it: a punctuation-heavy title (parens, a
// hyphen, a period, a literal asterisk), a fractional cost, and an
// schedule.EpicCounts with more than pausedIdentifierCap PausedIdentifiers (the "+N
// more" overflow case — see TestMarkdownV2Validity_OverflowPausedIdentifiers
// for a dedicated tripwire). Each case is checked against
// isValidTelegramMarkdownV2's own independently-written special-character
// list, not production's, so a bug in the shared list can't hide from it.
func TestMarkdownV2Validity_AllNotificationTexts(t *testing.T) {
	t.Parallel()

	const adversarialTitle = `Fix (parens) - literal *asterisk* and a period.`
	const adversarialEpic = "ui-tree.migration (v2)"
	const adversarialTicket = "04"
	const fractionalCost = 12.345

	adversarialPath := filepath.Join(t.TempDir(), "04-x.md")
	if err := os.WriteFile(adversarialPath, []byte("# "+adversarialTitle+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	overflowCounts := schedule.EpicCounts{
		Done:              2,
		PausedIdentifiers: []string{"01", "02", "03", "04", "05", "06"},
		Total:             10,
	}
	cases := map[string]chatmarkup.Text{
		"ticketPauseMessage/answer": ticketPauseMessage(adversarialEpic, adversarialEpic, adversarialPath, adversarialTicket, string(schema.StatusLegacyAnswer), "No commits merged (yet).").Text(chatmarkup.Telegram),
		"ticketPauseMessage/repair": ticketPauseMessage(adversarialEpic, adversarialEpic, adversarialPath, adversarialTicket, string(schema.StatusLegacyRepair), "Operator intervention required.").Text(chatmarkup.Telegram),
		"conflictStartedMessage":    conflictStartedMessage(adversarialEpic, adversarialEpic, adversarialPath, adversarialTicket).Text(chatmarkup.Telegram),
		"escalatedMessage/ticket":   escalatedMessage(adversarialEpic, adversarialEpic, adversarialPath, adversarialTicket, "No commits merged (yet).").Text(chatmarkup.Telegram),
		"escalatedMessage/epic":     escalatedMessage(adversarialEpic, adversarialEpic, "", "", "").Text(chatmarkup.Telegram),
		"epicOutcomeMessage":        epicOutcomeMessage(adversarialEpic, adversarialEpic, overflowCounts, 492, fractionalCost, true).Text(chatmarkup.Telegram),
		"progressMessage": progressMessage(adversarialEpic, adversarialEpic, ProgressFacts{
			Counts:  schedule.EpicCounts{Done: 3, InProgress: 1, Total: 12},
			Running: []ProgressRunning{{Identifier: adversarialTicket, Title: adversarialTitle, ElapsedSeconds: 840}}, FinishedCount: 2, FinishedCost: fractionalCost,
		}).Text(chatmarkup.Telegram),
		"mutedMessage":         mutedMessage(adversarialPath, adversarialTicket).Text(chatmarkup.Telegram),
		"globallyMutedMessage": globallyMutedMessage("telegram").Text(chatmarkup.Telegram),
		"testMessageText":      telegramStyle.testMessageText(),
	}

	for name, text := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got := text.String()
			if !isValidTelegramMarkdownV2(got) {
				t.Errorf("%s output %q is not valid MarkdownV2", name, got)
			}
		})
	}
}

// TestMarkdownV2Validity_OverflowPausedIdentifiers is the named tripwire for
// the >5-paused-identifiers regression (ticket 01b): a future change to
// RenderCountsLine or to who escapes what should fail this test even though
// nothing else in this file is "about" paused overflow specifically.
func TestMarkdownV2Validity_OverflowPausedIdentifiers(t *testing.T) {
	t.Parallel()

	counts := schedule.EpicCounts{
		Done:              2,
		PausedIdentifiers: []string{"01", "02", "03", "04", "05", "06", "07"},
		Total:             10,
	}

	got := epicOutcomeMessage("gx", "ui-tree-migration", counts, 492, 12.5, true).Text(chatmarkup.Telegram).String()
	if !isValidTelegramMarkdownV2(got) {
		t.Fatalf("epicOutcomeMessage output %q is not valid MarkdownV2", got)
	}
	if !strings.Contains(got, `\+2 more`) {
		t.Errorf("expected escaped overflow marker in %q", got)
	}
}

// TestMarkdownV2Validity_RenderBatchWithDedupSuffix covers renderBatch, the
// non-*Text function whose output can also reach sendSync — the ×N dedup
// suffix branch is where the second of the three past incidents actually
// happened (see chat_eventsink.go's renderBatch doc), so this exercises it
// directly rather than trusting the *Text-only test above to catch it.
func TestMarkdownV2Validity_RenderBatchWithDedupSuffix(t *testing.T) {
	t.Parallel()

	epicName := "ui-tree.migration (v2)"
	ticket1 := tickets.Ticket{Identifier: "01", Title: "Fix (parens) - literal *asterisk*."}
	ticket2 := tickets.Ticket{Identifier: "02", Title: "Second item: cost $1.23!"}
	muted1 := mutedMessage(filepath.Join(epicName, "issues", ticket1.Identifier+"-x.md"), ticket1.Identifier)
	muted2 := mutedMessage(filepath.Join(epicName, "issues", ticket2.Identifier+"-x.md"), ticket2.Identifier)
	items := []batchedMessage{
		{text: muted1.Text(chatmarkup.Telegram), kind: notifyKindMuted, count: 1, msg: &muted1},
		{text: muted2.Text(chatmarkup.Telegram), kind: notifyKindMuted, count: 3, msg: &muted2},
	}

	got := renderBatch(telegramStyle, items).String()
	if !isValidTelegramMarkdownV2(got) {
		t.Errorf("renderBatch output %q is not valid MarkdownV2", got)
	}
	if !strings.Contains(got, `×3`) {
		t.Errorf("expected dedup suffix ×3 in %q", got)
	}
}
