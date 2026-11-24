package ticketwrite

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/ralphloop/notify"
	"github.com/elentok/gx/tickets"
)

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
