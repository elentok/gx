package tickets

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elentok/gx/ralphloop"
	"github.com/elentok/gx/transcript"
)

// writeFakeTranscript writes a minimal transcript file at the path
// transcript.Path(cwd, sessionID) resolves to (under HOME, which the caller
// must have pointed at a temp dir via t.Setenv), with a single line stamped
// at start — enough for resolveStartedAt's FirstLineTimestamp read.
func writeFakeTranscript(t *testing.T, cwd, sessionID string, start time.Time) {
	t.Helper()
	path, err := transcript.Path(cwd, sessionID)
	if err != nil {
		t.Fatalf("transcript.Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	line := `{"timestamp":"` + start.UTC().Format(time.RFC3339Nano) + `","type":"user","message":{}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestApplyLiveEvent_IterationStarted_PopulatesStartedAt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cwd := "/fake/iter-01"
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	writeFakeTranscript(t, cwd, "sess-1", start)

	live := map[string]liveTicketState{}
	labelIdentifier := map[string]string{}
	applyLiveEvent(live, labelIdentifier, nil, ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationStarted, Identifier: "01", Label: "iter-01",
		Cwd: cwd, SessionID: "sess-1",
	})

	got, ok := live["01"]
	if !ok {
		t.Fatal("expected a live entry for ticket 01")
	}
	if !got.startedAt.Equal(start) {
		t.Errorf("startedAt = %v, want %v", got.startedAt, start)
	}
	if got.tokens != 0 {
		t.Errorf("tokens = %d, want 0 (no ContextOccupancy event yet)", got.tokens)
	}
}

func TestApplyLiveEvent_ContextOccupancy_UpdatesTokens(t *testing.T) {
	live := map[string]liveTicketState{
		"01": {running: true, label: "iter-01"},
	}
	labelIdentifier := map[string]string{"iter-01": "01"}

	applyLiveEvent(live, labelIdentifier, nil, ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventContextOccupancy, Identifier: "01", Tokens: 45200,
	})

	if got := live["01"].tokens; got != 45200 {
		t.Errorf("tokens = %d, want 45200", got)
	}
}

func TestApplyLiveEvent_PauseResumeCycle_PreservesStartedAtAndTokens(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	live := map[string]liveTicketState{
		"01": {running: true, label: "iter-01", startedAt: start, tokens: 45200},
	}
	labelIdentifier := map[string]string{"iter-01": "01"}

	applyLiveEvent(live, labelIdentifier, nil, ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationPaused, Label: "iter-01",
		PauseKind: ralphloop.PauseRateLimit, Reason: "rate limited",
	})

	paused := live["01"]
	if !paused.paused {
		t.Fatal("expected ticket 01 to be paused")
	}
	if !paused.startedAt.Equal(start) {
		t.Errorf("after pause: startedAt = %v, want %v (preserved)", paused.startedAt, start)
	}
	if paused.tokens != 45200 {
		t.Errorf("after pause: tokens = %d, want 45200 (preserved)", paused.tokens)
	}

	applyLiveEvent(live, labelIdentifier, nil, ralphloop.LiveEvent{
		Kind: ralphloop.LiveEventIterationResumed, Label: "iter-01", PauseKind: ralphloop.PauseRateLimit,
	})

	resumed := live["01"]
	if !resumed.running {
		t.Fatal("expected ticket 01 to be running again after resume")
	}
	if !resumed.startedAt.Equal(start) {
		t.Errorf("after resume: startedAt = %v, want %v (preserved)", resumed.startedAt, start)
	}
	if resumed.tokens != 45200 {
		t.Errorf("after resume: tokens = %d, want 45200 (preserved)", resumed.tokens)
	}
}
