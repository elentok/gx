package ticketwrite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/tickets"
	"github.com/elentok/gx/tickets/ticketlog"
)

// pausedTicket writes a paused ticket with the given reason under a fresh
// epic and, when ask is set, an open agent question in its log.
func pausedTicket(t *testing.T, id, reason string, ask bool) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "epic-a", "issues")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, id+"-x.md")
	body := "---\nid: \"" + id + "\"\nstatus: paused\npause_reason: " + reason + "\ntype: implement\n---\n# X\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if ask {
		if _, err := ticketlog.AppendLog(path, ticketlog.LogEvent{Type: ticketlog.EventPauseQuestion, By: "agent", Summary: "which?", Handoff: "files touched"}); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestWriterPush_PauseRoute(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		reason string
		kind   events.Kind
		ask    bool
		want   string
	}{
		{"self-reported with open question", "self-reported", events.SelfReported, true, "/answer/proj:epic-a/01"},
		{"self-reported without a question", "self-reported", events.SelfReported, false, "/t/proj:epic-a/01"},
		{"gx-asked answer pause", "zero-commit", events.ZeroCommit, true, "/t/proj:epic-a/01"},
		{"repair pause", "iteration-error", events.IterationError, false, "/t/proj:epic-a/01"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rec := &pushRecorder{}
			w := retroServer(t.TempDir())
			w.d.Chat = notify.NewServerChat(notify.ServerChatConfig{Push: rec})
			path := pausedTicket(t, "01", c.reason, c.ask)

			w.NotifyPause(tickets.Address{Project: "proj", Epic: "epic-a", ID: "01"}, path, c.kind, "why")
			if len(rec.sent) != 1 {
				t.Fatalf("pushes = %+v, want one", rec.sent)
			}
			got := rec.sent[0]
			if p := got.Target.Path(); p != c.want {
				t.Errorf("path = %q, want %q", p, c.want)
			}
			if tag := got.Target.Tag(got.Kind); tag != "ticket:proj:epic-a/01" {
				t.Errorf("tag = %q, want the unchanged ticket tag", tag)
			}
		})
	}
}

func TestWriterPush_ChildPauseRoute(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		reason string
		kind   events.Kind
		ask    bool
		want   string
	}{
		{"self-reported", events.SelfReported, true, "/answer/proj:epic-a/01a"},
		{"iteration-error", events.IterationError, false, "/t/proj:epic-a/01a"},
	} {
		t.Run(c.reason, func(t *testing.T) {
			t.Parallel()
			rec := &pushRecorder{}
			w := retroServer(t.TempDir())
			w.d.Chat = notify.NewServerChat(notify.ServerChatConfig{Push: rec})
			child := pausedTicket(t, "01a", c.reason, c.ask)
			// The parent sits beside the child: <scratch>/<epic>/issues/<id>-*.md.
			parentPath := filepath.Join(filepath.Dir(child), "01-first.md")
			sink := w.FinishSink(tickets.Address{Project: "proj", Epic: "epic-a", ID: "01"}, parentPath)

			sink.TicketNeedsHuman("01a", "epic-a", "paused", c.kind, "why")
			if len(rec.sent) != 1 {
				t.Fatalf("pushes = %+v, want one", rec.sent)
			}
			if p := rec.sent[0].Target.Path(); p != c.want {
				t.Errorf("path = %q, want %q", p, c.want)
			}
		})
	}
}

type pushRecorder struct{ sent []notify.PushNote }

func (r *pushRecorder) Send(n notify.PushNote) { r.sent = append(r.sent, n) }

func TestWriterPush_ConflictChildPauseSendsOnePush(t *testing.T) {
	t.Parallel()
	rec := &pushRecorder{}
	w := retroServer(t.TempDir())
	w.d.Chat = notify.NewServerChat(notify.ServerChatConfig{Push: rec})
	parentPath := filepath.Join(t.TempDir(), "epic-a", "issues", "01-first.md")
	sink := w.FinishSink(tickets.Address{Project: "proj", Epic: "epic-a", ID: "01"}, parentPath)

	sink.TicketNeedsHuman("01", "epic-a", "paused", events.IterationError, "the parent")
	if len(rec.sent) != 0 {
		t.Fatalf("the parent's own pause pushed %+v, want none", rec.sent)
	}

	sink.TicketNeedsHuman("01a", "epic-a", "paused", events.IterationError, "boom")
	if len(rec.sent) != 1 {
		t.Fatalf("pushes = %+v, want one for the child", rec.sent)
	}
	got := rec.sent[0]
	if got.Tag != "" || got.Target.Tag(got.Kind) != "ticket:proj:epic-a/01a" || got.Target.Path() != "/t/proj:epic-a/01a" || got.Urgent || got.Body != "boom" {
		t.Errorf("push = %+v, want the child's tag and path, normal urgency", got)
	}
}
