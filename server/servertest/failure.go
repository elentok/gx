package servertest

import (
	"context"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
)

const (
	APIError401 = `{"type":"assistant","uuid":"e1","isApiErrorMessage":true,"error":"authentication_error","apiErrorStatus":401,"timestamp":"2026-01-01T10:00:00Z","message":{"content":[{"type":"text","text":"Invalid API key"}]}}`
	Reply       = `{"type":"assistant","uuid":"a1","timestamp":"2026-01-01T10:00:01Z","message":{"content":[{"type":"text","text":"hi"}]}}`
)

// FailureNow reads session id's failure from the list route and the chat route.
func FailureNow(ctx context.Context, t *testing.T, h *Harness, id string) (list, chat *wire.SessionFailure) {
	t.Helper()
	sessions, err := h.Client.Sessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range sessions {
		if s.ID == id {
			list = s.Failure
		}
	}
	res, err := h.Client.SessionChat(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return list, res.Failure
}

// WaitForFailure polls until the session's failure has kind ("" for none) on
// both routes and returns it.
func WaitForFailure(ctx context.Context, t *testing.T, h *Harness, id, kind string) *wire.SessionFailure {
	t.Helper()
	var list, chat *wire.SessionFailure
	for {
		list, chat = FailureNow(ctx, t, h, id)
		if kindOf(list) == kind && kindOf(chat) == kind {
			if list != nil && *list != *chat {
				t.Fatalf("list failure %+v differs from chat failure %+v", list, chat)
			}
			return list
		}
		select {
		case <-ctx.Done():
			t.Fatalf("failure never became %q; list %+v, chat %+v", kind, list, chat)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func kindOf(f *wire.SessionFailure) string {
	if f == nil {
		return ""
	}
	return f.Kind
}
