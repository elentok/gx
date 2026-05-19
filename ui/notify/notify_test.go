package notify_test

import (
	"testing"

	"github.com/elentok/gx/ui/notify"
)

func TestInfoCmd(t *testing.T) {
	cmd := notify.Info("hello")
	msg, ok := cmd().(notify.NotifyMsg)
	if !ok {
		t.Fatal("expected NotifyMsg")
	}
	if msg.Kind != notify.KindInfo || msg.Message != "hello" {
		t.Errorf("Info() = %+v", msg)
	}
}

func TestSuccessCmd(t *testing.T) {
	cmd := notify.Success("done")
	msg, ok := cmd().(notify.NotifyMsg)
	if !ok {
		t.Fatal("expected NotifyMsg")
	}
	if msg.Kind != notify.KindSuccess {
		t.Errorf("expected KindSuccess, got %d", msg.Kind)
	}
}

func TestWarningCmd(t *testing.T) {
	cmd := notify.Warning("watch out")
	msg, ok := cmd().(notify.NotifyMsg)
	if !ok {
		t.Fatal("expected NotifyMsg")
	}
	if msg.Kind != notify.KindWarning {
		t.Errorf("expected KindWarning, got %d", msg.Kind)
	}
}

func TestErrorCmd(t *testing.T) {
	cmd := notify.Error("oops")
	msg, ok := cmd().(notify.NotifyMsg)
	if !ok {
		t.Fatal("expected NotifyMsg")
	}
	if msg.Kind != notify.KindError {
		t.Errorf("expected KindError, got %d", msg.Kind)
	}
}

func TestProgressCmd(t *testing.T) {
	cmd := notify.Progress("my-id", "loading...")
	msg, ok := cmd().(notify.NotifyMsg)
	if !ok {
		t.Fatal("expected NotifyMsg")
	}
	if msg.Kind != notify.KindProgress || msg.ID != "my-id" {
		t.Errorf("Progress() = %+v", msg)
	}
}

func TestCloseCmd(t *testing.T) {
	cmd := notify.Close("my-id")
	msg, ok := cmd().(notify.CloseMsg)
	if !ok {
		t.Fatal("expected CloseMsg")
	}
	if msg.ID != "my-id" {
		t.Errorf("Close() ID = %q, want 'my-id'", msg.ID)
	}
}
