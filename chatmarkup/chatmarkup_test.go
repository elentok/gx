package chatmarkup

import "testing"

func TestStyleEscape(t *testing.T) {
	cases := []struct {
		name  string
		style Style
		raw   string
		want  string
	}{
		{
			name:  "telegram escapes every reserved char",
			style: Telegram,
			raw:   "_*[]()~`>#+-=|{}.!\\",
			want:  "\\_\\*\\[\\]\\(\\)\\~\\`\\>\\#\\+\\-\\=\\|\\{\\}\\.\\!\\\\",
		},
		{
			name:  "telegram leaves ordinary text alone",
			style: Telegram,
			raw:   "hello world 123",
			want:  "hello world 123",
		},
		{
			name:  "slack escapes only amp lt gt",
			style: Slack,
			raw:   "a & b < c > d",
			want:  "a &amp; b &lt; c &gt; d",
		},
		{
			name:  "slack leaves asterisk and underscore alone",
			style: Slack,
			raw:   "*bold* _ital_",
			want:  "*bold* _ital_",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.style.Escape(c.raw).String()
			if got != c.want {
				t.Errorf("Escape(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

func TestStyleBoldItalic(t *testing.T) {
	if got, want := Telegram.Bold("hi").String(), "*hi*"; got != want {
		t.Errorf("Telegram.Bold = %q, want %q", got, want)
	}
	if got, want := Telegram.Italic("hi").String(), "__hi__"; got != want {
		t.Errorf("Telegram.Italic = %q, want %q", got, want)
	}
	if got, want := Slack.Bold("hi").String(), "*hi*"; got != want {
		t.Errorf("Slack.Bold = %q, want %q", got, want)
	}
	if got, want := Slack.Italic("hi").String(), "_hi_"; got != want {
		t.Errorf("Slack.Italic = %q, want %q", got, want)
	}

	// Content is escaped before wrapping.
	if got, want := Telegram.Bold("a.b").String(), "*a\\.b*"; got != want {
		t.Errorf("Telegram.Bold(a.b) = %q, want %q", got, want)
	}
}

func TestStyleMessage(t *testing.T) {
	text := Telegram.Message("🚀", "epic.started", "3/5 done", "some.detail", "epic.name")
	want := "🚀 *epic\\.started*\n\n3/5 done\n" +
		"some\\.detail\n" +
		"epic\\.name"
	if got := text.String(); got != want {
		t.Errorf("Message = %q, want %q", got, want)
	}
}

func TestStyleMessageOmitsEmptyLines(t *testing.T) {
	text := Telegram.Message("🚀", "started", "", "", "epic")
	want := "🚀 *started*\n\nepic"
	if got := text.String(); got != want {
		t.Errorf("Message = %q, want %q", got, want)
	}
}

func TestJoin(t *testing.T) {
	sep := Telegram.Escape(", ")
	items := []Text{Telegram.Escape("a.a"), Telegram.Escape("b.b"), Telegram.Escape("c.c")}
	got := Join(sep, items).String()
	want := "a\\.a, b\\.b, c\\.c"
	if got != want {
		t.Errorf("Join = %q, want %q", got, want)
	}
}

func TestTextWithSuffix(t *testing.T) {
	base := Telegram.Escape("ticket.landed")
	got := base.WithSuffix(" ×3!", Telegram).String()
	want := "ticket\\.landed ×3\\!"
	if got != want {
		t.Errorf("WithSuffix = %q, want %q", got, want)
	}
}

func TestStylePlain(t *testing.T) {
	// A bold headline plus a body containing an escaped literal asterisk
	// (e.g. from a ticket title "fix *ptr deref") must come out with the
	// bold marker stripped and the literal "*" surviving unescaped and
	// unstripped, per the epic-wide dedup incident this seals off.
	headline := Telegram.Bold("urgent")
	body := Telegram.Escape("fix *ptr deref")
	full := Join(Telegram.Escape("\n"), []Text{headline, body})

	got := Telegram.Plain(full)
	want := "urgent\nfix *ptr deref"
	if got != want {
		t.Errorf("Plain = %q, want %q", got, want)
	}
}

func TestStylePlainItalic(t *testing.T) {
	text := Telegram.Italic("careful")
	got := Telegram.Plain(text)
	want := "careful"
	if got != want {
		t.Errorf("Plain = %q, want %q", got, want)
	}
}

func TestStylePlainSlack(t *testing.T) {
	text := Slack.Bold("hi & bye")
	got := Slack.Plain(text)
	want := "hi & bye"
	if got != want {
		t.Errorf("Plain = %q, want %q", got, want)
	}
}

func TestTextStringRoundTrips(t *testing.T) {
	raw := "a.b_c*d"
	text := Telegram.Escape(raw)
	if got, want := text.String(), "a\\.b\\_c\\*d"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
