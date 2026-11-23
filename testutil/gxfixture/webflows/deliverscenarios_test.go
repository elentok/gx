package webflows

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/elentok/gx/server/wire"
	"github.com/elentok/gx/testutil"
)

// launch creates the one session of a deliver-launch-* fixture and returns the
// create output.
func (r *running) createLaunch(t *testing.T, prompt string) string {
	t.Helper()
	return r.Gx(t, "agents", "create", "--project", "demo", "--dir", r.Repo(), "--prompt", prompt, "--json")
}

func (r *running) onlySession(t *testing.T) (wire.SessionInfo, string) {
	t.Helper()
	raw := r.Gx(t, "agents", "list", "--json")
	var list []wire.SessionInfo
	if err := json.Unmarshal([]byte(raw), &list); err != nil || len(list) != 1 {
		return wire.SessionInfo{}, raw
	}
	return list[0], raw
}

// TestDeliverScenario_UndecidedLaunchSlashPromptLeavesNoCard: the first slash
// prompt stays in the box, so gx cannot tell and raises nothing.
func TestDeliverScenario_UndecidedLaunchSlashPromptLeavesNoCard(t *testing.T) {
	t.Parallel()
	r := start(t, "deliver-launch-undecided")
	r.createLaunch(t, "/model opus")
	// The grace is 2s; give the first prompt well past it, then the card must still be absent.
	time.Sleep(8 * time.Second) //nolint:forbidigo // absence of a card has no event to wait for
	got, raw := r.onlySession(t)
	if got.ID == "" {
		t.Fatalf("no single session: %s", raw)
	}
	if got.Actionable.Kind != "" || got.Actionable.Reason != "" {
		t.Errorf("actionable = %+v, want empty", got.Actionable)
	}
	r.Stop(t)
}

// TestDeliverScenario_FirstPromptExits: each scenario's knob ends the first prompt on its card.
func TestDeliverScenario_FirstPromptExits(t *testing.T) {
	t.Parallel()
	rows := []struct {
		scenario string
		prompt   string
		kind     string
		reason   string
	}{
		{"deliver-launch-rejected", "/gx-implement demo:ready-epic/01", wire.DeliveryUnknownCommand, "Unknown command:"},
		{"deliver-launch-never-started", "do the thing", wire.DeliverySendFailed, "agent never started"},
	}
	for _, row := range rows {
		t.Run(row.scenario, func(t *testing.T) {
			t.Parallel()
			r := start(t, row.scenario)
			created := r.createLaunch(t, row.prompt)
			var got wire.SessionInfo
			var raw string
			if !testutil.Poll(60*time.Second, 300*time.Millisecond, func() bool {
				got, raw = r.onlySession(t)
				return got.Actionable.Kind == row.kind
			}) {
				t.Fatalf("never raised %s; create said %q, list said %q", row.kind, created, raw)
			}
			if !strings.Contains(got.Actionable.Reason, row.reason) {
				t.Errorf("reason = %q, want it to hold %q", got.Actionable.Reason, row.reason)
			}
			r.Stop(t)
		})
	}
}
