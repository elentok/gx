package notifylog_test

import (
	"testing"

	"github.com/elentok/gx/ui/notify"
	"github.com/elentok/gx/ui/notifylog"
)

func TestAppend_EvictsOldestFirstAtCapacity(t *testing.T) {
	l := notifylog.New()
	for range notifylog.Capacity + 1 {
		l.Append(notify.NotifyMsg{Kind: notify.KindInfo, Message: "msg"})
	}

	entries := l.Entries()
	if len(entries) != notifylog.Capacity {
		t.Fatalf("len(entries) = %d, want %d", len(entries), notifylog.Capacity)
	}
}

func TestAppend_ProgressProducesStartedAndClosedRows(t *testing.T) {
	l := notifylog.New()
	l.Append(notify.NotifyMsg{ID: "p1", Kind: notify.KindProgress, Message: "loading"})
	l.Close("p1")

	entries := l.Entries()
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].Closed {
		t.Errorf("entries[0].Closed = true, want false (started row)")
	}
	if !entries[1].Closed {
		t.Errorf("entries[1].Closed = false, want true (closed row)")
	}
	if entries[1].Message != "loading" {
		t.Errorf("entries[1].Message = %q, want %q", entries[1].Message, "loading")
	}
}

func TestAppend_OtherKindProducesOneRow(t *testing.T) {
	l := notifylog.New()
	l.Append(notify.NotifyMsg{Kind: notify.KindSuccess, Message: "done"})

	entries := l.Entries()
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
}

func TestClose_WithoutMatchingProgressIsIgnored(t *testing.T) {
	l := notifylog.New()
	l.Close("missing")

	if len(l.Entries()) != 0 {
		t.Fatalf("len(entries) = %d, want 0", len(l.Entries()))
	}
}
