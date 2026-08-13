package ralphloop

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestEpicFailureReporter_EpicFailed_SendsOneMessagePerTarget(t *testing.T) {
	r := NewEpicFailureReporter(t.TempDir())
	r.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")
	transport := &fakeChatTransport{}
	r.targets = append(r.targets, epicFailureTarget{style: slackStyle, transport: transport})

	r.EpicFailed("my-epic", errors.New("boom"))

	got := waitForSentCount(transport, 1)
	if len(got) != 1 {
		t.Fatalf("sent = %v, want exactly one message", got)
	}
	if !strings.Contains(got[0], "epic failed") || !strings.Contains(got[0], "boom") || !strings.Contains(got[0], "my-epic") {
		t.Fatalf("message = %q, want headline/err/epic name", got[0])
	}
}

func TestEpicFailureReporter_EpicFailed_NilErrSendsNothing(t *testing.T) {
	r := NewEpicFailureReporter(t.TempDir())
	r.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")
	transport := &fakeChatTransport{}
	r.targets = append(r.targets, epicFailureTarget{style: slackStyle, transport: transport})

	r.EpicFailed("my-epic", nil)

	if got := transport.snapshot(); len(got) != 0 {
		t.Fatalf("sent = %v, want none for a nil error", got)
	}
}

func TestEpicFailureReporter_EpicFailed_GlobalBreakerTrips_SuppressesFurtherSends(t *testing.T) {
	r := NewEpicFailureReporter(t.TempDir())
	r.gateStatePath = filepath.Join(t.TempDir(), "notifications-state.json")
	transport := &fakeChatTransport{}
	r.targets = append(r.targets, epicFailureTarget{style: slackStyle, transport: transport})

	for range globalThreshold + 5 {
		r.EpicFailed("my-epic", errors.New("boom"))
	}

	want := globalThreshold - 1
	got := waitForSentCount(transport, want)
	if len(got) != want {
		t.Fatalf("sent = %d messages, want exactly %d (breaker trips before the send that crosses the threshold)", len(got), want)
	}
}
