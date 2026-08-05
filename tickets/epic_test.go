package tickets

import (
	"testing"
	"time"
)

func TestEpic_AllDone(t *testing.T) {
	cases := []struct {
		name    string
		tickets []Ticket
		want    bool
	}{
		{"zero tickets", nil, false},
		{"all done", []Ticket{{Number: 1, Status: "done"}, {Number: 2, Status: "resolved"}}, true},
		{"one open", []Ticket{{Number: 1, Status: "done"}, {Number: 2, Status: "open"}}, false},
	}

	for _, c := range cases {
		epic := Epic{Tickets: c.tickets}
		if got := epic.AllDone(); got != c.want {
			t.Errorf("%s: AllDone() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestEpic_CompletionDuration(t *testing.T) {
	started := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	completed := started.Add(2*time.Hour + 15*time.Minute)

	both := Epic{StartedAt: started, CompletedAt: completed}
	if dur, ok := both.CompletionDuration(); !ok || dur != 2*time.Hour+15*time.Minute {
		t.Errorf("both timestamps set: got dur=%v ok=%v, want 2h15m/true", dur, ok)
	}

	missingCompleted := Epic{StartedAt: started}
	if _, ok := missingCompleted.CompletionDuration(); ok {
		t.Errorf("missing completed_at: got ok=true, want false")
	}

	missingStarted := Epic{CompletedAt: completed}
	if _, ok := missingStarted.CompletionDuration(); ok {
		t.Errorf("missing started_at: got ok=true, want false")
	}

	neither := Epic{}
	if _, ok := neither.CompletionDuration(); ok {
		t.Errorf("neither timestamp set: got ok=true, want false")
	}
}
