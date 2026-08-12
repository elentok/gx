package ralphloop

import (
	"errors"
	"strings"
	"testing"
)

func TestEpicFailureReporter_EpicFailed_SendsOneMessagePerTarget(t *testing.T) {
	r := NewEpicFailureReporter(t.TempDir())
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
	transport := &fakeChatTransport{}
	r.targets = append(r.targets, epicFailureTarget{style: slackStyle, transport: transport})

	r.EpicFailed("my-epic", nil)

	if got := transport.snapshot(); len(got) != 0 {
		t.Fatalf("sent = %v, want none for a nil error", got)
	}
}
