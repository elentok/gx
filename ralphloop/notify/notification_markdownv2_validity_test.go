package notify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/chatmarkup"
	"github.com/elentok/gx/ralphloop/runlog"
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
	ticket := tickets.Ticket{Identifier: adversarialTicket, Title: adversarialTitle}
	stats := runlog.IterationStats{ElapsedSeconds: 332, PeakContextTokens: 39000, Cost: fractionalCost, Completed: 2, Total: 5}

	cases := map[string]chatmarkup.Text{
		"iterationFinishedText":            telegramStyle.iterationFinishedText(ticket, adversarialEpic, stats),
		"ticketPauseMessage/answer":        ticketPauseMessage(adversarialEpic, adversarialEpic, adversarialPath, adversarialTicket, string(schema.StatusLegacyAnswer), "No commits merged (yet).").Text(chatmarkup.Telegram),
		"ticketPauseMessage/repair":        ticketPauseMessage(adversarialEpic, adversarialEpic, adversarialPath, adversarialTicket, string(schema.StatusLegacyRepair), "Operator intervention required.").Text(chatmarkup.Telegram),
		"epicOutcomeText":                  telegramStyle.epicOutcomeText("\U0001f389", "epic complete", adversarialEpic, overflowCounts, 5, 492, fractionalCost),
		"mutedText":                        telegramStyle.mutedText(adversarialEpic, adversarialTicket),
		"globallyMutedText":                telegramStyle.globallyMutedText("telegram"),
		"testMessageText":                  telegramStyle.testMessageText(),
		"identityLine/punctuation-bearing": telegramStyle.iterationFinishedText(tickets.Ticket{Identifier: "04 (retry)", Title: "x"}, "ui-tree.migration (v2)", stats),
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

	got := telegramStyle.epicOutcomeText("\U0001f389", "epic complete", "ui-tree-migration", counts, 5, 492, 12.5).String()
	if !isValidTelegramMarkdownV2(got) {
		t.Fatalf("epicOutcomeText output %q is not valid MarkdownV2", got)
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
	items := []batchedMessage{
		{text: telegramStyle.mutedText(epicName, ticket1.Identifier), kind: notifyKindMuted, count: 1},
		{
			text: telegramStyle.iterationFinishedText(ticket2, epicName, runlog.IterationStats{
				ElapsedSeconds: 65, PeakContextTokens: 1500, Cost: 3.456, Completed: 1, Total: 4,
			}),
			kind:  "iteration-finished",
			count: 3,
		},
	}

	got := renderBatch(telegramStyle, items).String()
	if !isValidTelegramMarkdownV2(got) {
		t.Errorf("renderBatch output %q is not valid MarkdownV2", got)
	}
	if !strings.Contains(got, `×3`) {
		t.Errorf("expected dedup suffix ×3 in %q", got)
	}
}
