package notify

import (
	"github.com/elentok/gx/ralphloop/ticketops"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/elentok/gx/ralphloop/runlog"
	"github.com/elentok/gx/ralphloop/schedule"
	"github.com/elentok/gx/tickets"
)

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	cases := map[int]string{
		0:    "0s",
		42:   "42s",
		59:   "59s",
		60:   "1m0s",
		332:  "5m32s",
		3600: "1h0m0s",
		3725: "1h2m5s",
	}
	for in, want := range cases {
		if got := formatDuration(in); got != want {
			t.Errorf("formatDuration(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatTokens(t *testing.T) {
	t.Parallel()
	cases := map[int]string{
		0:      "0 tok",
		500:    "500 tok",
		999:    "999 tok",
		1000:   "1k tok",
		39000:  "39k tok",
		119013: "119k tok",
		1499:   "1k tok",
		1500:   "2k tok",
	}
	for in, want := range cases {
		if got := formatTokens(in); got != want {
			t.Errorf("formatTokens(%d) = %q, want %q", in, got, want)
		}
	}
}

// The shared "message" layout constructor itself moved into chatmarkup (see
// chatmarkup_test.go's TestStyleMessage/TestStyleMessageOmitsEmptyLines) —
// mrkdwnStyle no longer has its own copy to test directly.

// TestIdentityLine_NeverCarriesTheIterationLabel documents the prohibition
// directly: identityLine's signature has no label parameter at all, so
// there is no way to plug one in — this test just pins what it does accept
// (epic alone, or epic and ticket).
func TestIdentityLine_EpicAlone(t *testing.T) {
	t.Parallel()
	got := slackStyle.identityLine("my-epic", "")
	want := "[gx] my-epic"
	if got != want {
		t.Errorf("identityLine = %q, want %q", got, want)
	}
}

func TestIdentityLine_EpicAndTicket(t *testing.T) {
	t.Parallel()
	got := slackStyle.identityLine("my-epic", "04")
	want := "[gx] my-epic/04"
	if got != want {
		t.Errorf("identityLine = %q, want %q", got, want)
	}
}

func TestTelegramStyleIterationFinishedText_EscapesAndFormats(t *testing.T) {
	t.Parallel()
	ticket := tickets.Ticket{Identifier: "02", Title: "Migrate ui"}
	stats := runlog.IterationStats{ElapsedSeconds: 332, PeakContextTokens: 39000, Cost: 1.234, Completed: 2, Total: 5}

	got := telegramStyle.iterationFinishedText(ticket, "ui-tree-migration", stats).String()
	want := "✅ *Migrate ui*\n\n5m32s · 39k tok · $1\\.23 · 2 done · 5 total\n\\[gx\\] ui\\-tree\\-migration/02"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestSlackStyleIterationFinishedText_NoEscaping(t *testing.T) {
	t.Parallel()
	ticket := tickets.Ticket{Identifier: "02", Title: "Migrate ui"}
	stats := runlog.IterationStats{ElapsedSeconds: 332, PeakContextTokens: 39000, Cost: 1.234, Completed: 2, Total: 5}

	got := slackStyle.iterationFinishedText(ticket, "ui-tree-migration", stats).String()
	want := "✅ *Migrate ui*\n\n5m32s · 39k tok · $1.23 · 2 done · 5 total\n[gx] ui-tree-migration/02"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTicketPauseMessage_AnswerLayout(t *testing.T) {
	t.Parallel()
	msg := ticketPauseMessage("gx", "ui-tree-migration", "", "04", ticketops.PauseStatus("zero-commit"), "No commits merged; marked paused.")
	got := msg.Text(telegramStyle.chatStyle).String()
	want := "🙋 Answer ui\\-tree\\-migration/04\nNo commits merged; marked paused\\.\ngx"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTicketPauseMessage_RepairUsesDifferentWordAndIcon(t *testing.T) {
	t.Parallel()
	msg := ticketPauseMessage("gx", "ui-tree-migration", "", "04", ticketops.PauseStatus("spinning"), "Codex is waiting")
	got := msg.Text(slackStyle.chatStyle).String()
	want := "🛑 Repair ui-tree-migration/04\nCodex is waiting\ngx"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTicketPauseMessage_TitleComesFromTicketFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "04-fix-the-thing.md")
	if err := os.WriteFile(path, []byte("---\nid: 04\n---\n\n# Fix the thing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ticketPauseMessage("gx", "e", path, "04", ticketops.PauseStatus("zero-commit"), "d").Headline()
	if want := "🙋 Answer e/04 Fix the thing"; got != want {
		t.Errorf("headline = %q, want %q", got, want)
	}
}

func TestTelegramStyle_OverflowPausedIdentifiersEscapesPlus(t *testing.T) {
	t.Parallel()
	counts := schedule.EpicCounts{Done: 2, PausedIdentifiers: []string{"01", "02", "03", "04", "05", "06"}, Total: 8}
	const countsLine = "2 done · 6 paused: 01, 02, 03, 04, 05, \\+1 more · 8 total"

	t.Run("epicOutcomeText", func(t *testing.T) {
		t.Parallel()
		got := telegramStyle.epicOutcomeText("\U0001f389", "epic complete", "ui-tree-migration", counts, 5, 492, 12.5).String()
		want := "\U0001f389 *epic complete*\n\n" + countsLine + "\n5 ticket\\(s\\) merged in 8m12s · $12\\.50\n\\[gx\\] ui\\-tree\\-migration"
		if got != want {
			t.Errorf("text = %q, want %q", got, want)
		}
	})
}

func TestEpicOutcomeText_CountsRecoveredPausesOnlyWhenThereAre(t *testing.T) {
	if got := slackStyle.epicOutcomeText("g", "t", "e", schedule.EpicCounts{}, 1, 10, 0).String(); strings.Contains(got, "recovered") {
		t.Errorf("no recoveries but text says so: %s", got)
	}
	if got := slackStyle.epicOutcomeText("g", "t", "e", schedule.EpicCounts{Recovered: 2}, 1, 10, 0).String(); !strings.Contains(got, "2 recovered automatically") {
		t.Errorf("text lacks the recovered count: %s", got)
	}
}
