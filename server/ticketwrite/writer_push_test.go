package ticketwrite

import (
	"path/filepath"
	"testing"

	"github.com/elentok/gx/events"
	"github.com/elentok/gx/tickets"
)

type recordedPush struct {
	title, body, path, tag string
	urgent                 bool
}

type pushRecorder struct{ sent []recordedPush }

func (r *pushRecorder) SendPush(title, body, path, tag string, urgent bool) {
	r.sent = append(r.sent, recordedPush{title, body, path, tag, urgent})
}

func TestWriterPush_ConflictChildPauseSendsOnePush(t *testing.T) {
	t.Parallel()
	rec := &pushRecorder{}
	w := retroServer(t.TempDir()) // no Chat: the push does not need it
	w.d.Push = rec
	parentPath := filepath.Join(t.TempDir(), "epic-a", "issues", "01-first.md")
	sink := w.FinishSink(tickets.Address{Project: "proj", Epic: "epic-a", ID: "01"}, parentPath)

	sink.TicketNeedsHuman("01", "epic-a", "needs-repair", events.IterationError, "the parent")
	if len(rec.sent) != 0 {
		t.Fatalf("the parent's own pause pushed %+v, want none", rec.sent)
	}

	sink.TicketNeedsHuman("01a", "epic-a", "needs-repair", events.IterationError, "boom")
	if len(rec.sent) != 1 {
		t.Fatalf("pushes = %+v, want one for the child", rec.sent)
	}
	got := rec.sent[0]
	if got.tag != "ticket:proj:epic-a/01a" || got.path != "/t/proj:epic-a/01a" || got.urgent || got.body != "boom" {
		t.Errorf("push = %+v, want the child's tag and path, normal urgency", got)
	}
}
