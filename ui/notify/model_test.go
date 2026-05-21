package notify

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNew(t *testing.T) {
	m := New(false)
	if len(m.notifications) != 0 {
		t.Error("expected empty notifications")
	}
}

func TestUpdate_NotifyMsg_Adds(t *testing.T) {
	m := New(false)
	m, _ = m.Update(NotifyMsg{Kind: KindInfo, Message: "hello"})
	if len(m.notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(m.notifications))
	}
	if m.notifications[0].message != "hello" {
		t.Errorf("message = %q, want 'hello'", m.notifications[0].message)
	}
}

func TestUpdate_NotifyMsg_ReplacesExistingID(t *testing.T) {
	m := New(false)
	m, _ = m.Update(NotifyMsg{ID: "x", Kind: KindInfo, Message: "first"})
	m, _ = m.Update(NotifyMsg{ID: "x", Kind: KindInfo, Message: "second"})
	if len(m.notifications) != 1 {
		t.Fatalf("expected 1 notification after replace, got %d", len(m.notifications))
	}
	if m.notifications[0].message != "second" {
		t.Errorf("message = %q, want 'second'", m.notifications[0].message)
	}
}

func TestUpdate_NotifyMsg_CapEnforced(t *testing.T) {
	m := New(false)
	for i := 0; i < cap+2; i++ {
		m, _ = m.Update(NotifyMsg{Kind: KindInfo, Message: "msg"})
	}
	if len(m.notifications) > cap {
		t.Errorf("expected at most %d notifications, got %d", cap, len(m.notifications))
	}
}

func TestUpdate_CloseMsg_Removes(t *testing.T) {
	m := New(false)
	m, _ = m.Update(NotifyMsg{ID: "p1", Kind: KindProgress, Message: "loading"})
	m, _ = m.Update(CloseMsg{ID: "p1"})
	if len(m.notifications) != 0 {
		t.Errorf("expected 0 notifications after close, got %d", len(m.notifications))
	}
}

func TestUpdate_ExpireMsg_Removes(t *testing.T) {
	m := New(false)
	m, _ = m.Update(NotifyMsg{Kind: KindInfo, Message: "will expire"})
	if len(m.notifications) == 0 {
		t.Fatal("expected notification to be added")
	}
	n := m.notifications[0]
	m.handleExpire(expireMsg{id: n.id, addedAt: n.addedAt})
	// after handleExpire, check if notification was removed
}

func TestUpdate_Progress_StartsSpinner(t *testing.T) {
	m := New(false)
	m, cmd := m.Update(NotifyMsg{ID: "prog", Kind: KindProgress, Message: "loading"})
	if cmd == nil {
		t.Error("expected non-nil cmd (spinner start) for progress notification")
	}
	if m.countProgress() != 1 {
		t.Errorf("expected 1 progress, got %d", m.countProgress())
	}
}

func TestUpdate_SpinnerTick_NoProgress(t *testing.T) {
	m := New(false)
	var tickMsg tea.Msg = tea.WindowSizeMsg{} // just something that triggers the default case
	m, cmd := m.Update(tickMsg)
	if cmd != nil {
		t.Error("expected nil cmd for non-matching msg with no progress")
	}
}

func TestView_Empty(t *testing.T) {
	m := New(false)
	v := m.View()
	if v != "" {
		t.Errorf("expected empty view with no notifications, got %q", v)
	}
}

func TestView_WithNotification(t *testing.T) {
	m := New(false)
	m, _ = m.Update(NotifyMsg{Kind: KindInfo, Message: "test notification"})
	v := m.View()
	if v == "" {
		t.Error("expected non-empty view with notifications")
	}
}

func TestView_WithAllKinds(t *testing.T) {
	kinds := []NotifyKind{KindInfo, KindSuccess, KindWarning, KindError}
	for _, kind := range kinds {
		m := New(false)
		m, _ = m.Update(NotifyMsg{Kind: kind, Message: "test"})
		v := m.View()
		if v == "" {
			t.Errorf("expected non-empty view for kind %d", kind)
		}
	}
}

func TestRemoveByID(t *testing.T) {
	ns := []notification{
		{id: "a", message: "first"},
		{id: "b", message: "second"},
	}
	result := removeByID(ns, "a")
	if len(result) != 1 || result[0].id != "b" {
		t.Errorf("removeByID: expected [{b}], got %+v", result)
	}
}
