package ralphloop

import (
	"testing"

	"github.com/elentok/gx/tickets"
)

func TestFormatDuration(t *testing.T) {
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

func TestTelegramStyleIterationFinishedText_EscapesAndFormats(t *testing.T) {
	ticket := tickets.Ticket{Identifier: "02", Title: "Migrate ui"}
	stats := IterationStats{ElapsedSeconds: 332, PeakContextTokens: 39000, Completed: 2, Total: 5}

	got := telegramStyle.iterationFinishedText(ticket, "ui-tree-migration", stats)
	want := "✅ *Migrate ui*\n\n5m32s · 39k tok · 2/5 done\n\\[gx\\] ui\\-tree\\-migration/02"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestSlackStyleIterationFinishedText_NoEscaping(t *testing.T) {
	ticket := tickets.Ticket{Identifier: "02", Title: "Migrate ui"}
	stats := IterationStats{ElapsedSeconds: 332, PeakContextTokens: 39000, Completed: 2, Total: 5}

	got := slackStyle.iterationFinishedText(ticket, "ui-tree-migration", stats)
	want := "✅ *Migrate ui*\n\n5m32s · 39k tok · 2/5 done\n[gx] ui-tree-migration/02"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTelegramStyleIterationPausedText_NeedsAttentionUsesStopEmoji(t *testing.T) {
	got := telegramStyle.iterationPausedText("iter-04", PauseNeedsAttention, "agent blocked on permission prompt")
	want := "\U0001f6d1 *iter\\-04 paused*\n\nagent blocked on permission prompt"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTelegramStyleIterationPausedText_RateLimitUsesPauseEmoji(t *testing.T) {
	got := telegramStyle.iterationPausedText("iter-04", PauseRateLimit, "rate limit hit")
	want := "⏸ *iter\\-04 paused*\n\nrate limit hit"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTelegramStyleTicketNeedsInfoText_DistinctFromIterationPausedText(t *testing.T) {
	got := telegramStyle.ticketNeedsInfoText("04", "ui-tree-migration")
	want := "\U0001f198 *ui\\-tree\\-migration/04 needs info*\n\nNo commits landed; marked needs\\-info\\."
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if paused := telegramStyle.iterationPausedText("ui-tree-migration/04", PauseNeedsAttention, "stuck"); got == paused {
		t.Errorf("ticketNeedsInfoText matched iterationPausedText: %q", got)
	}
}

func TestSlackStyleTicketNeedsInfoText_NoEscaping(t *testing.T) {
	got := slackStyle.ticketNeedsInfoText("04", "ui-tree-migration")
	want := "\U0001f198 *ui-tree-migration/04 needs info*\n\nNo commits landed; marked needs-info."
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestTelegramStyleEpicCompleteText(t *testing.T) {
	got := telegramStyle.epicCompleteText("ui-tree-migration", 5, 492)
	want := "\U0001f389 *epic complete: ui\\-tree\\-migration*\n\n5 tickets landed in 8m12s"
	if got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
}

func TestEscapeTelegramMarkdownV2_EscapesSpecialChars(t *testing.T) {
	got := escapeTelegramMarkdownV2("a.b-c_d*e[f]g(h)~i`j>k#l+m=n|o{p}q!r\\s")
	want := "a\\.b\\-c\\_d\\*e\\[f\\]g\\(h\\)\\~i\\`j\\>k\\#l\\+m\\=n\\|o\\{p\\}q\\!r\\\\s"
	if got != want {
		t.Errorf("escapeTelegramMarkdownV2 = %q, want %q", got, want)
	}
}

func TestEscapeSlackMrkdwn_EscapesAmpLtGt(t *testing.T) {
	got := escapeSlackMrkdwn("a&b<c>d")
	want := "a&amp;b&lt;c&gt;d"
	if got != want {
		t.Errorf("escapeSlackMrkdwn = %q, want %q", got, want)
	}
}
