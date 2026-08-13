package ralphloop

import (
	"strings"
	"testing"

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

// TestMessage_DocumentedLayout pins the shared constructor's layout: emoji
// + bold headline, blank line, optional counts, optional detail, identity
// last.
func TestMessage_DocumentedLayout(t *testing.T) {
	t.Parallel()
	got := slackStyle.message("🚀", "headline", "counts line", "detail line", "identity line")
	want := "🚀 *headline*\n\ncounts line\ndetail line\nidentity line"
	if got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestMessage_OmitsBlankCountsAndDetailLines(t *testing.T) {
	t.Parallel()
	got := slackStyle.message("🚀", "headline", "", "", "identity line")
	want := "🚀 *headline*\n\nidentity line"
	if got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

func TestMessage_EscapesHeadlineButNotPreEscapedFields(t *testing.T) {
	t.Parallel()
	got := telegramStyle.message("🚀", "a-b", "1-2", "", "3-4")
	want := "🚀 *a\\-b*\n\n1-2\n3-4"
	if got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

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

func TestTelegramStyleIterationStartedText_IdentityLineLast(t *testing.T) {
	t.Parallel()
	ticket := tickets.Ticket{Identifier: "02", Title: "Migrate ui"}

	got := telegramStyle.iterationStartedText(ticket, "ui-tree-migration")
	want := "▶️ *Migrate ui*\n\n\\[gx\\] ui\\-tree\\-migration/02"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, "ui\\-tree\\-migration/02") {
		t.Errorf("text %q must end with the identity line", got)
	}
}

func TestTelegramStyleIterationFinishedText_EscapesAndFormats(t *testing.T) {
	t.Parallel()
	ticket := tickets.Ticket{Identifier: "02", Title: "Migrate ui"}
	stats := IterationStats{ElapsedSeconds: 332, PeakContextTokens: 39000, Cost: 1.234, Completed: 2, Total: 5}

	got := telegramStyle.iterationFinishedText(ticket, "ui-tree-migration", stats)
	want := "✅ *Migrate ui*\n\n5m32s · 39k tok · $1.23 · 2 done · 5 total\n\\[gx\\] ui\\-tree\\-migration/02"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestSlackStyleIterationFinishedText_NoEscaping(t *testing.T) {
	t.Parallel()
	ticket := tickets.Ticket{Identifier: "02", Title: "Migrate ui"}
	stats := IterationStats{ElapsedSeconds: 332, PeakContextTokens: 39000, Cost: 1.234, Completed: 2, Total: 5}

	got := slackStyle.iterationFinishedText(ticket, "ui-tree-migration", stats)
	want := "✅ *Migrate ui*\n\n5m32s · 39k tok · $1.23 · 2 done · 5 total\n[gx] ui-tree-migration/02"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTelegramStyleIterationPausedText_LabelInDetailNotIdentity(t *testing.T) {
	t.Parallel()
	got := telegramStyle.iterationPausedText("iter-04", "rate limit hit", "ui-tree-migration", "04")
	want := "⏸ *paused*\n\niter\\-04: rate limit hit\n\\[gx\\] ui\\-tree\\-migration/04"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if strings.HasSuffix(got, "iter\\-04") {
		t.Errorf("text %q must not end with the label", got)
	}
}

func TestTelegramStyleIterationResumedText_LabelInDetailNotIdentity(t *testing.T) {
	t.Parallel()
	got := telegramStyle.iterationResumedText("iter-04", "ui-tree-migration", "04")
	want := "▶️ *resumed*\n\niter\\-04\n\\[gx\\] ui\\-tree\\-migration/04"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTelegramStyleTicketNeedsHumanText_DistinctFromIterationPausedText(t *testing.T) {
	t.Parallel()
	got := telegramStyle.ticketNeedsHumanText("04", "ui-tree-migration", "needs-answer", "No commits landed; marked needs-answer.", EpicCounts{Done: 3, Total: 5})
	want := "\U0001f198 *needs answer*\n\nNo commits landed; marked needs\\-answer\\.\n3 done · 5 total\n\\[gx\\] ui\\-tree\\-migration/04"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if paused := telegramStyle.iterationPausedText("ui-tree-migration/04", "stuck", "ui-tree-migration", "04"); got == paused {
		t.Errorf("ticketNeedsHumanText matched iterationPausedText: %q", got)
	}
}

func TestTelegramStyleTicketNeedsHumanText_NeedsRepairUsesDifferentLabel(t *testing.T) {
	t.Parallel()
	got := telegramStyle.ticketNeedsHumanText("04", "ui-tree-migration", "needs-repair", "Codex is waiting for operator intervention", EpicCounts{Done: 3, Total: 5})
	want := "\U0001f6d1 *needs repair*\n\nCodex is waiting for operator intervention\n3 done · 5 total\n\\[gx\\] ui\\-tree\\-migration/04"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestSlackStyleTicketNeedsHumanText_NoEscaping(t *testing.T) {
	t.Parallel()
	got := slackStyle.ticketNeedsHumanText("04", "ui-tree-migration", "needs-answer", "No commits landed; marked needs-answer.", EpicCounts{Done: 3, Total: 5})
	want := "\U0001f198 *needs answer*\n\nNo commits landed; marked needs-answer.\n3 done · 5 total\n[gx] ui-tree-migration/04"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTelegramStyleEpicStartedText(t *testing.T) {
	t.Parallel()
	got := telegramStyle.epicStartedText("ui-tree-migration", EpicCounts{Done: 2, Total: 5})
	want := "\U0001f680 *epic started*\n\n2 done · 5 total\n\\[gx\\] ui\\-tree\\-migration"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTelegramStyleEpicParkedText(t *testing.T) {
	t.Parallel()
	got := telegramStyle.epicParkedText("ui-tree-migration", []string{"04", "07"})
	want := "\U0001f17f️ *epic parked*\n\nNothing runnable left; waiting on 04, 07\n\\[gx\\] ui\\-tree\\-migration"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTelegramStyleEpicCompleteText(t *testing.T) {
	t.Parallel()
	got := telegramStyle.epicCompleteText("ui-tree-migration", EpicCounts{Done: 8, Total: 10}, 5, 492, 12.5)
	want := "\U0001f389 *epic complete*\n\n8 done · 10 total\n5 ticket(s) landed in 8m12s · $12.50\n\\[gx\\] ui\\-tree\\-migration"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestEscapeTelegramMarkdownV2_EscapesSpecialChars(t *testing.T) {
	t.Parallel()
	got := escapeTelegramMarkdownV2("a.b-c_d*e[f]g(h)~i`j>k#l+m=n|o{p}q!r\\s")
	want := "a\\.b\\-c\\_d\\*e\\[f\\]g\\(h\\)\\~i\\`j\\>k\\#l\\+m\\=n\\|o\\{p\\}q\\!r\\\\s"
	if got != want {
		t.Errorf("escapeTelegramMarkdownV2 = %q, want %q", got, want)
	}
}

func TestEscapeSlackMrkdwn_EscapesAmpLtGt(t *testing.T) {
	t.Parallel()
	got := escapeSlackMrkdwn("a&b<c>d")
	want := "a&amp;b&lt;c&gt;d"
	if got != want {
		t.Errorf("escapeSlackMrkdwn = %q, want %q", got, want)
	}
}
