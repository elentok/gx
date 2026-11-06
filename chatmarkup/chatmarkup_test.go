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

func TestLayout(t *testing.T) {
	headline := "🙋 Answer e/04 Fix *this* _now_ (v2)."
	lines := []string{"detail.one"}
	if got, want := Telegram.Layout(headline, lines, "my-proj").String(),
		"🙋 Answer e/04 Fix \\*this\\* \\_now\\_ \\(v2\\)\\.\ndetail\\.one\nmy\\-proj"; got != want {
		t.Errorf("Telegram Layout = %q, want %q", got, want)
	}
	if got, want := Slack.Layout(headline, lines, "my-proj").String(), headline+"\ndetail.one\nmy-proj"; got != want {
		t.Errorf("Slack Layout = %q, want %q", got, want)
	}
}

func TestLayout_EmptyProjectHasNoLastLine(t *testing.T) {
	if got, want := Slack.Layout("🚀 Server started", []string{"build x"}, "").String(), "🚀 Server started\nbuild x"; got != want {
		t.Errorf("Layout = %q, want %q", got, want)
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

func TestStylePlain(t *testing.T) {
	// A bold headline plus a body containing an escaped literal asterisk
	// (e.g. from a ticket title "fix *ptr deref") must come out with the
	// bold marker stripped and the literal "*" surviving unescaped and
	// unstripped, per the epic-wide dedup incident this seals off.
	headline := Text{s: "*urgent*"}
	body := Telegram.Escape("fix *ptr deref")
	full := Join(Telegram.Escape("\n"), []Text{headline, body})

	got := Telegram.Plain(full)
	want := "urgent\nfix *ptr deref"
	if got != want {
		t.Errorf("Plain = %q, want %q", got, want)
	}
}

func TestStylePlainItalic(t *testing.T) {
	text := Text{s: "__careful__"}
	got := Telegram.Plain(text)
	want := "careful"
	if got != want {
		t.Errorf("Plain = %q, want %q", got, want)
	}
}

func TestStylePlainSlack(t *testing.T) {
	text := Text{s: "*hi & bye*"}
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
