package tickets

import "testing"

func baseEpics(blockerStatus string) []Epic {
	return []Epic{
		{Name: "a", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: blockerStatus}}},
		{Name: "b", BlockedBy: []string{"a/01"}, Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open"}}},
	}
}

func TestDeriveRootBase(t *testing.T) {
	for name, tc := range map[string]struct {
		epics   []Epic
		want    string
		wantErr bool
	}{
		"unlanded blocker gives its branch": {epics: baseEpics("open"), want: "a"},
		"landed blocker gives trunk":        {epics: baseEpics("done"), want: ""},
		"no blockers gives trunk": {epics: func() []Epic {
			e := baseEpics("open")
			e[1].BlockedBy = nil
			return e
		}(), want: ""},
		"commitless blocker is ignored": {epics: func() []Epic {
			e := baseEpics("open")
			e[0].Tickets[0].Commitless = true
			return e
		}(), want: ""},
		"two unlanded blockers are ambiguous": {epics: func() []Epic {
			e := append(baseEpics("open"), Epic{Name: "c", Tickets: []Ticket{{Number: 1, Identifier: "01", Status: "open"}}})
			e[1].BlockedBy = []string{"a/01", "c/01"}
			return e
		}(), wantErr: true},
		"base node ref unlanded": {epics: func() []Epic {
			e := baseEpics("open")
			e[1].BlockedBy, e[1].Base = nil, "a/01"
			return e
		}(), want: "a"},
		"base node ref landed": {epics: func() []Epic {
			e := baseEpics("done")
			e[1].BlockedBy, e[1].Base = nil, "a/01"
			return e
		}(), want: ""},
		"raw branch base": {epics: func() []Epic {
			e := baseEpics("open")
			e[1].Base = "release/1.0"
			return e
		}(), want: "release/1.0"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := DeriveRootBase("p", tc.epics, "b", tc.epics[1].Tickets[0])
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Errorf("got (%q, %v), want (%q, err=%v)", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
