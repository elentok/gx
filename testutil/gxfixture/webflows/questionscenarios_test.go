package webflows

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/elentok/gx/tickets/ticketlog"
)

func TestQuestionScenarios_OneOffQuestionIsPausedAgentAsk(t *testing.T) {
	t.Parallel()
	r := start(t, "one-off-question")
	d := r.Detail(t, "demo:one-offs/01")
	if r.TicketStatus(t, "demo:one-offs/01") != askedStatus {
		t.Errorf("status = %q, want %q", r.TicketStatus(t, "demo:one-offs/01"), askedStatus)
	}
	if d.Latest.By != "agent" || len(d.Latest.Options) != 3 || d.Latest.Handoff == "" {
		t.Errorf("latest = %+v, want an agent ask with three options and a handoff", d.Latest)
	}
	for _, want := range []string{"answer", "resume"} {
		if !slices.Contains(d.Actions, want) {
			t.Errorf("actions = %v, want %s", d.Actions, want)
		}
	}
	r.Stop(t)
}

func TestQuestionScenarios_PermissionBlocksAnAskedAgent(t *testing.T) {
	t.Parallel()
	r := start(t, "permission")
	const addr = "demo:permission-epic/01"
	r.WaitStatus(t, addr, askedStatus)
	d := r.WaitPermission(t, addr)
	if d.Latest.By != "agent" || len(d.Latest.Options) != 2 || d.Latest.Recommended == "" {
		t.Errorf("latest = %+v, want an agent ask with two options and a recommended one", d.Latest)
	}
	data, ok := r.Web(t).Try(t, "/v1/tickets/answer", map[string]any{"address": addr, "state": d.State, "choice": d.Latest.Recommended})
	if ok || !strings.Contains(data, "agent-not-ready") {
		t.Errorf("answer = %s (ok %v), want an agent-not-ready refusal", data, ok)
	}
	if got := r.TicketStatus(t, addr); got != askedStatus {
		t.Errorf("status after the refusal = %q, want %q: the round stays open", got, askedStatus)
	}
	r.Stop(t)
}

func TestQuestionScenarios_LongAndShortAgentQuestions(t *testing.T) {
	t.Parallel()
	r := start(t, "long-question")
	long := r.Detail(t, "demo:long-question-epic/01")
	short := r.Detail(t, "demo:long-question-epic/02")

	for name, d := range map[string]*struct {
		actions []string
		by      string
	}{
		"01": {long.Actions, long.Latest.By},
		"02": {short.Actions, short.Latest.By},
	} {
		if d.by != "agent" {
			t.Errorf("%s: question by %q, want agent", name, d.by)
		}
		for _, want := range []string{"answer", "resume"} {
			if !slices.Contains(d.actions, want) {
				t.Errorf("%s: actions = %v, want %s", name, d.actions, want)
			}
		}
	}

	if n := utf8.RuneCountInString(long.Latest.Summary); n <= 80 || n > ticketlog.MaxQuestionSummary {
		t.Errorf("long summary is %d characters, want 81 to %d", n, ticketlog.MaxQuestionSummary)
	}
	if long.Latest.Handoff == "" || len(long.Latest.Options) != 3 || long.Latest.Recommended != "b" {
		t.Errorf("long latest = %+v, want a handoff, three options, recommended b", long.Latest)
	}
	if len(long.Rounds) != 2 {
		t.Fatalf("long has %d earlier rounds, want 2", len(long.Rounds))
	}
	if len(short.Rounds) != 0 || utf8.RuneCountInString(short.Latest.Summary) > 40 {
		t.Errorf("short = %d rounds, summary %q; want none and a short title", len(short.Rounds), short.Latest.Summary)
	}
	if len(short.Latest.Options) != 3 || short.Latest.Recommended != "b" || short.Latest.Handoff == "" {
		t.Errorf("short latest = %+v, want the same options, recommended b and a handoff", short.Latest)
	}
	r.Stop(t)
}
