package tickets

import "testing"

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
